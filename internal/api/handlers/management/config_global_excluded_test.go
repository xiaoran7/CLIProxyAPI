package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
)

func TestGlobalExcludedModelsManagement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := "port: 8317\napi-keys: [client]\n"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}

	h := &Handler{cfg: cfg, configFilePath: path}
	router := gin.New()
	router.GET("/v0/management/global-excluded-models", h.GetGlobalExcludedModels)
	router.PUT("/v0/management/global-excluded-models", h.PutGlobalExcludedModels)
	router.PATCH("/v0/management/global-excluded-models", h.PatchGlobalExcludedModels)
	router.DELETE("/v0/management/global-excluded-models", h.DeleteGlobalExcludedModels)
	router.POST("/v0/management/global-excluded-models/toggle", h.ToggleGlobalExcludedModel)

	request := func(method, url, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, url, strings.NewReader(body)))
		if rec.Code != status {
			t.Fatalf("%s %s: status=%d body=%s", method, url, rec.Code, rec.Body.String())
		}
		return rec
	}

	// 1. Initial GET
	rec := request(http.MethodGet, "/v0/management/global-excluded-models", "", 200)
	var getResp struct {
		Models []string `json:"global-excluded-models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &getResp); err != nil {
		t.Fatal(err)
	}
	if len(getResp.Models) != 0 {
		t.Fatalf("expected empty global-excluded-models, got %v", getResp.Models)
	}

	// 2. PUT list
	request(http.MethodPut, "/v0/management/global-excluded-models", `["deepseek-v4.1-flash-free", "*-free"]`, 200)
	rec = request(http.MethodGet, "/v0/management/global-excluded-models", "", 200)
	_ = json.Unmarshal(rec.Body.Bytes(), &getResp)
	if len(getResp.Models) != 2 {
		t.Fatalf("expected 2 excluded models, got %v", getResp.Models)
	}
	if !registry.GetGlobalRegistry().IsModelGloballyExcluded("deepseek-v4.1-flash-free") {
		t.Fatalf("expected registry to have deepseek-v4.1-flash-free excluded")
	}

	// 3. Toggle off (disable model -> exclude)
	request(http.MethodPost, "/v0/management/global-excluded-models/toggle", `{"model":"gpt-6-astra"}`, 200)
	rec = request(http.MethodGet, "/v0/management/global-excluded-models", "", 200)
	_ = json.Unmarshal(rec.Body.Bytes(), &getResp)
	if len(getResp.Models) != 3 {
		t.Fatalf("expected 3 excluded models, got %v", getResp.Models)
	}

	// 4. Toggle on (re-enable model -> unexclude)
	request(http.MethodPost, "/v0/management/global-excluded-models/toggle", `{"model":"gpt-6-astra"}`, 200)
	rec = request(http.MethodGet, "/v0/management/global-excluded-models", "", 200)
	_ = json.Unmarshal(rec.Body.Bytes(), &getResp)
	if len(getResp.Models) != 2 {
		t.Fatalf("expected 2 excluded models after toggle, got %v", getResp.Models)
	}
}
