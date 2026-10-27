package management

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/qoder"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestQoderManagement_GetAndPostCheckin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpDir, err := os.MkdirTemp("", "qoder_mgmt_checkin_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	engine := qoder.InitGlobalCheckinEngine(tmpDir, func() []*coreauth.Auth {
		return []*coreauth.Auth{
			{
				ID:       "qoder-test-1",
				Provider: "qoder",
				Metadata: map[string]any{
					"nickname": "TestUser",
				},
			},
		}
	})
	defer engine.Stop()

	h := &Handler{
		cfg: &config.Config{AuthDir: tmpDir},
	}

	router := gin.New()
	router.GET("/v8/management/qoder/checkin", h.GetQoderCheckin)
	router.POST("/v8/management/qoder/checkin", h.PostQoderCheckin)
	router.PUT("/v8/management/qoder/checkin", h.PutQoderCheckin)

	// 1. Initial GET checkin
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v8/management/qoder/checkin", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Enabled bool           `json:"enabled"`
		Today   string         `json:"today"`
		Records map[string]any `json:"records"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Today == "" {
		t.Fatalf("expected non-empty today date")
	}
	if !resp.Enabled {
		t.Fatalf("expected enabled by default")
	}

	// 2. Record checkin for auth in engine
	today := qoder.TodayBeijing()
	engine.RecordCheckin("qoder-test-1", qoder.CheckinResult{
		AuthID:  "qoder-test-1",
		User:    "user_123",
		Date:    today,
		Day:     today,
		Status:  qoder.CheckinClaimed,
		Outcome: qoder.CheckinClaimed,
		Points:  100,
		Credit:  100,
		Message: "checkin success",
		Realm:   qoder.RealmCN,
		Checked: time.Now(),
		At:      time.Now(),
	})

	// 3. GET checkin with auth_id
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/v8/management/qoder/checkin?auth_id=qoder-test-1", nil)
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var resp2 struct {
		Found  bool                 `json:"found"`
		Record *qoder.CheckinResult `json:"record"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !resp2.Found || resp2.Record == nil {
		t.Fatalf("expected found record, got %+v", resp2)
	}
	if resp2.Record.Points != 100 {
		t.Fatalf("expected 100 points, got %d", resp2.Record.Points)
	}

	// 4. POST checkin with nonexistent auth returns 404
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodPost, "/v8/management/qoder/checkin?auth_id=nonexistent", nil)
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent auth, got %d: %s", w3.Code, w3.Body.String())
	}

	// 5. POST checkin with enabled: false toggles engine
	w4 := httptest.NewRecorder()
	disableBody := []byte(`{"enabled": false}`)
	req4, _ := http.NewRequest(http.MethodPost, "/v8/management/qoder/checkin", bytes.NewReader(disableBody))
	req4.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w4, req4)
	if w4.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w4.Code, w4.Body.String())
	}
	if engine.IsEnabled() {
		t.Fatal("expected engine to be disabled after toggle")
	}

	// 6. PUT checkin with enabled: true re-enables engine
	w5 := httptest.NewRecorder()
	enableBody := []byte(`{"enabled": true}`)
	req5, _ := http.NewRequest(http.MethodPut, "/v8/management/qoder/checkin", bytes.NewReader(enableBody))
	req5.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w5, req5)
	if w5.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w5.Code, w5.Body.String())
	}
	if !engine.IsEnabled() {
		t.Fatal("expected engine to be re-enabled after PUT")
	}
}
