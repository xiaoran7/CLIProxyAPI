package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/traffichub"
)

func TestGetTrafficHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Pre-populate traffichub singleton with sample records
	hub := traffichub.Default()
	hub.Reset()

	hub.Record(traffichub.TrafficRecord{
		Timestamp: time.Now().Unix(),
		ClientIP:  "203.0.113.10",
		KeyLabel:  "sk-prod-team",
		Model:     "claude-3-7-sonnet",
		Channel:   "claude",
		Status:    200,
		LatencyMs: 320,
		Tokens:    1500,
	})

	h := &Handler{}
	r := gin.New()
	r.GET("/v0/management/traffic", h.GetTraffic)

	req := httptest.NewRequest(http.MethodGet, "/v0/management/traffic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp traffichub.TrafficResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse traffic response: %v", err)
	}

	if resp.Summary.TotalRecorded != 1 {
		t.Fatalf("expected TotalRecorded=1, got %d", resp.Summary.TotalRecorded)
	}
	if len(resp.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(resp.Records))
	}
	if len(resp.RecentRequests) != 1 {
		t.Fatalf("expected 1 recent_request, got %d", len(resp.RecentRequests))
	}
	if resp.Records[0].ClientIP != "203.0.113.10" {
		t.Fatalf("expected client IP 203.0.113.10, got %s", resp.Records[0].ClientIP)
	}
	if len(resp.Topology.Nodes) == 0 {
		t.Fatal("expected topology nodes to be populated")
	}
	if len(resp.IPStats) != 1 || resp.IPStats[0].IP != "203.0.113.10" {
		t.Fatalf("expected IPStat for 203.0.113.10, got %+v", resp.IPStats)
	}
}
