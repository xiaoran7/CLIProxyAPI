package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// RouterConfig defines the parameters required to initialize the Console routes.
type RouterConfig struct {
	AdminSecret string
	CpaHost     string
	CpaPort     int
	CpaSecret   string
}

// RegisterRoutes sets up all Console, Member, System, and CPA management routes on the router.
func RegisterRoutes(r *gin.Engine, eng *engine.Engine, cfg RouterConfig, pricing *PricingEngine, providers *ProviderRegistryEngine) {
	cpaClient := NewCpaClient(cfg.CpaHost, cfg.CpaPort, cfg.CpaSecret)

	// 1. System endpoints (unauthenticated)
	registerSystemRoutes(r, eng)

	// 2. Member endpoints (authenticated via API key)
	registerMemberRoutes(r.Group("/console/api/member"), eng, pricing, providers)
	// Compatibility alias for member portal
	registerMemberRoutes(r.Group("/api/member"), eng, pricing, providers)

	// 3. Console management endpoints (authenticated via management secret)
	registerConsoleRoutes(r.Group("/console/api"), eng, cfg.AdminSecret, cfg.CpaSecret, cpaClient, pricing, providers)
	// Compatibility alias for legacy admin frontend
	registerConsoleRoutes(r.Group("/admin/api"), eng, cfg.AdminSecret, cfg.CpaSecret, cpaClient, pricing, providers)

	// 4. Upstream management proxy routes (/admin/upstream and /console/upstream)
	RegisterUpstreamProxyRoutes(r, cfg)
}

func registerSystemRoutes(r *gin.Engine, eng *engine.Engine) {
	healthz := HandleHealthz()
	readyz := HandleReadyz(eng)
	metrics := HandleMetrics(eng)
	statusQuota := HandleStatusQuota(eng)

	hasRoute := func(method, path string) bool {
		for _, rt := range r.Routes() {
			if rt.Method == method && rt.Path == path {
				return true
			}
		}
		return false
	}

	if !hasRoute(http.MethodGet, "/healthz") {
		r.GET("/healthz", healthz)
	}
	if !hasRoute(http.MethodGet, "/readyz") {
		r.GET("/readyz", readyz)
	}
	if !hasRoute(http.MethodGet, "/metrics") {
		r.GET("/metrics", metrics)
	}
	if !hasRoute(http.MethodGet, "/api/status/quota") {
		r.GET("/api/status/quota", statusQuota)
	}

	// Console prefixed system routes
	if !hasRoute(http.MethodGet, "/console/healthz") {
		r.GET("/console/healthz", healthz)
	}
	if !hasRoute(http.MethodGet, "/console/readyz") {
		r.GET("/console/readyz", readyz)
	}
	if !hasRoute(http.MethodGet, "/console/metrics") {
		r.GET("/console/metrics", metrics)
	}
}

func registerMemberRoutes(rg *gin.RouterGroup, eng *engine.Engine, pricing *PricingEngine, providers *ProviderRegistryEngine) {
	rg.Use(MemberAuthMiddleware(eng))
	{
		rg.GET("/info", HandleMemberInfo(eng, pricing))
		rg.GET("/status-summary", HandleMemberStatusSummary(eng, providers))
		rg.GET("/requests", HandleMemberRequests(eng, pricing, providers))
	}
}

func registerConsoleRoutes(
	rg *gin.RouterGroup,
	eng *engine.Engine,
	adminSecret, cpaSecret string,
	cpa *CpaClient,
	pricing *PricingEngine,
	providers *ProviderRegistryEngine,
) {
	rg.Use(AdminAuthMiddleware(adminSecret, cpaSecret))
	{
		// Keys management
		rg.GET("/keys", HandleGetKeys(eng))
		rg.POST("/keys", HandleCreateKey(eng))
		rg.GET("/keys/:id/reveal", HandleRevealKey(eng))
		rg.POST("/keys/:id/rotate", HandleRotateKey(eng))
		rg.PUT("/keys/:id/alias", HandleUpdateKeyAlias(eng))
		rg.DELETE("/keys/:id", HandleDeleteKey(eng))
		rg.PUT("/keys/:id/quota", HandleUpdateQuotaRule(eng))
		rg.DELETE("/keys/:id/quota", HandleDeleteQuotaRule(eng))

		// Usage stats and request logs
		rg.GET("/stats", HandleGetStats(eng, pricing, providers))
		rg.GET("/requests", HandleGetRequests(eng, pricing, providers))
		rg.GET("/errors", HandleGetErrors(eng, providers))
		rg.GET("/status-summary", HandleGetStatusSummary(eng, providers))

		// Pricing matrix
		rg.GET("/model-prices", HandleGetModelPrices(eng))
		rg.POST("/model-prices", HandleUpsertModelPrice(eng, pricing))
		rg.DELETE("/model-prices/:id", HandleDeleteModelPrice(eng, pricing))

		// Provider registry
		rg.GET("/provider-registry", HandleGetProviderRegistry(eng))
		rg.POST("/provider-registry", HandleUpsertProviderRegistry(eng, providers))
		rg.DELETE("/provider-registry/:key", HandleDeleteProviderRegistry(eng, providers))

		// Upstream quotas & validations
		rg.GET("/upstream-quotas", HandleGetUpstreamQuotas(eng, cpa, providers))
		rg.POST("/upstream-quotas", HandleGetUpstreamQuotas(eng, cpa, providers))
		rg.GET("/upstream-routing", HandleGetUpstreamRouting(cpa))
		rg.PUT("/upstream-routing", HandlePutUpstreamRouting(cpa))
		rg.PUT("/upstream-auth/batch", HandlePutUpstreamAuthBatch(cpa))
		rg.PUT("/upstream-auth/priority", HandlePutUpstreamAuthPriority(cpa))
		rg.PUT("/upstream-auth/status", HandlePutUpstreamAuthStatus(cpa))
		rg.POST("/upstream-auth/activate-window", HandlePostUpstreamAuthActivateWindow(eng, cpa))
		rg.GET("/runtime-facts", HandleGetRuntimeFacts(eng, cpa, providers))
		rg.GET("/models", HandleGetCpaModels(eng, cpa, providers))
		rg.GET("/models/catalog", HandleGetCpaModels(eng, cpa, providers))
		rg.GET("/openrouter/accounts", HandleGetOpenRouterAccounts(cpa))
		rg.POST("/openrouter/accounts", HandleCreateOpenRouterAccount(cpa))
		rg.PATCH("/openrouter/accounts/:id", HandleUpdateOpenRouterAccount(cpa))
		rg.PUT("/openrouter/accounts/:id", HandleUpdateOpenRouterAccount(cpa))
		rg.DELETE("/openrouter/accounts/:id", HandleDeleteOpenRouterAccount(cpa))
		rg.GET("/quota-snapshots", HandleGetQuotaSnapshots(eng))
		rg.GET("/upstream-auth-stats", HandleGetUpstreamAuthStats(eng, providers))
		rg.GET("/upstream-validations", HandleGetValidations(eng))
		rg.POST("/upstream-validations/:id/resolve", HandleResolveValidation(eng))
		rg.POST("/upstream-validations/:id/dismiss", HandleDismissValidation(eng))
		rg.DELETE("/upstream-validations/:id", HandleDeleteValidation(eng))
		rg.POST("/upstream-validations/manual", HandleManualValidation(eng))

		// Direct runtime config snapshot
		rg.GET("/runtime-config", func(c *gin.Context) {
			ctx := c.Request.Context()
			var configData interface{}
			var routingData interface{}
			var versionData interface{}

			_ = cpa.RequestJSON(ctx, http.MethodGet, "/config", nil, "v0", &configData)
			_ = cpa.RequestJSON(ctx, http.MethodGet, "/routing", nil, "v0", &routingData)
			_ = cpa.RequestJSON(ctx, http.MethodGet, "/latest-version", nil, "v0", &versionData)

			c.JSON(http.StatusOK, gin.H{
				"config":  configData,
				"routing": routingData,
				"version": versionData,
			})
		})

		// CPA loopback proxy routes
		RegisterCpaRoutes(rg.Group("/cpa"), cpa, eng, providers)
	}
}
