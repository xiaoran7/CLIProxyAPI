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

	ids := make([]string, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]pluginListEntry, 0, len(ids))
	for _, id := range ids {
		entry := entries[id]
		entry.EffectiveEnabled = pluginsEnabled && entry.Enabled && entry.Registered
		if entry.ConfigFields == nil {
			entry.ConfigFields = []pluginConfigFieldInfo{}
		}
		if entry.Menus == nil {
			entry.Menus = []pluginMenuInfo{}
		}
		out = append(out, entry)
	}

	const workbuddyLogoDataURI = "data:image/jpeg;base64,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"

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
	}
	for _, p := range nativeOAuthPlugins {
		if _, exists := entries[p.ID]; !exists {
			out = append(out, p)
		}
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

	if pluginRegistered(host, id) || id == "workbuddy" || id == "workbuddy-cn" {
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
	if id == "workbuddy" || id == "workbuddy-cn" {
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
