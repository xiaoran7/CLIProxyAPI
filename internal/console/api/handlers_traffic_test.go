package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
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
	r.GET("/console/api/traffic", HandleGetTraffic(nil))

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

func TestHandleGetTrafficHistorical(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_traffic.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	now := time.Now().Unix()
	_, err = database.Exec(`
INSERT INTO keys (id, api_key, alias, active) VALUES (1, "sk-test-1234", "test-key", 1);
INSERT INTO request_events (ts, key_id, model, status, prompt_tokens, completion_tokens, cached_tokens, total_tokens, latency_ms, stream, client_ip, auth_index)
VALUES (?, 1, "claude-3-5-sonnet", 200, 100, 200, 50, 300, 250, 1, "100.64.0.1", "auth-1");
`, now)
	if err != nil {
		t.Fatalf("failed to insert test event: %v", err)
	}

	eng, err := engine.NewEngine(database, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Stop()

	r := gin.New()
	r.GET("/console/api/traffic", HandleGetTraffic(eng))

	for _, rParam := range []string{"today", "7d", "30d", "all"} {
		req := httptest.NewRequest(http.MethodGet, "/console/api/traffic?range="+rParam, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("range %s: expected HTTP 200, got %d: %s", rParam, w.Code, w.Body.String())
		}

		var resp traffichub.TrafficResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("range %s: failed to unmarshal json: %v", rParam, err)
		}

		if resp.Range != rParam {
			t.Fatalf("expected range %s, got %s", rParam, resp.Range)
		}
		if resp.Summary.TotalRecorded != 1 {
			t.Fatalf("range %s: expected 1 total recorded, got %d", rParam, resp.Summary.TotalRecorded)
		}
		if len(resp.Topology.Nodes) == 0 {
			t.Fatalf("range %s: expected topology nodes, got 0", rParam)
		}
	}
}
