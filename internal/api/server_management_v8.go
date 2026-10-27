package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/api/handlers/management"
)

// registerManagementV8Routes defines the v8 contract independently of v0.
// Configuration paths mirror the v8 YAML tree; operational routes use its groups.
func (s *Server) registerManagementV8Routes() {
	const prefix = "/v8/management"
	s.engine.GET(prefix+"/oauth/callback", s.managementAvailabilityMiddleware(), s.mgmt.GetOAuthCallbackV8)
	s.engine.POST(prefix+"/oauth/callback", s.managementAvailabilityMiddleware(), s.mgmt.PostOAuthCallbackV8)

	v8 := s.engine.Group(prefix)
	v8.Use(s.managementAvailabilityMiddleware(), s.mgmt.Middleware(), func(c *gin.Context) {
		c.Set(management.ConfigV8ContextKey, true)
	})
	v8.GET("/config", s.mgmt.ConfigV8)
	v8.PUT("/config", s.mgmt.ConfigV8)
	v8.PATCH("/config", s.mgmt.ConfigV8)
	v8.GET("/config.yaml", s.mgmt.ConfigV8)
	v8.PUT("/config.yaml", s.mgmt.ConfigV8)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		v8.Handle(method, "/config/*path", s.mgmt.ConfigV8)
	}

	v8.GET("/server/latest-version", s.mgmt.GetLatestVersion)
	v8.POST("/requests/api-call", s.mgmt.APICall)
	v8.POST("/routing/cooldown/reset", s.mgmt.ResetQuota)
	v8.GET("/routing/model-definitions/:channel", s.mgmt.GetStaticModelDefinitions)
	v8.POST("/routing/global-excluded-models/toggle", s.mgmt.ToggleGlobalExcludedModel)
	v8.POST("/models/global-excluded-models/toggle", s.mgmt.ToggleGlobalExcludedModel)
	v8.GET("/models/global-excluded-models", s.mgmt.GetGlobalExcludedModels)
	v8.PUT("/models/global-excluded-models", s.mgmt.PutGlobalExcludedModels)
	v8.DELETE("/models/global-excluded-models", s.mgmt.DeleteGlobalExcludedModels)
	v8.GET("/models/exit-catalog", s.mgmt.GetExitModels)

	v8.GET("/observability/logs", s.mgmt.GetLogs)
	v8.DELETE("/observability/logs", s.mgmt.DeleteLogs)
	v8.GET("/observability/logs/errors", s.mgmt.GetRequestErrorLogs)
	v8.GET("/observability/logs/errors/:name", s.mgmt.DownloadRequestErrorLog)
	v8.GET("/observability/logs/requests/:id", s.mgmt.GetRequestLogByID)
	v8.GET("/observability/usage/api-keys", s.mgmt.GetAPIKeyUsage)
	v8.GET("/observability/usage/queue", s.mgmt.GetUsageQueue)

	v8.GET("/credentials", s.mgmt.ListAuthFiles)
	v8.POST("/credentials", s.mgmt.UploadAuthFileV8)
	v8.DELETE("/credentials", s.mgmt.DeleteAuthFileV8)
	v8.GET("/credentials/models", s.mgmt.GetAuthFileModels)
	v8.GET("/credentials/download", s.mgmt.DownloadAuthFile)
	v8.PATCH("/credentials/status", s.mgmt.PatchAuthFileStatus)
	v8.PATCH("/credentials/fields", s.mgmt.PatchAuthFileFields)
	v8.POST("/credentials/refresh", s.mgmt.RefreshAuthFiles)
	v8.POST("/oauth/import", s.mgmt.ImportOAuthV8)
	v8.GET("/oauth/auth-url", s.mgmt.StartOAuthV8)
	v8.POST("/oauth/siwc/bind", s.mgmt.StartSIWCBind)
	v8.GET("/oauth/siwc/status", s.mgmt.GetSIWCStatus)
	v8.GET("/credentials/siwc/status", s.mgmt.GetSIWCStatus)
	v8.GET("/oauth/status", s.mgmt.GetAuthStatus)
	v8.DELETE("/oauth/session", s.mgmt.CancelAuthSession)
	v8.GET("/workbuddy/checkin", s.mgmt.GetWorkBuddyCheckin)
	v8.POST("/workbuddy/checkin", s.mgmt.PostWorkBuddyCheckin)
	v8.PUT("/workbuddy/checkin", s.mgmt.PutWorkBuddyCheckin)
	v8.GET("/workbuddy/compensation", s.mgmt.GetWorkBuddyCompensation)
	v8.POST("/workbuddy/compensation", s.mgmt.PostWorkBuddyCompensation)
	v8.GET("/qoder/checkin", s.mgmt.GetQoderCheckin)
	v8.POST("/qoder/checkin", s.mgmt.PostQoderCheckin)
	v8.PUT("/qoder/checkin", s.mgmt.PutQoderCheckin)

	v8.GET("/plugins", s.mgmt.ListPlugins)
	v8.DELETE("/plugins/:id", s.mgmt.DeletePlugin)
	v8.GET("/plugins/store", s.mgmt.ListPluginStore)
	v8.POST("/plugins/store/:id/install", s.mgmt.InstallPluginFromStore)
	v8.GET("/plugins/:id/quota", s.mgmt.GetPluginQuota)
	v8.POST("/plugins/:id/quota", s.mgmt.FetchPluginQuota)
	v8.DELETE("/plugins/:id/quota", s.mgmt.ResetPluginQuota)
}
