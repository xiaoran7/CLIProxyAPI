package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/qoder"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
)

func TestQoderManagement_GetAndPostCheckin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg: &config.Config{},
	}

	router := gin.New()
	router.GET("/v8/management/qoder/checkin", h.GetQoderCheckin)
	router.POST("/v8/management/qoder/checkin", h.PostQoderCheckin)

	// 1. Initial GET checkin
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v8/management/qoder/checkin", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Today   string         `json:"today"`
		Records map[string]any `json:"records"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Today == "" {
		t.Fatalf("expected non-empty today date")
	}

	// 2. Record checkin for auth
	today := qoder.TodayBeijing()
	RecordQoderCheckin("qoder-test-1", &qoder.CheckinResult{
		AuthID:  "qoder-test-1",
		User:    "user_123",
		Date:    today,
		Status:  qoder.CheckinClaimed,
		Points:  10,
		Message: "checkin success",
		Realm:   qoder.RealmCN,
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
	if resp2.Record.Points != 10 {
		t.Fatalf("expected 10 points, got %d", resp2.Record.Points)
	}

	// 4. POST checkin for missing auth returns 404
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodPost, "/v8/management/qoder/checkin?auth_id=nonexistent", nil)
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent auth, got %d: %s", w3.Code, w3.Body.String())
	}
}
