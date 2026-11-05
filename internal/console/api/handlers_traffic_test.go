package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/traffichub"
)

func TestHandleGetTraffic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	hub := traffichub.Default()
	hub.Reset()

	hub.Record(traffichub.TrafficRecord{
		Timestamp: time.Now().Unix(),
		ClientIP:  "198.51.100.22",
		KeyLabel:  "team-beta",
		Model:     "gpt-4o",
		Channel:   "openai",
		Status:    200,
		LatencyMs: 180,
		Tokens:    950,
	})

	r := gin.New()
	r.GET("/console/api/traffic", HandleGetTraffic())

	req := httptest.NewRequest(http.MethodGet, "/console/api/traffic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp traffichub.TrafficResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse traffic json: %v", err)
	}

	if resp.Summary.TotalRecorded != 1 {
		t.Fatalf("expected 1 recorded event, got %d", resp.Summary.TotalRecorded)
	}
	if len(resp.Records) != 1 || resp.Records[0].ClientIP != "198.51.100.22" {
		t.Fatalf("unexpected records: %+v", resp.Records)
	}
}
