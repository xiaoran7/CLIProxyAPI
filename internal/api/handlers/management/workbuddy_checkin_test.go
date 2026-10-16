package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/workbuddy"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestWorkBuddyManagement_GetAndPostCompensation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup fake auth manager
	fakeAuth := &cliproxyauth.Auth{
		ID:       "wb-mgmt-test-cn",
		Provider: "workbuddy-cn",
		Attributes: map[string]string{
			"realm": workbuddy.RealmCN,
		},
		Metadata: map[string]any{
			"access_token": "token-123",
			"nickname":     "TestDev",
		},
	}

	tmpDir := t.TempDir()
	engine := workbuddy.InitGlobalCheckinEngine(tmpDir, func() []*cliproxyauth.Auth {
		return []*cliproxyauth.Auth{fakeAuth}
	})
	defer engine.Stop()

	h := &Handler{
		cfg: &config.Config{},
	}

	router := gin.New()
	router.GET("/v8/management/workbuddy/checkin", h.GetWorkBuddyCheckin)
	router.POST("/v8/management/workbuddy/checkin", h.PostWorkBuddyCheckin)
	router.GET("/v8/management/workbuddy/compensation", h.GetWorkBuddyCompensation)
	router.POST("/v8/management/workbuddy/compensation", h.PostWorkBuddyCompensation)

	// 1. Query initial compensation status (should return 200 with compensations map)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v8/management/workbuddy/compensation", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var getResp struct {
		Compensations map[string]any `json:"compensations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	// 2. Pre-record a claimed compensation into engine and query single auth_id
	engine.RecordCompensation("wb-mgmt-test-cn", workbuddy.CompensationResult{
		AuthID:  "wb-mgmt-test-cn",
		Outcome: workbuddy.CompensationClaimed,
		Credit:  500.0,
		Message: "500 credits claimed",
	})

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/v8/management/workbuddy/compensation?auth_id=wb-mgmt-test-cn", nil)
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var singleResp struct {
		Found  bool                         `json:"found"`
		Record workbuddy.CompensationResult `json:"record"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &singleResp); err != nil {
		t.Fatalf("unmarshal single resp failed: %v", err)
	}
	if !singleResp.Found || singleResp.Record.Outcome != workbuddy.CompensationClaimed {
		t.Fatalf("expected found=true with claimed outcome, got %+v", singleResp)
	}

	// 3. Trigger PostWorkBuddyCompensation
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodPost, "/v8/management/workbuddy/compensation", nil)
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 from PostWorkBuddyCompensation, got %d", w3.Code)
	}

	// 4. Trigger PostWorkBuddyCheckin with action=compensation
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest(http.MethodPost, "/v8/management/workbuddy/checkin?action=compensation", nil)
	router.ServeHTTP(w4, req4)
	if w4.Code != http.StatusOK {
		t.Fatalf("expected 200 from PostWorkBuddyCheckin?action=compensation, got %d", w4.Code)
	}
}
