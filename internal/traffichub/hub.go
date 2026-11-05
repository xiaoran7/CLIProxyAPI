package traffichub

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultCapacity is the maximum number of recent traffic records stored in memory.
	DefaultCapacity = 500
	// Window5mSeconds is the sliding time window for active metric aggregation.
	Window5mSeconds = 300
)

var (
	defaultHubInstance *Hub
	defaultHubOnce     sync.Once
	idSeq              uint64
)

// Hub is an in-memory, thread-safe circular buffer storing recent proxied requests.
type Hub struct {
	mu       sync.RWMutex
	capacity int
	records  []TrafficRecord
	start    int
	count    int
	nowFunc  func() time.Time
}

// NewHub initializes a new Hub instance with the specified capacity.
func NewHub(capacity int) *Hub {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Hub{
		capacity: capacity,
		records:  make([]TrafficRecord, capacity),
		nowFunc:  time.Now,
	}
}

// Default returns the process-wide singleton Hub instance.
func Default() *Hub {
	defaultHubOnce.Do(func() {
		defaultHubInstance = NewHub(DefaultCapacity)
	})
	return defaultHubInstance
}

// Record appends a new traffic event into the circular buffer.
func (h *Hub) Record(rec TrafficRecord) {
	if h == nil {
		return
	}

	now := h.nowFunc()
	if rec.Timestamp == 0 {
		rec.Timestamp = now.Unix()
	}
	if rec.ID == "" {
		seq := atomic.AddUint64(&idSeq, 1)
		rec.ID = fmt.Sprintf("tr-%d-%d", now.UnixMilli(), seq)
	}
	if rec.ClientIP == "" {
		rec.ClientIP = "127.0.0.1"
	}
	if rec.Model == "" {
		rec.Model = "unknown"
	}
	if rec.Channel == "" {
		rec.Channel = InferChannel(rec.Model, "")
	}
	if rec.KeyLabel == "" {
		rec.KeyLabel = "-"
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	idx := (h.start + h.count) % h.capacity
	if h.count < h.capacity {
		h.records[idx] = rec
		h.count++
	} else {
		// Buffer is full: overwrite oldest record at start and advance start index
		h.records[h.start] = rec
		h.start = (h.start + 1) % h.capacity
	}
}

// GetRecords returns up to limit traffic records sorted in reverse-chronological order (newest first).
func (h *Hub) GetRecords(limit int) []TrafficRecord {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.count == 0 {
		return []TrafficRecord{}
	}

	if limit <= 0 || limit > h.count {
		limit = h.count
	}

	result := make([]TrafficRecord, limit)
	for i := 0; i < limit; i++ {
		pos := (h.start + h.count - 1 - i) % h.capacity
		if pos < 0 {
			pos += h.capacity
		}
		result[i] = h.records[pos]
	}
	return result
}

// Reset clears all records in the hub (used in testing).
func (h *Hub) Reset() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = make([]TrafficRecord, h.capacity)
	h.start = 0
	h.count = 0
}

// GetSnapshot calculates the complete traffic metrics, records, topology, and IP breakdown.
func (h *Hub) GetSnapshot() TrafficResponse {
	if h == nil {
		return TrafficResponse{
			Summary:  TrafficSummary{},
			Records:  []TrafficRecord{},
			Topology: Topology{Nodes: []TopologyNode{}, Links: []TopologyLink{}},
			IPStats:  []IPStat{},
		}
	}

	records := h.GetRecords(h.capacity)
	now := h.nowFunc().Unix()
	cutoff5m := now - Window5mSeconds

	var (
		reqs5m        int64
		success5m     int64
		latencySum5m  int64
		latencyCount  int64
		activeIPs5m   = make(map[string]struct{})
		overallIPs    = make(map[string]struct{})
		overallSucc   int64
		overallLatSum int64
	)

	type ipAccum struct {
		ip        string
		count     int64
		tokens    int64
		lastSeen  int64
		modelHits map[string]int64
	}
	ipMap := make(map[string]*ipAccum)

	// Topology link counters: "source|target" -> count
	linkCounts := make(map[string]int64)
	nodeMap := make(map[string]*TopologyNode)

	ensureNode := func(id, nodeType, label string) *TopologyNode {
		if n, ok := nodeMap[id]; ok {
			return n
		}
		n := &TopologyNode{
			ID:    id,
			Type:  nodeType,
			Label: label,
			Count: 0,
		}
		nodeMap[id] = n
		return n
	}

	for _, r := range records {
		overallIPs[r.ClientIP] = struct{}{}
		overallLatSum += r.LatencyMs

		isSuccess := (r.Status >= 200 && r.Status < 400) || r.Status == 0
		if isSuccess {
			overallSucc++
		}

		if r.Timestamp >= cutoff5m {
			reqs5m++
			activeIPs5m[r.ClientIP] = struct{}{}
			latencySum5m += r.LatencyMs
			latencyCount++
			if isSuccess {
				success5m++
			}
		}

		// IP stats aggregation
		acc, ok := ipMap[r.ClientIP]
		if !ok {
			acc = &ipAccum{
				ip:        r.ClientIP,
				modelHits: make(map[string]int64),
			}
			ipMap[r.ClientIP] = acc
		}
		acc.count++
		acc.tokens += r.Tokens
		if r.Timestamp > acc.lastSeen {
			acc.lastSeen = r.Timestamp
		}
		if r.Model != "" {
			acc.modelHits[r.Model]++
		}

		// Topology node & link generation
		statusGroup := formatStatusGroup(r.Status)
		ipNodeID := "ip:" + r.ClientIP
		modelNodeID := "model:" + r.Model
		channelNodeID := "channel:" + r.Channel
		statusNodeID := "status:" + statusGroup

		nIP := ensureNode(ipNodeID, "client_ip", r.ClientIP)
		nIP.Count++
		nModel := ensureNode(modelNodeID, "model", r.Model)
		nModel.Count++
		nChan := ensureNode(channelNodeID, "channel", r.Channel)
		nChan.Count++
		nStatus := ensureNode(statusNodeID, "status", statusGroup)
		nStatus.Count++

		linkCounts[ipNodeID+"|"+modelNodeID]++
		linkCounts[modelNodeID+"|"+channelNodeID]++
		linkCounts[channelNodeID+"|"+statusNodeID]++
	}

	totalRec := len(records)
	var avgLatency float64
	var successRate float64

	if latencyCount > 0 {
		avgLatency = float64(latencySum5m) / float64(latencyCount)
		successRate = float64(success5m) / float64(reqs5m)
	} else if totalRec > 0 {
		avgLatency = float64(overallLatSum) / float64(totalRec)
		successRate = float64(overallSucc) / float64(totalRec)
	}

	activeIPCount := len(activeIPs5m)
	if activeIPCount == 0 {
		activeIPCount = len(overallIPs)
	}

	// Build nodes and links slice
	nodes := make([]TopologyNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, *n)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Type != nodes[j].Type {
			return nodeTypeOrder(nodes[i].Type) < nodeTypeOrder(nodes[j].Type)
		}
		return nodes[i].Count > nodes[j].Count
	})

	links := make([]TopologyLink, 0, len(linkCounts))
	for k, v := range linkCounts {
		parts := strings.Split(k, "|")
		if len(parts) == 2 {
			links = append(links, TopologyLink{
				Source: parts[0],
				Target: parts[1],
				Value:  v,
			})
		}
	}
	sort.Slice(links, func(i, j int) bool {
		return links[i].Value > links[j].Value
	})

	// Build IP stats slice
	ipStats := make([]IPStat, 0, len(ipMap))
	for _, acc := range ipMap {
		type mHit struct {
			model string
			hits  int64
		}
		mList := make([]mHit, 0, len(acc.modelHits))
		for m, h := range acc.modelHits {
			mList = append(mList, mHit{model: m, hits: h})
		}
		sort.Slice(mList, func(i, j int) bool {
			return mList[i].hits > mList[j].hits
		})
		topModels := make([]string, 0, 3)
		for i := 0; i < len(mList) && i < 3; i++ {
			topModels = append(topModels, mList[i].model)
		}
		ipStats = append(ipStats, IPStat{
			IP:           acc.ip,
			RequestCount: acc.count,
			Tokens:       acc.tokens,
			LastSeen:     acc.lastSeen,
			TopModels:    topModels,
		})
	}
	sort.Slice(ipStats, func(i, j int) bool {
		return ipStats[i].RequestCount > ipStats[j].RequestCount
	})

	return TrafficResponse{
		Summary: TrafficSummary{
			ActiveIPs:     activeIPCount,
			Requests5m:    reqs5m,
			AvgLatencyMs:  avgLatency,
			SuccessRate:   successRate,
			TotalRecorded: totalRec,
		},
		Records: records,
		Topology: Topology{
			Nodes: nodes,
			Links: links,
		},
		IPStats: ipStats,
	}
}

func nodeTypeOrder(t string) int {
	switch t {
	case "client_ip":
		return 1
	case "model":
		return 2
	case "channel":
		return 3
	case "status":
		return 4
	default:
		return 5
	}
}

func formatStatusGroup(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx Success"
	case code == 499:
		return "499 Client Closed"
	case code >= 400 && code < 500:
		return "4xx Client Error"
	case code >= 500:
		return "5xx Upstream Error"
	case code == 0:
		return "2xx Success"
	default:
		return fmt.Sprintf("%d Status", code)
	}
}

// InferChannel determines the canonical AI provider channel name based on model name or auth hint.
func InferChannel(model, channelHint string) string {
	if channelHint != "" && !strings.HasPrefix(channelHint, "http") && len(channelHint) < 30 {
		return strings.ToLower(strings.TrimSpace(channelHint))
	}

	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(m, "claude"):
		return "claude"
	case strings.Contains(m, "gpt") || strings.Contains(m, "o1") || strings.Contains(m, "o3") || strings.Contains(m, "text-embedding"):
		return "openai"
	case strings.Contains(m, "argon") || strings.Contains(m, "antigravity"):
		return "antigravity"
	case strings.Contains(m, "gemini"):
		return "gemini"
	case strings.Contains(m, "deepseek"):
		return "deepseek"
	case strings.Contains(m, "qwen") || strings.Contains(m, "qoder") || strings.Contains(m, "dfmodel"):
		return "qoder"
	case strings.Contains(m, "glm") || strings.Contains(m, "zcode") || strings.Contains(m, "bigmodel"):
		return "zcode"
	case strings.Contains(m, "workbuddy"):
		return "workbuddy"
	case strings.Contains(m, "kimi") || strings.Contains(m, "moonshot"):
		return "kimi"
	case strings.Contains(m, "grok") || strings.Contains(m, "xai"):
		return "xai"
	default:
		return "general"
	}
}
