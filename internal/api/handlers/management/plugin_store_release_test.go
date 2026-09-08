package management

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pluginstore"
)

func listPluginStoreForTest(t *testing.T, h *Handler) pluginStoreListResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/plugin-store", nil)
	h.ListPluginStore(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var body pluginStoreListResponse
	if errDecode := json.Unmarshal(rec.Body.Bytes(), &body); errDecode != nil {
		t.Fatal(errDecode)
	}
	return body
}

func TestListPluginStoreSkipsUninstalledReleases(t *testing.T) {
	t.Parallel()

	registry := pluginstore.Registry{SchemaVersion: pluginstore.SchemaVersion}
	for index := range 100 {
		version := ""
		if index%2 == 0 {
			version = "1.0.0"
		}
		registry.Plugins = append(registry.Plugins, pluginstore.Plugin{
			ID: fmt.Sprintf("plugin-%d", index), Name: "Plugin", Description: "Test plugin", Author: "test",
			Version: version, Repository: fmt.Sprintf("https://github.com/test/plugin-%d", index),
		})
	}
	data, errMarshal := json.Marshal(registry)
	if errMarshal != nil {
		t.Fatal(errMarshal)
	}
	httpClient := &countingPluginStoreHTTPClient{responses: fakePluginStoreHTTPClient{
		pluginstore.DefaultRegistryURL: data,
	}}
	h := &Handler{
		cfg: &config.Config{Plugins: config.PluginsConfig{
			Dir: t.TempDir(),
			// Configuration alone does not mean the plugin is installed.
			Configs: map[string]config.PluginInstanceConfig{"plugin-0": pluginConfigFromYAML(t, "enabled: true\n")},
		}},
		pluginStoreHTTPClient: httpClient,
	}
	for range 2 {
		body := listPluginStoreForTest(t, h)
		if len(body.Plugins) != 100 {
			t.Fatalf("plugins = %d, want 100", len(body.Plugins))
		}
		for index, entry := range body.Plugins {
			if entry.Version != registry.Plugins[index].Version || entry.Installed || entry.UpdateAvailable {
				t.Fatalf("unexpected catalog entry: %#v", entry)
			}
		}
	}
	httpClient.mu.Lock()
	defer httpClient.mu.Unlock()
	if len(httpClient.counts) != 1 || httpClient.counts[pluginstore.DefaultRegistryURL] != 2 {
		t.Fatalf("requests = %v, want registry requests only", httpClient.counts)
	}
}

func TestListPluginStoreSkipsInstalledDirectRelease(t *testing.T) {
	t.Parallel()

	httpClient := &countingPluginStoreHTTPClient{responses: fakePluginStoreHTTPClient{
		pluginstore.DefaultRegistryURL: directRegistryJSON("https://downloads.example/plugin.zip", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
	}}
	h := &Handler{
		cfg:                   &config.Config{Plugins: config.PluginsConfig{Dir: writeManagementPluginFile(t, "sample-provider")}},
		pluginStoreHTTPClient: httpClient,
	}
	body := listPluginStoreForTest(t, h)
	if len(body.Plugins) != 1 || !body.Plugins[0].Installed || body.Plugins[0].Version != "0.4.0" {
		t.Fatalf("unexpected direct catalog entry: %#v", body.Plugins)
	}
	httpClient.mu.Lock()
	defer httpClient.mu.Unlock()
	if len(httpClient.counts) != 1 {
		t.Fatalf("requests = %v, want registry request only", httpClient.counts)
	}
}
