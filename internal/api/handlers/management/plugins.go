package management

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/htmlsanitize"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/pluginhost"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"gopkg.in/yaml.v3"
)

type pluginListResponse struct {
	PluginsEnabled bool              `json:"plugins_enabled"`
	PluginsDir     string            `json:"plugins_dir"`
	Plugins        []pluginListEntry `json:"plugins"`
}

type pluginListEntry struct {
	ID               string                  `json:"id"`
	Path             string                  `json:"path"`
	Configured       bool                    `json:"configured"`
	Registered       bool                    `json:"registered"`
	Enabled          bool                    `json:"enabled"`
	EffectiveEnabled bool                    `json:"effective_enabled"`
	SupportsOAuth    bool                    `json:"supports_oauth"`
	OAuthProvider    string                  `json:"oauth_provider"`
	SupportsQuota    bool                    `json:"supports_quota"`
	QuotaProvider    string                  `json:"quota_provider,omitempty"`
	Logo             string                  `json:"logo"`
	ConfigFields     []pluginConfigFieldInfo `json:"config_fields"`
	Menus            []pluginMenuInfo        `json:"menus"`
	Metadata         *pluginMetadataInfo     `json:"metadata"`
}

type pluginMetadataInfo struct {
	Name             string                  `json:"name"`
	Version          string                  `json:"version"`
	Author           string                  `json:"author"`
	GitHubRepository string                  `json:"github_repository"`
	Logo             string                  `json:"logo"`
	ConfigFields     []pluginConfigFieldInfo `json:"config_fields"`
}

type pluginConfigFieldInfo struct {
	Default     any      `json:"default"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	EnumValues  []string `json:"enum_values"`
	Description string   `json:"description"`
}

type pluginMenuInfo struct {
	Path        string `json:"path"`
	Menu        string `json:"menu"`
	Description string `json:"description"`
}

// ListPlugins returns discovered, configured, and registered plugin entries.
func (h *Handler) ListPlugins(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusOK, pluginListResponse{
			PluginsDir: "plugins",
			Plugins:    []pluginListEntry{},
		})
		return
	}

	h.mu.Lock()
	pluginsEnabled := h.cfg.Plugins.Enabled
	pluginsDir := normalizedPluginsDir(h.cfg.Plugins.Dir)
	configs := make(map[string]config.PluginInstanceConfig, len(h.cfg.Plugins.Configs))
	for id, item := range h.cfg.Plugins.Configs {
		configs[id] = item
	}
	host := h.pluginHost
	h.mu.Unlock()

	resolvedPluginsDir, errResolvePluginsDir := config.ResolvePluginsDir(pluginsDir)
	if errResolvePluginsDir != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "plugin_directory_invalid", "message": errResolvePluginsDir.Error()})
		return
	}
	pluginsDir = resolvedPluginsDir
	entries := make(map[string]pluginListEntry)
	files, errDiscover := pluginhost.DiscoverPluginFiles(pluginsDir, pluginStoreDesiredVersions(configs))
	if errDiscover != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "plugin_discovery_failed", "message": errDiscover.Error()})
		return
	}
	for _, file := range files {
		entries[file.ID] = pluginListEntry{
			ID:           htmlsanitize.String(file.ID),
			Path:         htmlsanitize.String(file.Path),
			Enabled:      false,
			ConfigFields: []pluginConfigFieldInfo{},
			Menus:        []pluginMenuInfo{},
		}
	}
	for id, item := range configs {
		entry := entries[id]
		entry.ID = htmlsanitize.String(id)
		entry.Configured = true
		entry.Enabled = pluginInstanceEnabled(item)
		if entry.ConfigFields == nil {
			entry.ConfigFields = []pluginConfigFieldInfo{}
		}
		if entry.Menus == nil {
			entry.Menus = []pluginMenuInfo{}
		}
		entries[id] = entry
	}
	if host != nil {
		for _, info := range host.RegisteredPlugins() {
			entry := entries[info.ID]
			entry.ID = htmlsanitize.String(info.ID)
			entry.Registered = true
			entry.SupportsOAuth = info.SupportsOAuth
			entry.OAuthProvider = htmlsanitize.String(info.OAuthProvider)
			entry.SupportsQuota = info.SupportsQuota
			entry.QuotaProvider = htmlsanitize.String(info.QuotaProvider)
			entry.Logo = htmlsanitize.String(info.Metadata.Logo)
			entry.ConfigFields = pluginConfigFields(info.Metadata.ConfigFields)
			entry.Menus = pluginMenus(info.Menus)
			entry.Metadata = pluginMetadata(info.Metadata)
			entries[info.ID] = entry
		}
	}

	const workbuddyLogoDataURI = "data:image/jpeg;base64,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"
	const qoderLogoDataURI = "data:image/png;base64,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"
	const zcodeLogoDataURI = "data:image/png;base64,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"
	const bigmodelLogoDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAFoAAABaCAYAAAA4qEECAAAACXBIWXMAACE4AAAhOAFFljFgAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAAOdEVYdFNvZnR3YXJlAEZpZ21hnrGWYwAABRxJREFUeAHtnaFTG0EUxh9MLQy1YK4KJMyAT1EoKAYDM23BgKIGFDOFwaFSBQamzIDBdFDgGoMCgQTVGNAM/AF0v8tee9lsQjbZt7m929/MJtcjaciXl7dvv+PuEQUCgYAxfdQBr6+vkbgriTEkRxF4EqOK0dfXd0uGtC20EBeCrovxRYyIik1VjJ9iHAvRq+08oS2hhcgQeJtk9N7f39P19TU9Pj7Sy8sLFYHh4WEaGBigqakpGh0dTXYjystC7J23nt9SaBnFv6iWJuj8/JxOT0/p7u6OigxEX1tbo7m5uWRXRYyvraK7qdBS5N9ijCNy19fX40gO/AeCHx0dxfdUSycTQuwn3WNbCQ2RSxB3eXm5MCnCFKSTw8NDGhsbwz8rQuiPusf163YKkb+LuxIiOYjcGmizsrKSpNOS0O6b7nENES1LN0RzNDMzE094gbdB+ri8vMQmUscHNYXoIvqTGBEmviBy+0Cr/f19bGJua4hqXUT/oRDNHYF8fXV1hc0nEdHv0z+ri2iZNiJMgEFkc5Cvb25usDkktfyHmjrGcYPFSKAzUmuMUnq/KnSEm1BldE4qE0Tp/arQQ8qDA4Y0C9J+CjghCO2IILQjgtCOeEeWgGWYsg3Z2dvbq7Nrl5aWaHp6mmyDUleu+LrCmtBY609OTpIL8MbTIsOM39zcJNug+oIVYQPvUkfKU4gZGRmh3d1d4gCvY6vU9U5o2LZpVldXE+PdKohkW9EMvBJajTDkZY55Qf3W2MAboWHWqCkDx+04wGE726tjL4SGyFtbW3X7cPgItqRt8GFyHBu1VnUgn0mL0CrPz88NbxyRzJGXOVJGgjWh8Uu6MKO4UgbMIHWitYlXk+Hg4GCcMjiwWcrp8EpozlLu5OSEOPFGaJRxKOdsw5mX03ghNGcpx50yErwQGktsjpSBdGFz9deKzAuNSOYwqxDFcABdkWmhOVMGZymnI9NC+1rK6cis0FyrP9UzcUUmhYaRz5EyEMWqZ+KKzAnti5FvSuaE9sXINyVTQvtk5JuSGaF9M/JNyYzQvhn5pmRCaB+NfFN6LrSvRr4pPRXaZyPflJ4K7bORb0rPhPbdyDelJ0Lnwcg3pSdC58HIN8W50Hkx8k1xKnSejHxTnApdlFJOhzOh82bkm+JE6Dwa+aawC51XI98UdqHzauSbwip0no18U9iEzruRb4q1v49WcWnk48JRHK+FD/Ph4YFswCK0KyM/mWi5zm/Ea9lKUdZTh0sjf2Njw9lJpN1iVWiXRj4+TI5TkrmwKrQrI59zouXCmtAujXyubw0nVoR2aeRzTbTcWKk6ZmdnWU5/wyUc0qs/fKDwTTjOZ9Rh8/1YEdpmGdQK1LRZ952bEa5A44ggtCOC0I4IQjtCFTq+5jGHQVMUmmmnFdrHOjUrpLSr69WiCl3BDWrVQGektKum99cJLdtbVNFnRDYHCBiABZXs0dLQfUg3GR7jZnFxkQJmwFSTHKs/0116Hpc2xuXnhxYWFgrf3KZdEM0XFxfYxDw3oTa/aYho2W0hbklULpdjjznQGsWH/6HrMNRWwxv0GcFFpAKNJCLL3GzW8EYyL8Yt/oOzs7P4qxGoBwUDtEkmQKpppsW4KdnBwYG1I8O+AmGVv1mpiDHfrE8WaLfN3ra4+0yyQUDSZg8HTIvSDwArPixGNG32doTA5beeb9I4MqJa1yH0Noyo2FSpVsKVW0Vxmm5aoaJnC+6L1AoVo9JuV85AIBDokr8WvnnavEglMwAAAABJRU5ErkJggg=="

	nativeOAuthPlugins := []pluginListEntry{
		{
			ID:               "workbuddy",
			Configured:       true,
			Registered:       true,
			Enabled:          true,
			EffectiveEnabled: true,
			SupportsOAuth:    true,
			OAuthProvider:    "workbuddy",
			SupportsQuota:    true,
			QuotaProvider:    "workbuddy",
			Logo:             workbuddyLogoDataURI,
			ConfigFields:     []pluginConfigFieldInfo{},
			Menus:            []pluginMenuInfo{},
			Metadata: &pluginMetadataInfo{
				Name:             "WorkBuddy",
				Author:           "Tencent",
				Version:          "1.0.0",
				GitHubRepository: "https://copilot.tencent.com",
			},
		},
		{
			ID:               "qoder",
			Configured:       true,
			Registered:       true,
			Enabled:          true,
			EffectiveEnabled: true,
			SupportsOAuth:    true,
			OAuthProvider:    "qoder",
			SupportsQuota:    true,
			QuotaProvider:    "qoder",
			Logo:             qoderLogoDataURI,
			ConfigFields:     []pluginConfigFieldInfo{},
			Menus:            []pluginMenuInfo{},
			Metadata: &pluginMetadataInfo{
				Name:             "Qoder",
				Author:           "Qoder",
				Version:          "1.0.0",
				GitHubRepository: "https://qoder.com",
			},
		},
		{
			ID:               "zcode",
			Configured:       true,
			Registered:       true,
			Enabled:          true,
			EffectiveEnabled: true,
			SupportsOAuth:    true,
			OAuthProvider:    "zcode",
			SupportsQuota:    true,
			QuotaProvider:    "zcode",
			Logo:             zcodeLogoDataURI,
			ConfigFields:     []pluginConfigFieldInfo{},
			Menus:            []pluginMenuInfo{},
			Metadata: &pluginMetadataInfo{
				Name:             "ZCode (Z.ai)",
				Author:           "Z.ai",
				Version:          "1.0.0",
				GitHubRepository: "https://zcode.z.ai",
				Logo:             zcodeLogoDataURI,
			},
		},
		{
			ID:               "zcode-cn",
			Configured:       true,
			Registered:       true,
			Enabled:          true,
			EffectiveEnabled: true,
			SupportsOAuth:    true,
			OAuthProvider:    "zcode-cn",
			SupportsQuota:    true,
			QuotaProvider:    "zcode-cn",
			Logo:             bigmodelLogoDataURI,
			ConfigFields:     []pluginConfigFieldInfo{},
			Menus:            []pluginMenuInfo{},
			Metadata: &pluginMetadataInfo{
				Name:             "BigModel (智谱)",
				Author:           "Zhipu AI",
				Version:          "1.0.0",
				GitHubRepository: "https://open.bigmodel.cn",
				Logo:             bigmodelLogoDataURI,
			},
		},
	}

	nativeMap := make(map[string]pluginListEntry, len(nativeOAuthPlugins))
	for _, p := range nativeOAuthPlugins {
		nativeMap[p.ID] = p
	}
	for id, p := range nativeMap {
		entry, exists := entries[id]
		if !exists {
			entry = p
		} else {
			entry.Registered = true
			entry.SupportsOAuth = p.SupportsOAuth
			entry.OAuthProvider = p.OAuthProvider
			entry.SupportsQuota = p.SupportsQuota
			entry.QuotaProvider = p.QuotaProvider
			if entry.Logo == "" {
				entry.Logo = p.Logo
			}
			if entry.Metadata == nil {
				entry.Metadata = p.Metadata
			}
		}
		entry.Configured = true
		if _, hasConfig := configs[id]; !hasConfig {
			entry.Enabled = true
		}
		entries[id] = entry
	}

	ids := make([]string, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]pluginListEntry, 0, len(ids))
	for _, id := range ids {
		entry := entries[id]
		if _, isNative := nativeMap[id]; isNative {
			entry.EffectiveEnabled = entry.Enabled
		} else {
			entry.EffectiveEnabled = pluginsEnabled && entry.Enabled && entry.Registered
		}
		if entry.ConfigFields == nil {
			entry.ConfigFields = []pluginConfigFieldInfo{}
		}
		if entry.Menus == nil {
			entry.Menus = []pluginMenuInfo{}
		}
		out = append(out, entry)
	}

	c.JSON(http.StatusOK, pluginListResponse{
		PluginsEnabled: pluginsEnabled,
		PluginsDir:     htmlsanitize.String(pluginsDir),
		Plugins:        out,
	})
}

// GetPluginConfig returns the preserved plugins.configs.<id> object as JSON.
func (h *Handler) GetPluginConfig(c *gin.Context) {
	id, okID := pluginIDFromRequest(c)
	if !okID {
		return
	}
	if h == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin_not_found", "message": "plugin not found"})
		return
	}

	h.mu.Lock()
	if h.cfg == nil {
		h.mu.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin_not_found", "message": "plugin not found"})
		return
	}
	item, configured := h.cfg.Plugins.Configs[id]
	pluginsDir := normalizedPluginsDir(h.cfg.Plugins.Dir)
	host := h.pluginHost
	h.mu.Unlock()

	if configured {
		body, errBody := pluginConfigJSONObject(item)
		if errBody != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "plugin_config_encode_failed", "message": errBody.Error()})
			return
		}
		c.JSON(http.StatusOK, body)
		return
	}

	if pluginRegistered(host, id) || id == "workbuddy" || id == "workbuddy-cn" || id == "qoder" || id == "qoder-cn" || id == "zcode" || id == "zcode-cn" {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	resolvedPluginsDir, errResolvePluginsDir := config.ResolvePluginsDir(pluginsDir)
	if errResolvePluginsDir != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "plugin_directory_invalid", "message": errResolvePluginsDir.Error()})
		return
	}
	discovered, errDiscover := pluginDiscovered(resolvedPluginsDir, id)
	if errDiscover != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "plugin_discovery_failed", "message": errDiscover.Error()})
		return
	}
	if discovered {
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "plugin_not_found", "message": "plugin not found"})
}

// PatchPluginEnabled updates plugins.configs.<id>.enabled without touching plugins.enabled.
func (h *Handler) PatchPluginEnabled(c *gin.Context) {
	id, okID := pluginIDFromRequest(c)
	if !okID {
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if errBindJSON := c.ShouldBindJSON(&body); errBindJSON != nil || body.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": "enabled is required"})
		return
	}

	h.mu.Lock()
	ensurePluginConfigMap(h.cfg)
	item := h.cfg.Plugins.Configs[id]
	node := pluginConfigNode(item)
	setYAMLMappingValue(node, "enabled", boolYAMLNode(*body.Enabled))
	updated, errConfig := pluginInstanceConfigFromNode(node)
	if errConfig != nil {
		h.mu.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_config", "message": errConfig.Error()})
		return
	}
	h.cfg.Plugins.Configs[id] = updated
	cfgSnapshot, okSnapshot := h.saveConfigAndSnapshotLocked(c)
	h.mu.Unlock()
	if !okSnapshot {
		return
	}

	h.reloadConfigAfterManagementSaveAsync(c.Request.Context(), cfgSnapshot)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// PutPluginConfig replaces plugins.configs.<id> with the request object.
func (h *Handler) PutPluginConfig(c *gin.Context) {
	id, okID := pluginIDFromRequest(c)
	if !okID {
		return
	}
	body, okBody := readPluginConfigObject(c)
	if !okBody {
		return
	}
	node, errNode := yamlNodeFromJSONObject(body)
	if errNode != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": errNode.Error()})
		return
	}
	updated, errConfig := pluginInstanceConfigFromNode(node)
	if errConfig != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_config", "message": errConfig.Error()})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	ensurePluginConfigMap(h.cfg)
	h.cfg.Plugins.Configs[id] = updated
	h.persistLocked(c)
}

// PatchPluginConfig shallow-merges plugins.configs.<id> with the request object.
func (h *Handler) PatchPluginConfig(c *gin.Context) {
	id, okID := pluginIDFromRequest(c)
	if !okID {
		return
	}
	body, okBody := readPluginConfigObject(c)
	if !okBody {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	ensurePluginConfigMap(h.cfg)
	node := pluginConfigNode(h.cfg.Plugins.Configs[id])
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := body[key]
		if value == nil {
			deleteYAMLMappingKey(node, key)
			continue
		}
		valueNode, errNode := yamlNodeFromJSONValue(value)
		if errNode != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": errNode.Error()})
			return
		}
		setYAMLMappingValue(node, key, valueNode)
	}
	updated, errConfig := pluginInstanceConfigFromNode(node)
	if errConfig != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_config", "message": errConfig.Error()})
		return
	}
	h.cfg.Plugins.Configs[id] = updated
	h.persistLocked(c)
}

// DeletePlugin removes the selected local plugin file and its saved config.
func (h *Handler) DeletePlugin(c *gin.Context) {
	id, okID := pluginIDFromRequest(c)
	if !okID {
		return
	}
	if id == "workbuddy" || id == "workbuddy-cn" || id == "qoder" || id == "qoder-cn" || id == "zcode" || id == "zcode-cn" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot_delete_native_plugin", "message": "native suite plugin cannot be deleted"})
		return
	}
	if h == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin_not_found", "message": "plugin not found"})
		return
	}

	h.mu.Lock()
	if h.cfg == nil {
		h.mu.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin_not_found", "message": "plugin not found"})
		return
	}
	pluginsDir := normalizedPluginsDir(h.cfg.Plugins.Dir)
	item, configured := h.cfg.Plugins.Configs[id]
	host := h.pluginHost
	h.mu.Unlock()

	resolvedPluginsDir, errResolvePluginsDir := config.ResolvePluginsDir(pluginsDir)
	if errResolvePluginsDir != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "plugin_directory_invalid", "message": errResolvePluginsDir.Error()})
		return
	}
	pluginsDir = resolvedPluginsDir
	var desiredVersions map[string]string
	if configured {
		desiredVersions = pluginStoreDesiredVersions(map[string]config.PluginInstanceConfig{id: item})
	}
	path, errPath := pluginFilePath(pluginsDir, id, desiredVersions)
	if errPath != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "plugin_discovery_failed", "message": errPath.Error()})
		return
	}
	if path == "" && !configured {
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin_not_found", "message": "plugin not found"})
		return
	}

	if pluginBusy(host, id) && (host == nil || !host.UnloadPlugin(id)) && pluginBusy(host, id) {
		c.JSON(http.StatusConflict, gin.H{
			"error":            "plugin_delete_requires_restart",
			"message":          "loaded plugin cannot be deleted while the server is running",
			"restart_required": true,
		})
		return
	}

	fileDeleted := false
	if path != "" {
		if errRemove := os.Remove(path); errRemove != nil {
			if !errors.Is(errRemove, os.ErrNotExist) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "plugin_delete_failed", "message": errRemove.Error()})
				return
			}
		} else {
			fileDeleted = true
		}
	}

	h.mu.Lock()
	delete(h.cfg.Plugins.Configs, id)
	if configured {
		if errSave := config.SaveConfigPreserveComments(h.configFilePath, h.cfg, c.GetBool(ConfigV8ContextKey)); errSave != nil {
			h.mu.Unlock()
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":        "config_save_failed",
				"message":      fmt.Sprintf("plugin deleted but saving config failed: %s", errSave.Error()),
				"file_deleted": fileDeleted,
				"path":         path,
			})
			return
		}
	}
	cfgSnapshot := h.reloadSnapshotConfigLocked()
	h.mu.Unlock()

	h.reloadConfigAfterManagementSaveAsync(c.Request.Context(), cfgSnapshot)
	c.JSON(http.StatusOK, gin.H{
		"status":             "deleted",
		"id":                 htmlsanitize.String(id),
		"path":               htmlsanitize.String(path),
		"file_deleted":       fileDeleted,
		"configured_removed": configured,
		"restart_required":   false,
	})
}

func normalizedPluginsDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "plugins"
	}
	return dir
}

func pluginInstanceEnabled(item config.PluginInstanceConfig) bool {
	if item.Enabled == nil {
		return false
	}
	return *item.Enabled
}

func pluginRegistered(host *pluginhost.Host, id string) bool {
	if host == nil {
		return false
	}
	for _, info := range host.RegisteredPlugins() {
		if info.ID == id {
			return true
		}
	}
	return false
}

func pluginDiscovered(pluginsDir string, id string) (bool, error) {
	files, errDiscover := pluginhost.DiscoverPluginFiles(pluginsDir)
	if errDiscover != nil {
		return false, errDiscover
	}
	for _, file := range files {
		if file.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func pluginFilePath(pluginsDir string, id string, desiredVersions ...map[string]string) (string, error) {
	files, errDiscover := pluginhost.DiscoverPluginFiles(pluginsDir, desiredVersions...)
	if errDiscover != nil {
		return "", errDiscover
	}
	for _, file := range files {
		if file.ID == id {
			return file.Path, nil
		}
	}
	return "", nil
}

func pluginConfigFields(fields []pluginapi.ConfigField) []pluginConfigFieldInfo {
	out := make([]pluginConfigFieldInfo, 0, len(fields))
	for _, field := range fields {
		out = append(out, pluginConfigFieldInfo{
			Name:        htmlsanitize.String(field.Name),
			Type:        htmlsanitize.String(string(field.Type)),
			Default:     htmlsanitize.JSONValue(field.Default),
			EnumValues:  htmlsanitize.Strings(field.EnumValues),
			Description: htmlsanitize.String(field.Description),
		})
	}
	return out
}

func pluginMenus(menus []pluginhost.RegisteredPluginMenu) []pluginMenuInfo {
	out := make([]pluginMenuInfo, 0, len(menus))
	for _, menu := range menus {
		out = append(out, pluginMenuInfo{
			Path:        htmlsanitize.String(menu.Path),
			Menu:        htmlsanitize.String(menu.Menu),
			Description: htmlsanitize.String(menu.Description),
		})
	}
	return out
}

func pluginMetadata(meta pluginapi.Metadata) *pluginMetadataInfo {
	return &pluginMetadataInfo{
		Name:             htmlsanitize.String(meta.Name),
		Version:          htmlsanitize.String(meta.Version),
		Author:           htmlsanitize.String(meta.Author),
		GitHubRepository: htmlsanitize.String(meta.GitHubRepository),
		Logo:             htmlsanitize.String(meta.Logo),
		ConfigFields:     pluginConfigFields(meta.ConfigFields),
	}
}

func pluginIDFromRequest(c *gin.Context) (string, bool) {
	id := strings.TrimSpace(c.Param("id"))
	if !pluginhost.ValidatePluginID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_plugin_id", "message": "invalid plugin id"})
		return "", false
	}
	return id, true
}

func readPluginConfigObject(c *gin.Context) (map[string]any, bool) {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.UseNumber()
	var body map[string]any
	if errDecode := decoder.Decode(&body); errDecode != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": errDecode.Error()})
		return nil, false
	}
	if body == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": "body must be a JSON object"})
		return nil, false
	}
	return body, true
}

func ensurePluginConfigMap(cfg *config.Config) {
	if cfg == nil {
		return
	}
	cfg.NormalizePluginsConfig()
}

func pluginConfigNode(item config.PluginInstanceConfig) *yaml.Node {
	if item.Raw.Kind == yaml.MappingNode {
		return cloneYAMLNode(&item.Raw)
	}
	node := emptyYAMLMappingNode()
	if item.Enabled != nil {
		setYAMLMappingValue(node, "enabled", boolYAMLNode(*item.Enabled))
	}
	if item.Priority != 0 {
		setYAMLMappingValue(node, "priority", intYAMLNode(item.Priority))
	}
	return node
}

func pluginConfigJSONObject(item config.PluginInstanceConfig) (map[string]any, error) {
	value, errValue := yamlNodeToJSONValue(pluginConfigNode(item))
	if errValue != nil {
		return nil, errValue
	}
	body, ok := value.(map[string]any)
	if !ok || body == nil {
		return map[string]any{}, nil
	}
	return body, nil
}

func pluginInstanceConfigFromNode(node *yaml.Node) (config.PluginInstanceConfig, error) {
	if node == nil {
		node = emptyYAMLMappingNode()
	}
	var item config.PluginInstanceConfig
	if errDecode := node.Decode(&item); errDecode != nil {
		return config.PluginInstanceConfig{}, errDecode
	}
	return item, nil
}

func yamlNodeFromJSONObject(body map[string]any) (*yaml.Node, error) {
	node := emptyYAMLMappingNode()
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		valueNode, errNode := yamlNodeFromJSONValue(body[key])
		if errNode != nil {
			return nil, fmt.Errorf("%s: %w", key, errNode)
		}
		setYAMLMappingValue(node, key, valueNode)
	}
	return node, nil
}

func yamlNodeFromJSONValue(value any) (*yaml.Node, error) {
	switch typed := value.(type) {
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: typed}, nil
	case bool:
		return boolYAMLNode(typed), nil
	case json.Number:
		if _, errInt64 := typed.Int64(); errInt64 == nil {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: typed.String()}, nil
		}
		if _, errFloat64 := typed.Float64(); errFloat64 == nil {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: typed.String()}, nil
		}
		return nil, fmt.Errorf("invalid number %q", typed.String())
	case float64:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: strconv.FormatFloat(typed, 'f', -1, 64)}, nil
	case []any:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range typed {
			child, errChild := yamlNodeFromJSONValue(item)
			if errChild != nil {
				return nil, errChild
			}
			node.Content = append(node.Content, child)
		}
		return node, nil
	case map[string]any:
		return yamlNodeFromJSONObject(typed)
	default:
		return nil, fmt.Errorf("unsupported value type %T", value)
	}
}

func yamlNodeToJSONValue(node *yaml.Node) (any, error) {
	if node == nil {
		return nil, nil
	}
	switch node.Kind {
	case yaml.MappingNode:
		out := make(map[string]any, len(node.Content)/2)
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index]
			value := node.Content[index+1]
			if key == nil {
				continue
			}
			child, errChild := yamlNodeToJSONValue(value)
			if errChild != nil {
				return nil, fmt.Errorf("%s: %w", key.Value, errChild)
			}
			out[key.Value] = child
		}
		return out, nil
	case yaml.SequenceNode:
		out := make([]any, 0, len(node.Content))
		for _, childNode := range node.Content {
			child, errChild := yamlNodeToJSONValue(childNode)
			if errChild != nil {
				return nil, errChild
			}
			out = append(out, child)
		}
		return out, nil
	case yaml.ScalarNode:
		if node.Tag == "!!str" || node.Tag == "" {
			return node.Value, nil
		}
		var value any
		if errDecode := node.Decode(&value); errDecode != nil {
			return nil, errDecode
		}
		return value, nil
	case yaml.AliasNode:
		return yamlNodeToJSONValue(node.Alias)
	default:
		return nil, fmt.Errorf("unsupported YAML node kind %d", node.Kind)
	}
}

func emptyYAMLMappingNode() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

func boolYAMLNode(value bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(value)}
}

func intYAMLNode(value int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(value)}
}

func setYAMLMappingValue(mapping *yaml.Node, key string, value *yaml.Node) {
	if mapping.Kind != yaml.MappingNode {
		*mapping = *emptyYAMLMappingNode()
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index] != nil && mapping.Content[index].Value == key {
			mapping.Content[index+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

func deleteYAMLMappingKey(mapping *yaml.Node, key string) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index] != nil && mapping.Content[index].Value == key {
			mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
			return
		}
	}
}

func cloneYAMLNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	out := *node
	if len(node.Content) > 0 {
		out.Content = make([]*yaml.Node, 0, len(node.Content))
		for _, child := range node.Content {
			out.Content = append(out.Content, cloneYAMLNode(child))
		}
	}
	return &out
}
