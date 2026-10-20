package management

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/qoder"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

var (
	qoderCheckinRecordsMu sync.RWMutex
	qoderCheckinRecords   = make(map[string]*qoder.CheckinResult)
)

// RecordQoderCheckin saves a check-in result in memory.
func RecordQoderCheckin(authID string, res *qoder.CheckinResult) {
	if authID == "" || res == nil {
		return
	}
	qoderCheckinRecordsMu.Lock()
	defer qoderCheckinRecordsMu.Unlock()
	qoderCheckinRecords[authID] = res
}

// GetQoderCheckinRecord returns the recorded checkin result for an authID.
func GetQoderCheckinRecord(authID string) (*qoder.CheckinResult, bool) {
	qoderCheckinRecordsMu.RLock()
	defer qoderCheckinRecordsMu.RUnlock()
	res, ok := qoderCheckinRecords[authID]
	return res, ok
}

// GetQoderCheckin returns the daily checkin status for Qoder accounts.
func (h *Handler) GetQoderCheckin(c *gin.Context) {
	today := qoder.TodayBeijing()
	authID := strings.TrimSpace(c.Query("auth_id"))

	if authID != "" {
		if rec, found := GetQoderCheckinRecord(authID); found && rec != nil && rec.Date == today {
			c.JSON(http.StatusOK, gin.H{
				"today":  today,
				"found":  true,
				"record": rec,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"today": today,
			"found": false,
		})
		return
	}

	records := make(map[string]any)
	qoderCheckinRecordsMu.RLock()
	for k, v := range qoderCheckinRecords {
		if v != nil && v.Date == today {
			records[k] = v
		}
	}
	qoderCheckinRecordsMu.RUnlock()

	if h.authManager != nil {
		for _, a := range h.authManager.List() {
			if a == nil || isInternalStorageFile(a.ID) || isInternalStorageFile(a.FileName) {
				continue
			}
			provider := strings.ToLower(strings.TrimSpace(a.Provider))
			authKind := strings.ToLower(strings.TrimSpace(a.Attributes["auth_kind"]))
			isQoder := provider == "qoder" || provider == "qoder-cn" || provider == "qoder-intl" || provider == "qoder-en" || authKind == "qoder"
			if !isQoder {
				continue
			}

			if _, exists := records[a.ID]; exists {
				continue
			}

			realm := qoder.ResolveRealm(a)
			user := qoder.ResolveUserID(a)
			if a.Metadata != nil {
				if nick, ok := a.Metadata["nickname"].(string); ok && strings.TrimSpace(nick) != "" {
					user = strings.TrimSpace(nick)
				}
			}

			records[a.ID] = gin.H{
				"auth_id": a.ID,
				"user":    user,
				"day":     today,
				"outcome": "pending",
				"realm":   realm,
				"message": "今日待签到",
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"today":   today,
		"records": records,
	})
}

// PostQoderCheckin triggers daily check-in for one or all Qoder accounts.
func (h *Handler) PostQoderCheckin(c *gin.Context) {
	var req struct {
		AuthID string `json:"auth_id"`
	}
	_ = c.ShouldBindJSON(&req)

	authID := strings.TrimSpace(c.Query("auth_id"))
	if authID == "" && strings.TrimSpace(req.AuthID) != "" {
		authID = strings.TrimSpace(req.AuthID)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	if authID != "" {
		var targetAuth *cliproxyauth.Auth
		if h.authManager != nil {
			targetAuth, _ = h.authManager.GetByID(authID)
		}
		if targetAuth == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "auth not found"})
			return
		}

		client := &http.Client{Timeout: 20 * time.Second}
		res, err := qoder.DoCheckin(ctx, client, targetAuth)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  err.Error(),
				"result": res,
			})
			return
		}
		RecordQoderCheckin(targetAuth.ID, res)
		c.JSON(http.StatusOK, gin.H{
			"today":  qoder.TodayBeijing(),
			"result": res,
		})
		return
	}

	// Checkin all Qoder accounts
	today := qoder.TodayBeijing()
	results := make([]*qoder.CheckinResult, 0)
	if h.authManager != nil {
		for _, a := range h.authManager.List() {
			if a == nil || a.Disabled {
				continue
			}
			provider := strings.ToLower(strings.TrimSpace(a.Provider))
			authKind := strings.ToLower(strings.TrimSpace(a.Attributes["auth_kind"]))
			if provider != "qoder" && provider != "qoder-cn" && provider != "qoder-intl" && provider != "qoder-en" && authKind != "qoder" {
				continue
			}

			client := &http.Client{Timeout: 20 * time.Second}
			res, err := qoder.DoCheckin(ctx, client, a)
			if err == nil && res != nil {
				RecordQoderCheckin(a.ID, res)
				results = append(results, res)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"today":   today,
		"results": results,
	})
}
