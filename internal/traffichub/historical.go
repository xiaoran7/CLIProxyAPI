package traffichub

import (
	"sort"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/geoip"
)

// BuildHistoricalSnapshot aggregates grouped historical metrics into a visual topology snapshot.
func BuildHistoricalSnapshot(rangeName string, groups []HistoricalAggregateInput, recentRecords []TrafficRecord) TrafficResponse {
	if len(groups) == 0 && len(recentRecords) == 0 {
		return TrafficResponse{
			Range:          rangeName,
			Summary:        TrafficSummary{},
			Records:        []TrafficRecord{},
			RecentRequests: []TrafficRecord{},
			Topology:       Topology{Nodes: []TopologyNode{}, Links: []TopologyLink{}},
			IPStats:        []IPStat{},
			GeoStats:       []GeoSummaryStat{},
		}
	}

	var (
		totalRequests int64
		totalSuccess  int64
		totalLatency  int64
		overallIPs    = make(map[string]struct{})
	)

	type ipKeyAccum struct {
		key    string
		count  int64
		tokens int64
	}

	type ipAccum struct {
		ip           string
		count        int64
		tokens       int64
		lastSeen     int64
		latencySum   int64
		successCount int64
		modelHits    map[string]int64
		keyHits      map[string]*ipKeyAccum
		geo          *geoip.Location
	}
	ipMap := make(map[string]*ipAccum)

	type keyAccum struct {
		key          string
		ips          map[string]struct{}
		requestCount int64
		tokens       int64
		lastSeen     int64
	}
	keyMap := make(map[string]*keyAccum)

	type geoAccum struct {
		location     string
		countryCode  string
		requestCount int64
		tokens       int64
		isProxy      bool
	}
	geoMap := make(map[string]*geoAccum)

	linkCounts := make(map[string]int64)
	nodeMap := make(map[string]*TopologyNode)

	ensureNode := func(id, nodeType, label string, geo *geoip.Location) *TopologyNode {
		if n, ok := nodeMap[id]; ok {
			if n.Geo == nil && geo != nil {
				n.Geo = geo
			}
			return n
		}
		n := &TopologyNode{
			ID:    id,
			Type:  nodeType,
			Label: label,
			Count: 0,
			Geo:   geo,
		}
		nodeMap[id] = n
		return n
	}

	for _, g := range groups {
		if g.ClientIP == "" {
			g.ClientIP = "127.0.0.1"
		}
		if g.Model == "" {
			g.Model = "unknown"
		}
		if g.KeyLabel == "" {
			g.KeyLabel = "-"
		}

		totalRequests += g.ReqCount
		totalLatency += g.TotalLatency
		overallIPs[g.ClientIP] = struct{}{}

		isSuccess := (g.Status >= 200 && g.Status < 400) || g.Status == 0
		if isSuccess {
			totalSuccess += g.ReqCount
		}

		// IP stats aggregation
		acc, ok := ipMap[g.ClientIP]
		if !ok {
			loc := geoip.Default().Lookup(g.ClientIP)
			acc = &ipAccum{
				ip:        g.ClientIP,
				modelHits: make(map[string]int64),
				keyHits:   make(map[string]*ipKeyAccum),
				geo:       loc,
			}
			ipMap[g.ClientIP] = acc
		}
		acc.count += g.ReqCount
		acc.tokens += g.TotalTokens
		acc.latencySum += g.TotalLatency
		if isSuccess {
			acc.successCount += g.ReqCount
		}
		if g.LastSeen > acc.lastSeen {
			acc.lastSeen = g.LastSeen
		}
		if g.Model != "" {
			acc.modelHits[g.Model] += g.ReqCount
		}
		keyName := g.KeyLabel
		if keyName == "" {
			keyName = "-"
		}
		kh, okKey := acc.keyHits[keyName]
		if !okKey {
			kh = &ipKeyAccum{key: keyName}
			acc.keyHits[keyName] = kh
		}
		kh.count += g.ReqCount
		kh.tokens += g.TotalTokens

		// Key summary aggregation
		ka, okKa := keyMap[keyName]
		if !okKa {
			ka = &keyAccum{
				key: keyName,
				ips: make(map[string]struct{}),
			}
			keyMap[keyName] = ka
		}
		ka.ips[g.ClientIP] = struct{}{}
		ka.requestCount += g.ReqCount
		ka.tokens += g.TotalTokens
		if g.LastSeen > ka.lastSeen {
			ka.lastSeen = g.LastSeen
		}

		// Geo regional stats aggregation
		locStr := "未知地域"
		cCode := "UN"
		isProxy := false
		if acc.geo != nil {
			locStr = acc.geo.Format()
			cCode = acc.geo.CountryCode
			isProxy = acc.geo.IsProxy || acc.geo.IsVPN || acc.geo.IsTor
		}
		ga, okGeo := geoMap[locStr]
		if !okGeo {
			ga = &geoAccum{
				location:    locStr,
				countryCode: cCode,
				isProxy:     isProxy,
			}
			geoMap[locStr] = ga
		}
		ga.requestCount += g.ReqCount
		ga.tokens += g.TotalTokens
		if isProxy {
			ga.isProxy = true
		}

		// Topology node & link generation
		channel := g.Channel
		if channel == "" {
			channel = InferChannel(g.Model, g.AuthIndex)
		}
		statusGroup := formatStatusGroup(g.Status)

		ipNodeID := "ip:" + g.ClientIP
		modelNodeID := "model:" + g.Model
		channelNodeID := "channel:" + channel
		statusNodeID := "status:" + statusGroup

		nIP := ensureNode(ipNodeID, "client_ip", g.ClientIP, acc.geo)
		nIP.Count += g.ReqCount
		nModel := ensureNode(modelNodeID, "model", g.Model, nil)
		nModel.Count += g.ReqCount
		nChan := ensureNode(channelNodeID, "channel", channel, nil)
		nChan.Count += g.ReqCount
		nStatus := ensureNode(statusNodeID, "status", statusGroup, nil)
		nStatus.Count += g.ReqCount

		linkCounts[ipNodeID+"|"+modelNodeID] += g.ReqCount
		linkCounts[modelNodeID+"|"+channelNodeID] += g.ReqCount
		linkCounts[channelNodeID+"|"+statusNodeID] += g.ReqCount
	}

	var avgLatency float64
	var successRate float64
	if totalRequests > 0 {
		avgLatency = float64(totalLatency) / float64(totalRequests)
		successRate = float64(totalSuccess) / float64(totalRequests)
	}

	activeIPCount := len(overallIPs)

	// Build nodes and links slice
	nodes := make([]TopologyNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, *n)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Type != nodes[j].Type {
			return nodeTypeOrder(nodes[i].Type) < nodeTypeOrder(nodes[j].Type)
		}
		if nodes[i].Count != nodes[j].Count {
			return nodes[i].Count > nodes[j].Count
		}
		return nodes[i].ID < nodes[j].ID
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
		if links[i].Value != links[j].Value {
			return links[i].Value > links[j].Value
		}
		if links[i].Source != links[j].Source {
			return links[i].Source < links[j].Source
		}
		return links[i].Target < links[j].Target
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
			if mList[i].hits != mList[j].hits {
				return mList[i].hits > mList[j].hits
			}
			return mList[i].model < mList[j].model
		})
		topModels := make([]string, 0, 3)
		for i := 0; i < len(mList) && i < 3; i++ {
			topModels = append(topModels, mList[i].model)
		}

		keysList := make([]IPKeyStat, 0, len(acc.keyHits))
		for _, kh := range acc.keyHits {
			keysList = append(keysList, IPKeyStat{
				Key:          kh.key,
				RequestCount: kh.count,
				Tokens:       kh.tokens,
			})
		}
		sort.Slice(keysList, func(i, j int) bool {
			if keysList[i].RequestCount != keysList[j].RequestCount {
				return keysList[i].RequestCount > keysList[j].RequestCount
			}
			return keysList[i].Key < keysList[j].Key
		})

		var ipSuccRate float64
		var ipAvgLat float64
		if acc.count > 0 {
			ipSuccRate = float64(acc.successCount) / float64(acc.count)
			ipAvgLat = float64(acc.latencySum) / float64(acc.count)
		}

		ipStats = append(ipStats, IPStat{
			IP:           acc.ip,
			RequestCount: acc.count,
			Tokens:       acc.tokens,
			LastSeen:     acc.lastSeen,
			SuccessRate:  ipSuccRate,
			AvgLatencyMs: ipAvgLat,
			TopModels:    topModels,
			Keys:         keysList,
			Geo:          acc.geo,
		})
	}
	sort.Slice(ipStats, func(i, j int) bool {
		if ipStats[i].RequestCount != ipStats[j].RequestCount {
			return ipStats[i].RequestCount > ipStats[j].RequestCount
		}
		return ipStats[i].IP < ipStats[j].IP
	})

	// Build KeyStats reverse-lookup aggregation slice
	keyStats := make([]KeySummaryStat, 0, len(keyMap))
	for _, ka := range keyMap {
		ipList := make([]string, 0, len(ka.ips))
		for ip := range ka.ips {
			ipList = append(ipList, ip)
		}
		sort.Strings(ipList)
		keyStats = append(keyStats, KeySummaryStat{
			Key:          ka.key,
			IPCount:      len(ipList),
			IPs:          ipList,
			RequestCount: ka.requestCount,
			Tokens:       ka.tokens,
			LastSeen:     ka.lastSeen,
		})
	}
	sort.Slice(keyStats, func(i, j int) bool {
		if keyStats[i].RequestCount != keyStats[j].RequestCount {
			return keyStats[i].RequestCount > keyStats[j].RequestCount
		}
		return keyStats[i].Key < keyStats[j].Key
	})

	// Build GeoStats regional aggregation slice
	geoStats := make([]GeoSummaryStat, 0, len(geoMap))
	for _, ga := range geoMap {
		geoStats = append(geoStats, GeoSummaryStat{
			Location:     ga.location,
			CountryCode:  ga.countryCode,
			RequestCount: ga.requestCount,
			Tokens:       ga.tokens,
			IsProxy:      ga.isProxy,
		})
	}
	sort.Slice(geoStats, func(i, j int) bool {
		if geoStats[i].RequestCount != geoStats[j].RequestCount {
			return geoStats[i].RequestCount > geoStats[j].RequestCount
		}
		return geoStats[i].Location < geoStats[j].Location
	})

	if recentRecords == nil {
		recentRecords = []TrafficRecord{}
	}

	return TrafficResponse{
		Range: rangeName,
		Summary: TrafficSummary{
			ActiveIPs:     activeIPCount,
			Requests5m:    totalRequests,
			AvgLatencyMs:  avgLatency,
			SuccessRate:   successRate,
			TotalRecorded: int(totalRequests),
		},
		Records:        recentRecords,
		RecentRequests: recentRecords,
		Topology: Topology{
			Nodes: nodes,
			Links: links,
		},
		IPStats:  ipStats,
		KeyStats: keyStats,
		GeoStats: geoStats,
	}
}
