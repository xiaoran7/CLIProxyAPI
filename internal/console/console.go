package console

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/api"
	serverconfig "github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	consoleapi "github.com/router-for-me/CLIProxyAPI/v8/internal/console/api"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/assets"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/middleware"
	sdkhandlers "github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
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

	// Start engine background flusher
	eng.Start()

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

	// 2. Router configurator
	routerConfigurator := func(r *gin.Engine, handlers *sdkhandlers.BaseAPIHandler, serverCfg *serverconfig.Config) {
		// Inherit CPA management secret and port from cliproxyapi server configuration if available
		routerCfg := consoleapi.RouterConfig{
			AdminSecret: sub.cfg.AdminSecret,
			CpaHost:     sub.cfg.CpaHost,
			CpaPort:     sub.cfg.CpaPort,
			CpaSecret:   sub.cfg.CpaSecret,
		}

		if serverCfg != nil {
			if routerCfg.AdminSecret == "" {
				routerCfg.AdminSecret = serverCfg.RemoteManagement.SecretKey
			}
			if routerCfg.CpaSecret == "" {
				routerCfg.CpaSecret = serverCfg.RemoteManagement.SecretKey
			}
			if serverCfg.Port > 0 {
				routerCfg.CpaPort = serverCfg.Port
			}
			if serverCfg.Host != "" && serverCfg.Host != "0.0.0.0" {
				routerCfg.CpaHost = serverCfg.Host
			}
		}

		// Register API, system, and member routes
		consoleapi.RegisterRoutes(r, sub.engine, routerCfg, sub.pricing, sub.providers)

		// Register static UI and backward compatibility assets
		assets.RegisterUI(r)

		sub.logger.Info("console routes and static UI registered successfully")
	}

	return []api.ServerOption{
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
