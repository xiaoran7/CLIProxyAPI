package management

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/workbuddy"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

// GetWorkBuddyCheckin returns the current daily checkin records.
func (h *Handler) GetWorkBuddyCheckin(c *gin.Context) {
	engine := workbuddy.GetGlobalCheckinEngine()
	today := workbuddy.TodayBeijing()

	authID := strings.TrimSpace(c.Query("auth_id"))
	if authID != "" {
		if engine != nil {
			record, found := engine.GetStatus(authID)
			if found {
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

	// Merge all managed WorkBuddy credentials from AuthManager so un-checked-in accounts are also visible
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
			isWorkbuddy := provider == "workbuddy" || provider == "workbuddy-cn" || authKind == "workbuddy" || provider == "copilot-tencent"
			if !isWorkbuddy {
				continue
			}

			realm := workbuddy.ResolveRealm(a)
			if existing, exists := records[a.ID]; exists {
				if rec, ok := existing.(workbuddy.CheckinResult); ok && rec.Realm == "" {
					rec.Realm = realm
					records[a.ID] = rec
				}
				continue
			}

			outcome := "inactive"
			msg := "国际版无打卡活动"
			if strings.EqualFold(realm, workbuddy.RealmCN) {
				outcome = "pending"
				msg = "今日待签到"
			}
			user := workbuddy.ExtractNickname(a)
			if user == "" {
				user = a.ID
			}

			records[a.ID] = gin.H{
				"auth_id": a.ID,
				"user":    user,
				"day":     today,
				"outcome": outcome,
				"realm":   realm,
				"message": msg,
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled": engine != nil && engine.IsEnabled(),
		"today":   today,
		"records": records,
	})
}

// PostWorkBuddyCheckin triggers an immediate checkin or compensation claim for one or all WorkBuddy accounts,
// or toggles the automated check-in engine state if {"enabled": bool} is provided.
func (h *Handler) PostWorkBuddyCheckin(c *gin.Context) {
	engine := workbuddy.GetGlobalCheckinEngine()
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

	action := strings.ToLower(strings.TrimSpace(c.Query("action")))
	if action == "" {
		action = strings.ToLower(strings.TrimSpace(req.Action))
	}
	if action == "compensation" || action == "claim_compensation" {
		h.handleCompensationClaim(c, req.AuthID)
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

		res, err := workbuddy.Checkin(ctx, http.DefaultClient, targetAuth)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  err.Error(),
				"result": res,
			})
			return
		}
		if engine != nil && res != nil {
			engine.RecordCheckin(targetAuth.ID, *res)
		}
		c.JSON(http.StatusOK, gin.H{
			"today":  workbuddy.TodayBeijing(),
			"result": res,
		})
		return
	}

	if engine != nil {
		results := engine.RunCheckinAll(ctx)
		c.JSON(http.StatusOK, gin.H{
			"today":   workbuddy.TodayBeijing(),
			"results": results,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"today":   workbuddy.TodayBeijing(),
		"results": map[string]any{},
	})
}

// GetWorkBuddyCompensation returns the activity compensation (e.g. 500 DeepSeek outage credits) claim records.
func (h *Handler) GetWorkBuddyCompensation(c *gin.Context) {
	engine := workbuddy.GetGlobalCheckinEngine()
	authID := strings.TrimSpace(c.Query("auth_id"))

	if authID != "" {
		if engine != nil {
			record, found := engine.GetCompensationStatus(authID)
			if found {
				c.JSON(http.StatusOK, gin.H{
					"found":  true,
					"record": record,
				})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"found": false,
		})
		return
	}

	records := make(map[string]any)
	if engine != nil {
		for k, v := range engine.GetAllCompensations() {
			if isInternalStorageFile(k) || strings.HasSuffix(strings.ToLower(k), "checkins.json") || k == "enabled" || k == "records" || k == "compensations" {
				continue
			}
			records[k] = v
		}
	}

	// Merge all managed WorkBuddy credentials from AuthManager
	if h.authManager != nil {
		for _, a := range h.authManager.List() {
			if a == nil || isInternalStorageFile(a.ID) || isInternalStorageFile(a.FileName) || strings.HasSuffix(strings.ToLower(a.ID), "checkins.json") || a.ID == "enabled" || a.ID == "records" || a.ID == "compensations" {
				continue
			}
			path := strings.TrimSpace(authAttribute(a, "path"))
			if path != "" && isInternalStorageFile(path) {
				continue
			}
			provider := strings.ToLower(strings.TrimSpace(a.Provider))
			authKind := strings.ToLower(strings.TrimSpace(a.Attributes["auth_kind"]))
			isWorkbuddy := provider == "workbuddy" || provider == "workbuddy-cn" || authKind == "workbuddy" || provider == "copilot-tencent"
			if !isWorkbuddy {
				continue
			}

			realm := workbuddy.ResolveRealm(a)
			if existing, exists := records[a.ID]; exists {
				if rec, ok := existing.(workbuddy.CompensationResult); ok && rec.Realm == "" {
					rec.Realm = realm
					records[a.ID] = rec
				}
				continue
			}

			outcome := "pending"
			msg := "待领取官方活动补偿"
			if !strings.EqualFold(realm, workbuddy.RealmCN) {
				outcome = "inactive"
				msg = "国际版活动未开启或已过期"
			}
			user := workbuddy.ExtractNickname(a)
			if user == "" {
				user = a.ID
			}

			records[a.ID] = gin.H{
				"auth_id": a.ID,
				"user":    user,
				"outcome": outcome,
				"realm":   realm,
				"message": msg,
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"compensations": records,
	})
}

// PostWorkBuddyCompensation triggers immediate compensation claim for one or all WorkBuddy accounts.
func (h *Handler) PostWorkBuddyCompensation(c *gin.Context) {
	var req struct {
		AuthID string `json:"auth_id"`
	}
	_ = c.ShouldBindJSON(&req)
	h.handleCompensationClaim(c, req.AuthID)
}

func (h *Handler) handleCompensationClaim(c *gin.Context, bodyAuthID string) {
	engine := workbuddy.GetGlobalCheckinEngine()
	authID := strings.TrimSpace(c.Query("auth_id"))
	if authID == "" && strings.TrimSpace(bodyAuthID) != "" {
		authID = strings.TrimSpace(bodyAuthID)
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

		res, err := workbuddy.ClaimCompensation(ctx, http.DefaultClient, targetAuth)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  err.Error(),
				"result": res,
			})
			return
		}
		if engine != nil && res != nil {
			engine.RecordCompensation(targetAuth.ID, *res)
		}
		c.JSON(http.StatusOK, gin.H{
			"result": res,
		})
		return
	}

	if engine != nil {
		results := engine.ClaimCompensationAll(ctx)
		c.JSON(http.StatusOK, gin.H{
			"results": results,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"results": map[string]any{},
	})
}

// PutWorkBuddyCheckin updates the checkin engine configuration (e.g. enabled state).
func (h *Handler) PutWorkBuddyCheckin(c *gin.Context) {
	engine := workbuddy.GetGlobalCheckinEngine()
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
