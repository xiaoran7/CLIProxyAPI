package traffichub

// TrafficRecord represents an individual proxied API request for observability and topology analysis.
type TrafficRecord struct {
	ID           string `json:"id"`
	Timestamp    int64  `json:"timestamp"` // Unix timestamp in seconds
	ClientIP     string `json:"client_ip"`
	KeyLabel     string `json:"key_label"`
	Path         string `json:"path,omitempty"`
	Model        string `json:"model"`
	RequestModel string `json:"request_model,omitempty"`
	ActualModel  string `json:"actual_model,omitempty"`
	Channel      string `json:"channel"`
	Status       int    `json:"status"`
	LatencyMs    int64  `json:"latency_ms"`
	Tokens       int64  `json:"tokens"`
}

// TrafficSummary contains high-level aggregated metrics for the visual dashboard.
type TrafficSummary struct {
	ActiveIPs     int     `json:"active_ips"`
	Requests5m    int64   `json:"requests_5m"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	SuccessRate   float64 `json:"success_rate"`
	TotalRecorded int     `json:"total_recorded"`
}

// TopologyNode represents an entity in the traffic flow graph (client IP, model, channel, or status).
type TopologyNode struct {
	ID    string `json:"id"`
	Type  string `json:"type"` // "client_ip", "model", "channel", "status"
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// TopologyLink represents a directional connection between nodes.
type TopologyLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Value  int64  `json:"value"`
}

// Topology represents the complete multi-stage graph topology.
type Topology struct {
	Nodes []TopologyNode `json:"nodes"`
	Links []TopologyLink `json:"links"`
}

// IPStat summarizes activity for an individual client IP.
type IPStat struct {
	IP           string   `json:"ip"`
	RequestCount int64    `json:"request_count"`
	Tokens       int64    `json:"tokens"`
	LastSeen     int64    `json:"last_seen"`
	TopModels    []string `json:"top_models"`
}

// TrafficResponse is the payload returned by GET /v0/management/traffic.
type TrafficResponse struct {
	Summary        TrafficSummary  `json:"summary"`
	Records        []TrafficRecord `json:"records"`
	RecentRequests []TrafficRecord `json:"recent_requests"`
	Topology       Topology        `json:"topology"`
	IPStats        []IPStat        `json:"ip_stats"`
}
