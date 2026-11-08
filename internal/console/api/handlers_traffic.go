package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/geoip"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/traffichub"
)

// HandleGetTraffic returns the visual gateway topology and stream snapshot.
// Accepts optional ?range=5m|today|7d|30d|all query parameter.
func HandleGetTraffic(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")

		rangeParam := strings.ToLower(strings.TrimSpace(c.DefaultQuery("range", "5m")))
		if rangeParam == "" || rangeParam == "5m" || rangeParam == "realtime" {
			snapshot := traffichub.Default().GetSnapshot()
			c.JSON(http.StatusOK, snapshot)
			return
		}

		ctx := c.Request.Context()
		if eng == nil || eng.DB() == nil {
			snapshot := traffichub.Default().GetSnapshot()
			c.JSON(http.StatusOK, snapshot)
			return
		}

		// Flush in-memory usage outbox so latest events are committed to SQLite
		_ = eng.Flusher().FlushUsage(ctx)

		now := time.Now().In(cstZone)
		var sinceTS int64

		switch rangeParam {
		case "today", "1d", "24h":
			todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cstZone)
			sinceTS = todayStart.Unix()
		case "7d", "week":
			sinceTS = now.AddDate(0, 0, -7).Unix()
		case "30d", "month":
			sinceTS = now.AddDate(0, 0, -30).Unix()
		case "all":
			sinceTS = 0
		default:
			// Fallback to real-time 5m snapshot if unknown range
			snapshot := traffichub.Default().GetSnapshot()
			c.JSON(http.StatusOK, snapshot)
			return
		}

		groups, err := eng.DB().SelectTrafficAggregates(ctx, sinceTS)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query traffic aggregates: %v", err)})
			return
		}

		recentRows, _ := eng.DB().SelectTrafficRecent(ctx, sinceTS, 50)
		var recentRecords []traffichub.TrafficRecord
		for _, r := range recentRows {
			recentRecords = append(recentRecords, traffichub.TrafficRecord{
				ID:        fmt.Sprintf("tr-%d-%d", r.TS*1000, r.ID),
				Timestamp: r.TS,
				ClientIP:  r.ClientIP,
				KeyLabel:  r.KeyLabel,
				Model:     r.Model,
				Channel:   traffichub.InferChannel(r.Model, r.AuthIndex),
				Status:    r.Status,
				LatencyMs: r.LatencyMs,
				Tokens:    r.TotalTokens,
				Geo:       geoip.Default().Lookup(r.ClientIP),
			})
		}

		inputs := make([]traffichub.HistoricalAggregateInput, len(groups))
		for i, g := range groups {
			inputs[i] = traffichub.HistoricalAggregateInput{
				ClientIP:     g.ClientIP,
				Model:        g.Model,
				AuthIndex:    g.AuthIndex,
				Status:       g.Status,
				KeyLabel:     g.KeyLabel,
				ReqCount:     g.ReqCount,
				TotalLatency: g.TotalLatency,
				TotalTokens:  g.TotalTokens,
				LastSeen:     g.LastSeen,
			}
		}

		snapshot := traffichub.BuildHistoricalSnapshot(rangeParam, inputs, recentRecords)
		c.JSON(http.StatusOK, snapshot)
	}
}
