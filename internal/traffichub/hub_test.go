package traffichub

import (
	"fmt"
	"testing"
	"time"
)

func TestHubRingBufferOverflow(t *testing.T) {
	capacity := 10
	h := NewHub(capacity)

	baseTime := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	h.nowFunc = func() time.Time { return baseTime }

	// Insert 15 records (overflowing 10-item buffer by 5)
	for i := 1; i <= 15; i++ {
		h.Record(TrafficRecord{
			ID:        fmt.Sprintf("tr-%d", i),
			Timestamp: baseTime.Unix() + int64(i),
			ClientIP:  fmt.Sprintf("192.168.1.%d", i),
			Model:     "claude-3-5-sonnet",
			Channel:   "claude",
			Status:    200,
			LatencyMs: int64(i * 10),
			Tokens:    int64(i * 100),
		})
	}

	records := h.GetRecords(0)
	if len(records) != capacity {
		t.Fatalf("expected %d records in buffer, got %d", capacity, len(records))
	}

	// Newest should be tr-15
	if records[0].ID != "tr-15" {
		t.Fatalf("expected newest record to be 'tr-15', got %q", records[0].ID)
	}

	// Oldest remaining should be tr-6
	if records[len(records)-1].ID != "tr-6" {
		t.Fatalf("expected oldest surviving record to be 'tr-6', got %q", records[len(records)-1].ID)
	}
}

func TestHubGetSnapshot(t *testing.T) {
	h := NewHub(50)
	baseTime := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	h.nowFunc = func() time.Time { return baseTime }

	// Record 1: 200 OK from IP 1.1.1.1
	h.Record(TrafficRecord{
		Timestamp: baseTime.Unix() - 10,
		ClientIP:  "1.1.1.1",
		KeyLabel:  "sk-team-a",
		Model:     "claude-3-5-sonnet",
		Channel:   "claude",
		Status:    200,
		LatencyMs: 250,
		Tokens:    1200,
	})

	// Record 2: 200 OK from IP 1.1.1.1 (second call)
	h.Record(TrafficRecord{
		Timestamp: baseTime.Unix() - 5,
		ClientIP:  "1.1.1.1",
		KeyLabel:  "sk-team-a",
		Model:     "gpt-4o",
		Channel:   "openai",
		Status:    200,
		LatencyMs: 350,
		Tokens:    800,
	})

	// Record 3: 500 Error from IP 2.2.2.2
	h.Record(TrafficRecord{
		Timestamp: baseTime.Unix() - 2,
		ClientIP:  "2.2.2.2",
		KeyLabel:  "sk-team-b",
		Model:     "gemini-2.5-flash",
		Channel:   "gemini",
		Status:    500,
		LatencyMs: 150,
		Tokens:    0,
	})

	// Record 4: Outside 5m window (older than 300s)
	h.Record(TrafficRecord{
		Timestamp: baseTime.Unix() - 400,
		ClientIP:  "3.3.3.3",
		KeyLabel:  "sk-old",
		Model:     "deepseek-chat",
		Channel:   "deepseek",
		Status:    200,
		LatencyMs: 500,
		Tokens:    1000,
	})

	snapshot := h.GetSnapshot()

	// Metrics check
	if snapshot.Summary.TotalRecorded != 4 {
		t.Fatalf("expected TotalRecorded=4, got %d", snapshot.Summary.TotalRecorded)
	}
	if len(snapshot.Records) != 4 || len(snapshot.RecentRequests) != 4 {
		t.Fatalf("expected 4 records and 4 recent_requests, got %d and %d", len(snapshot.Records), len(snapshot.RecentRequests))
	}
	if snapshot.Summary.Requests5m != 3 {
		t.Fatalf("expected Requests5m=3, got %d", snapshot.Summary.Requests5m)
	}
	if snapshot.Summary.ActiveIPs != 2 {
		t.Fatalf("expected ActiveIPs=2, got %d", snapshot.Summary.ActiveIPs)
	}
	// 5m successes: 2 out of 3 -> ~0.666
	if snapshot.Summary.SuccessRate < 0.66 || snapshot.Summary.SuccessRate > 0.67 {
		t.Fatalf("expected SuccessRate ~0.666, got %f", snapshot.Summary.SuccessRate)
	}

	// Topology nodes check
	if len(snapshot.Topology.Nodes) == 0 {
		t.Fatal("expected non-empty topology nodes")
	}
	if len(snapshot.Topology.Links) == 0 {
		t.Fatal("expected non-empty topology links")
	}

	// IP stats check
	if len(snapshot.IPStats) != 3 {
		t.Fatalf("expected 3 distinct IP stats, got %d", len(snapshot.IPStats))
	}
	// Top IP should be 1.1.1.1 with 2 requests
	if snapshot.IPStats[0].IP != "1.1.1.1" || snapshot.IPStats[0].RequestCount != 2 {
		t.Fatalf("expected top IP to be 1.1.1.1 with 2 reqs, got %+v", snapshot.IPStats[0])
	}
}

func TestInferChannel(t *testing.T) {
	tests := []struct {
		model    string
		hint     string
		expected string
	}{
		{"claude-3-7-sonnet", "", "claude"},
		{"gpt-4o-mini", "", "openai"},
		{"o3-mini", "", "openai"},
		{"gemini-2.5-flash", "", "gemini"},
		{"deepseek-chat", "", "deepseek"},
		{"qwen-max", "", "qoder"},
		{"glm-4-plus", "", "zcode"},
		{"minimax-m3", "", "minimax"},
		{"abab6.5s-chat", "", "minimax"},
		{"minimax-m3", "1a2b3c4d5e6f7089", "minimax"},
		{"custom-model", "antigravity", "antigravity"},
		{"unknown-custom", "", "general"},
	}

	for _, tc := range tests {
		actual := InferChannel(tc.model, tc.hint)
		if actual != tc.expected {
			t.Fatalf("InferChannel(%q, %q) = %q; expected %q", tc.model, tc.hint, actual, tc.expected)
		}
	}
}

func TestHubRecordDualModelCompatibility(t *testing.T) {
	hub := NewHub(10)
	hub.Record(TrafficRecord{
		Timestamp:    time.Now().Unix(),
		ClientIP:     "127.0.0.1",
		KeyLabel:     "test-key",
		Model:        "minimax-m3",
		RequestModel: "claude-3-7-sonnet",
		ActualModel:  "minimax-m3",
		Channel:      "minimax",
		Status:       200,
		LatencyMs:    120,
		Tokens:       256,
	})

	records := hub.GetRecords(10)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.Model != "minimax-m3" {
		t.Fatalf("expected Model to be minimax-m3, got %s", r.Model)
	}
	if r.RequestModel != "claude-3-7-sonnet" {
		t.Fatalf("expected RequestModel to be claude-3-7-sonnet, got %s", r.RequestModel)
	}
	if r.ActualModel != "minimax-m3" {
		t.Fatalf("expected ActualModel to be minimax-m3, got %s", r.ActualModel)
	}
	if r.Channel != "minimax" {
		t.Fatalf("expected Channel to be minimax, got %s", r.Channel)
	}
}
