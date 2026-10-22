package console

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/api"
	serverconfig "github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	consoleapi "github.com/router-for-me/CLIProxyAPI/v8/internal/console/api"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/assets"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/middleware"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v8/sdk/access"
	sdkhandlers "github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/sirupsen/logrus"
)

var (
	globalSubsystemMu sync.Mutex
	globalSubsystem   *Subsystem
	bootstrapOnce     sync.Once
)

// Subsystem represents the running Console instance.
type Subsystem struct {
	cfg       SubsystemConfig
	db        *db.DB
	engine    *engine.Engine
	pricing   *consoleapi.PricingEngine
	providers *consoleapi.ProviderRegistryEngine
	logger    *logrus.Entry
}

// InitSubsystem initializes database, engine, pricing, and provider registry.
func InitSubsystem(cfg SubsystemConfig) (*Subsystem, error) {
	globalSubsystemMu.Lock()
	defer globalSubsystemMu.Unlock()

	if globalSubsystem != nil {
		return globalSubsystem, nil
	}

	logger := logrus.WithField("subsystem", "console")

	// Ensure directory for db exists if a path with directories is supplied
	if dir := filepath.Dir(cfg.DBPath); dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("console: failed to open sqlite database at %s: %w", cfg.DBPath, err)
	}

	eng, err := engine.NewEngine(database, cfg.FlushInterval)
	if err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("console: failed to initialize engine: %w", err)
	}

	pricing, err := consoleapi.NewPricingEngine(database)
	if err != nil {
		eng.Stop()
		_ = database.Close()
		return nil, fmt.Errorf("console: failed to initialize pricing engine: %w", err)
	}

	providers, err := consoleapi.NewProviderRegistryEngine(database)
	if err != nil {
		eng.Stop()
		_ = database.Close()
		return nil, fmt.Errorf("console: failed to initialize provider registry engine: %w", err)
	}

	// Connect provider resolver on engine outbox flusher
	eng.SetProviderResolver(func(model, authIndex string) string {
		return providers.InferProvider(model, authIndex).Key
	})

	// Trigger one-time async backfill of historical upstream daily stats providers from baselines
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = database.BackfillUpstreamDailyStatsProviders(ctx)
		if cleaned, errClean := database.CleanPhantomEvents(ctx); errClean == nil && cleaned > 0 {
			logger.Infof("console: cleaned %d legacy phantom request events", cleaned)
		}
	}()

	// Start engine background flusher
	eng.Start()

	// Register native CPA access provider for zero-latency, in-memory request authentication
	sdkaccess.RegisterProvider(AccessProviderTypeConsole, NewConsoleAccessProvider(eng))

	sub := &Subsystem{
		cfg:       cfg,
		db:        database,
		engine:    eng,
		pricing:   pricing,
		providers: providers,
		logger:    logger,
	}

	globalSubsystem = sub
	logger.Infof("console subsystem initialized successfully (db: %s)", cfg.DBPath)
	return sub, nil
}

// GetSubsystem returns the active Console subsystem instance.
func GetSubsystem() *Subsystem {
	globalSubsystemMu.Lock()
	defer globalSubsystemMu.Unlock()
	return globalSubsystem
}

// Stop shuts down the subsystem and flushes pending data.
func (s *Subsystem) Stop() {
	if s == nil {
		return
	}
	globalSubsystemMu.Lock()
	defer globalSubsystemMu.Unlock()

	sdkaccess.UnregisterProvider(AccessProviderTypeConsole)

	if s.engine != nil {
		s.engine.Stop()
	}
	if s.db != nil {
		_ = s.db.Close()
	}
	if globalSubsystem == s {
		globalSubsystem = nil
	}
	s.logger.Info("console subsystem stopped")
}

// Engine returns the underlying Console engine.
func (s *Subsystem) Engine() *engine.Engine {
	return s.engine
}

// DB returns the underlying Console database.
func (s *Subsystem) DB() *db.DB {
	return s.db
}

// Pricing returns the PricingEngine.
func (s *Subsystem) Pricing() *consoleapi.PricingEngine {
	return s.pricing
}

// Providers returns the ProviderRegistryEngine.
func (s *Subsystem) Providers() *consoleapi.ProviderRegistryEngine {
	return s.providers
}

// HydrateCoreAuthManager populates in-memory credential throughput ring buffers and lifetime counters from SQLite.
func (s *Subsystem) HydrateCoreAuthManager(ctx context.Context, mgr *coreauth.Manager) error {
	if s == nil || s.db == nil || mgr == nil {
		return nil
	}

	// 1. Query rolling 200-minute window for recent 10-minute buckets
	now := time.Now()
	sinceTS := now.Add(-210 * time.Minute).Unix()
	bucketRows, err := s.db.SelectRecentAuthBuckets(ctx, sinceTS)
	if err != nil {
		s.logger.WithError(err).Warn("console: failed to query recent auth buckets for hydration")
	}

	// 2. Query lifetime daily stats and baselines
	dailyAgg, errDaily := s.db.SelectUpstreamDailyAgg(ctx)
	if errDaily != nil {
		s.logger.WithError(errDaily).Warn("console: failed to query daily agg for auth hydration")
	}

	baselines, errBase := s.db.SelectBaselinesAll(ctx)
	if errBase != nil {
		s.logger.WithError(errBase).Warn("console: failed to query baselines for auth hydration")
	}

	type authAccum struct {
		success int64
		failed  int64
		buckets []coreauth.AuthBucketStat
	}
	accumMap := make(map[string]*authAccum)
	getOrCreate := func(idx string) *authAccum {
		acc, ok := accumMap[idx]
		if !ok {
			acc = &authAccum{}
			accumMap[idx] = acc
		}
		return acc
	}

	for _, b := range baselines {
		if b.AuthIndex != "" {
			acc := getOrCreate(b.AuthIndex)
			acc.success += b.BaseSuccessCount
			acc.failed += b.BaseFailureCount
		}
	}

	for _, d := range dailyAgg {
		if d.AuthIndex != "" {
			acc := getOrCreate(d.AuthIndex)
			acc.success += d.SuccessCount
			acc.failed += d.FailureCount
		}
	}

	for _, b := range bucketRows {
		if b.AuthIndex != "" {
			acc := getOrCreate(b.AuthIndex)
			acc.buckets = append(acc.buckets, coreauth.AuthBucketStat{
				BucketID: b.BucketID,
				Success:  b.Success,
				Failed:   b.Failed,
			})
		}
	}

	if len(accumMap) == 0 {
		return nil
	}

	updates := make([]coreauth.AuthStatsUpdate, 0, len(accumMap))
	for authIndex, acc := range accumMap {
		updates = append(updates, coreauth.AuthStatsUpdate{
			AuthIndex: authIndex,
			Success:   acc.success,
			Failed:    acc.failed,
			Buckets:   acc.buckets,
		})
	}

	mgr.HydrateStats(updates)
	s.logger.Infof("console: hydrated stats for %d upstream credentials", len(updates))
	return nil
}

// ServerOptions produces the api.ServerOption slice for hook registration.
func ServerOptions(cfg SubsystemConfig) []api.ServerOption {
	if !cfg.Enabled {
		return nil
	}

	sub, err := InitSubsystem(cfg)
	if err != nil {
		logrus.WithError(err).Error("failed to initialize console subsystem for server options")
		return nil
	}

	// 1. Accounting and security middleware
	gatewayMW := middleware.Gateway(sub.engine)

	// Generate or resolve dedicated high-entropy loopback token for CPA local management bypass
	loopbackSecret := sub.cfg.CpaSecret
	if loopbackSecret == "" || strings.HasPrefix(loopbackSecret, "$2a$") || strings.HasPrefix(loopbackSecret, "$2b$") || strings.HasPrefix(loopbackSecret, "$2y$") {
		b := make([]byte, 24)
		if _, errRand := rand.Read(b); errRand == nil {
			loopbackSecret = hex.EncodeToString(b)
		} else {
			loopbackSecret = fmt.Sprintf("cpa_loopback_%d", time.Now().UnixNano())
		}
	}

	// 2. Router configurator
	routerConfigurator := func(r *gin.Engine, handlers *sdkhandlers.BaseAPIHandler, serverCfg *serverconfig.Config) {
		adminSecret := sub.cfg.AdminSecret
		if adminSecret == "" {
			adminSecret = os.Getenv("CONSOLE_ADMIN_SECRET")
		}
		if adminSecret == "" {
			adminSecret = os.Getenv("GATE_ADMIN_SECRET")
		}
		if adminSecret == "" {
			adminSecret = os.Getenv("ADMIN_SECRET")
		}

		var additionalSecrets []string
		cpaPort := sub.cfg.CpaPort
		cpaHost := sub.cfg.CpaHost

		if serverCfg != nil {
			if serverCfg.RemoteManagement.SecretKey != "" {
				if adminSecret == "" {
					adminSecret = serverCfg.RemoteManagement.SecretKey
				} else if serverCfg.RemoteManagement.SecretKey != adminSecret {
					additionalSecrets = append(additionalSecrets, serverCfg.RemoteManagement.SecretKey)
				}
			}
			if serverCfg.Port > 0 {
				cpaPort = serverCfg.Port
			}
			if serverCfg.Host != "" && serverCfg.Host != "0.0.0.0" {
				cpaHost = serverCfg.Host
			}
		}

		// Inherit CPA management secret and port from cliproxyapi server configuration if available
		routerCfg := consoleapi.RouterConfig{
			AdminSecret:       adminSecret,
			CpaHost:           cpaHost,
			CpaPort:           cpaPort,
			CpaSecret:         loopbackSecret,
			AdditionalSecrets: additionalSecrets,
		}

		if serverCfg != nil {
			// Reconcile and import API keys defined in CPA server configuration into Console database
			if len(serverCfg.APIKeys) > 0 {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				reconciled := 0
				for _, k := range serverCfg.APIKeys {
					trimmed := strings.TrimSpace(k)
					if trimmed == "" {
						continue
					}
					// Do not resurrect keys explicitly deleted by administrators
					if isDeleted, errDel := sub.db.IsKeyDeleted(ctx, trimmed); errDel == nil && isDeleted {
						continue
					}
					if _, exists := sub.engine.ResolveKeyID(trimmed); !exists {
						keyID, err := sub.db.InsertKey(ctx, trimmed, "CPA Config Key")
						if err == nil && keyID > 0 {
							rule := &db.QuotaRuleRow{
								KeyID:          keyID,
								Enabled:        0,
								HardRatio:      1.2,
								RPMLimit:       0,
								ModelWhitelist: "[]",
							}
							_ = sub.db.UpsertQuotaRule(ctx, rule)
							reconciled++
						}
					}
				}
				if reconciled > 0 {
					_ = sub.engine.ReloadState(ctx)
					sub.logger.Infof("console: reconciled %d keys from CPA configuration", reconciled)
				}
			}

			// Reconcile and proactively register OpenAI compatibility channels into ProviderRegistryEngine
			if len(serverCfg.OpenAICompatibility) > 0 {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				var compatList []consoleapi.CpaOpenAICompatProvider
				for _, c := range serverCfg.OpenAICompatibility {
					var keyEntries []consoleapi.CpaOpenAICompatKeyEntry
					for _, k := range c.APIKeyEntries {
						keyEntries = append(keyEntries, consoleapi.CpaOpenAICompatKeyEntry{
							APIKey: k.APIKey,
							Weight: k.Weight,
						})
					}
					var models []consoleapi.CpaOpenAICompatModel
					for _, m := range c.Models {
						models = append(models, consoleapi.CpaOpenAICompatModel{
							Name:  m.Name,
							Alias: m.Alias,
						})
					}
					compatList = append(compatList, consoleapi.CpaOpenAICompatProvider{
						Name:          c.Name,
						Disabled:      c.Disabled,
						BaseURL:       c.BaseURL,
						APIKeyEntries: keyEntries,
						Models:        models,
					})
				}
				consoleapi.SyncOpenAICompatConfig(ctx, sub.engine, sub.providers, compatList)
				sub.logger.Infof("console: reconciled %d custom channels from OpenAICompatibility configuration", len(compatList))
			}
		}

		// Register API, system, and member routes
		consoleapi.RegisterRoutes(r, sub.engine, routerCfg, sub.pricing, sub.providers)

		// Register static UI and backward compatibility assets
		assets.RegisterUI(r)

		// Connect persistent stats hydrator to core auth manager and perform initial background hydration
		if handlers != nil && handlers.AuthManager != nil {
			handlers.AuthManager.SetStatsHydrator(sub.HydrateCoreAuthManager)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				if errHydrate := sub.HydrateCoreAuthManager(ctx, handlers.AuthManager); errHydrate != nil {
					sub.logger.WithError(errHydrate).Warn("console: initial auth manager stats hydration failed")
				}
			}()
		}

		sub.logger.Info("console routes and static UI registered successfully")
	}

	return []api.ServerOption{
		api.WithLocalManagementPassword(loopbackSecret),
		api.WithMiddleware(gatewayMW...),
		api.WithRouterConfigurator(routerConfigurator),
	}
}

// Bootstrap is the automatic entry point called during init to wire Console globally.
func Bootstrap() {
	bootstrapOnce.Do(func() {
		cfg := DefaultConfig()
		if !cfg.Enabled {
			return
		}
		opts := ServerOptions(cfg)
		if len(opts) > 0 {
			api.RegisterGlobalServerOption(opts...)
		}
	})
}
