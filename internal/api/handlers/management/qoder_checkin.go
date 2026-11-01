package management

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/qoder"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

// GetQoderCheckin returns the daily checkin status for Qoder accounts.
func (h *Handler) GetQoderCheckin(c *gin.Context) {
	engine := qoder.GetGlobalCheckinEngine()
	today := qoder.TodayBeijing()
	authID := strings.TrimSpace(c.Query("auth_id"))

	if authID != "" {
		if engine != nil {
			record, found := engine.GetStatus(authID)
			if found && (record.Date == today || record.Day == today) {
				c.JSON(http.StatusOK, gin.H{
					"enabled": engine.IsEnabled(),
					"today":   today,
					"found":   true,
					"record":  record,
				})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"enabled": engine != nil && engine.IsEnabled(),
			"today":   today,
			"found":   false,
		})
		return
	}

	records := make(map[string]any)
	if engine != nil {
		for k, v := range engine.GetAllRecords() {
			if isInternalStorageFile(k) || strings.HasSuffix(strings.ToLower(k), "checkins.json") || k == "enabled" || k == "records" {
				continue
			}
			records[k] = v
		}
	}

	// Merge all managed Qoder credentials from AuthManager so un-checked-in accounts are also visible
	if h.authManager != nil {
		for _, a := range h.authManager.List() {
			if a == nil || isInternalStorageFile(a.ID) || isInternalStorageFile(a.FileName) || strings.HasSuffix(strings.ToLower(a.ID), "checkins.json") || a.ID == "enabled" || a.ID == "records" {
				continue
			}
			path := strings.TrimSpace(authAttribute(a, "path"))
			if path != "" && isInternalStorageFile(path) {
				continue
			}
			provider := strings.ToLower(strings.TrimSpace(a.Provider))
			authKind := strings.ToLower(strings.TrimSpace(a.Attributes["auth_kind"]))
			isQoder := provider == "qoder" || provider == "qoder-cn" || provider == "qoder-intl" || provider == "qoder-en" || authKind == "qoder"
			if !isQoder {
				continue
			}

			realm := qoder.ResolveRealm(a)
			dispName, nick, em, lbl := qoder.ResolveUserIdentity(a)

			if existing, exists := records[a.ID]; exists {
				if rec, ok := existing.(qoder.CheckinResult); ok {
					if rec.Realm == "" {
						rec.Realm = realm
					}
					if rec.DisplayName == "" || isUUIDLike(rec.DisplayName) {
						rec.DisplayName = dispName
					}
					if rec.User == "" || isUUIDLike(rec.User) {
						rec.User = dispName
					}
					if rec.Nickname == "" {
						rec.Nickname = nick
					}
					if rec.Email == "" {
						rec.Email = em
					}
					if rec.Label == "" {
						rec.Label = lbl
					}
					records[a.ID] = rec
				} else if mRec, ok := existing.(map[string]any); ok {
					if mRec["display_name"] == nil || isUUIDLike(fmt.Sprint(mRec["display_name"])) {
						mRec["display_name"] = dispName
					}
					if mRec["user"] == nil || isUUIDLike(fmt.Sprint(mRec["user"])) {
						mRec["user"] = dispName
					}
					if mRec["nickname"] == nil {
						mRec["nickname"] = nick
					}
					if mRec["email"] == nil {
						mRec["email"] = em
					}
					if mRec["label"] == nil {
						mRec["label"] = lbl
					}
					records[a.ID] = mRec
				}
				continue
			}

			records[a.ID] = gin.H{
				"auth_id":      a.ID,
				"user":         dispName,
				"display_name": dispName,
				"nickname":     nick,
				"email":        em,
				"label":        lbl,
				"day":          today,
				"date":         today,
				"outcome":      "pending",
				"status":       "pending",
				"realm":        realm,
				"message":      "今日待打卡",
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled": engine != nil && engine.IsEnabled(),
		"today":   today,
		"records": records,
	})
}

// PostQoderCheckin triggers daily check-in for one or all Qoder accounts,
// or toggles the automated check-in engine state if {"enabled": bool} is provided.
func (h *Handler) PostQoderCheckin(c *gin.Context) {
	engine := qoder.GetGlobalCheckinEngine()
	var req struct {
		Action  string `json:"action"`
		AuthID  string `json:"auth_id"`
		Enabled *bool  `json:"enabled"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Enabled != nil {
		if engine != nil {
			engine.SetEnabled(*req.Enabled)
		}
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"enabled": engine != nil && engine.IsEnabled(),
		})
		return
	}

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
		res, err := qoder.Checkin(ctx, client, targetAuth)
		if err != nil {
			if engine != nil && res != nil {
				engine.RecordCheckin(targetAuth.ID, *res)
			}
			errMsg := err.Error()
			if strings.Contains(strings.ToLower(errMsg), "unauthorized") || strings.Contains(errMsg, "401") {
				errMsg = "登录凭证已失效或过期，请重新登录授权"
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  errMsg,
				"result": res,
			})
			return
		}
		if engine != nil && res != nil {
			engine.RecordCheckin(targetAuth.ID, *res)
		}
		c.JSON(http.StatusOK, gin.H{
			"today":  qoder.TodayBeijing(),
			"result": res,
		})
		return
	}

	// Trigger check-in for all Qoder accounts
	if engine != nil {
		results := engine.RunCheckinAll(ctx)
		c.JSON(http.StatusOK, gin.H{
			"today":   qoder.TodayBeijing(),
			"results": results,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"today":   qoder.TodayBeijing(),
		"results": map[string]any{},
	})
}

// PutQoderCheckin updates the checkin engine configuration (e.g. enabled state).
func (h *Handler) PutQoderCheckin(c *gin.Context) {
	engine := qoder.GetGlobalCheckinEngine()
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.Enabled != nil && engine != nil {
		engine.SetEnabled(*req.Enabled)
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"enabled": engine != nil && engine.IsEnabled(),
	})
}

func isUUIDLike(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) >= 32 && strings.Count(s, "-") >= 3
}
