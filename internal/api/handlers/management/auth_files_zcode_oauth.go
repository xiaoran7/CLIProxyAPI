package management

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	zcodeauth "github.com/router-for-me/CLIProxyAPI/v8/internal/auth/zcode"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

// RequestZCodeToken starts ZCode's CLI sign-in flow (international Z.ai by default).
func (h *Handler) RequestZCodeToken(c *gin.Context) {
	site := zcodeauth.SiteZAI
	if q := strings.TrimSpace(c.Query("site")); q != "" {
		switch strings.ToLower(q) {
		case zcodeauth.SiteBigModel, "zcode-bigmodel", "zcode-cn", "cn":
			site = zcodeauth.SiteBigModel
		default:
			site = zcodeauth.SiteZAI
		}
	} else if p := strings.TrimSpace(c.Query("provider")); p != "" {
		switch strings.ToLower(p) {
		case "zcode-cn", "zcode-bigmodel", "bigmodel", "zcode_cn":
			site = zcodeauth.SiteBigModel
		default:
			site = zcodeauth.SiteZAI
		}
	}
	h.requestZCodeTokenWithSite(c, site)
}

// RequestZCodeCNToken starts ZCode Mainland China (BigModel / 智谱) CLI sign-in flow.
func (h *Handler) RequestZCodeCNToken(c *gin.Context) {
	h.requestZCodeTokenWithSite(c, zcodeauth.SiteBigModel)
}

func (h *Handler) requestZCodeTokenWithSite(c *gin.Context, site string) {
	ctx := context.Background()
	ctx = PopulateAuthContext(ctx, c)

	providerName := "zcode"
	displayName := "ZCode (Z.ai)"
	if site == zcodeauth.SiteBigModel {
		providerName = "zcode-cn"
		displayName = "BigModel (智谱)"
	}

	state := fmt.Sprintf("%s-%d", providerName, time.Now().UnixNano())

	client := &http.Client{Timeout: 30 * time.Second}
	if h.cfg != nil {
		sdkCfg := h.cfg.SDKConfig
		client = util.SetProxy(&sdkCfg, client)
	}

	session, errStart := zcodeauth.StartLogin(site)
	if errStart != nil {
		log.Errorf("Failed to generate %s authorization URL: %v", displayName, errStart)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate authorization url"})
		return
	}

	RegisterOAuthSession(state, providerName)

	go func() {
		pollCtx, cancelPoll := context.WithCancel(ctx)
		defer cancelPoll()
		go watchOAuthSessionCancel(pollCtx, cancelPoll, state, providerName)

		bundle, errWait := zcodeauth.WaitForAuthorization(pollCtx, client, session)
		if errWait != nil {
			if !IsOAuthSessionPending(state, providerName) {
				return
			}
			SetOAuthSessionError(state, oauthSessionErrorWithCause("Authentication failed", errWait))
			log.Errorf("%s authentication failed: %v", displayName, errWait)
			return
		}
		if !IsOAuthSessionPending(state, providerName) {
			return
		}

		tokenStorage := &zcodeauth.TokenStorage{
			Type:        providerName,
			APIKey:      bundle.Key,
			StartToken:  bundle.StartToken,
			SiteToken:   bundle.SiteToken,
			Site:        bundle.Site,
			BaseURL:     bundle.BaseURL,
			Plan:        bundle.Plan,
			Team:        bundle.Team,
			TeamOrg:     bundle.TeamOrg,
			TeamProject: bundle.TeamProject,
			DeviceID:    bundle.Device,
			TrialModels: bundle.TrialModels,
			UserID:      bundle.UserID,
			Email:       bundle.Email,
			Nickname:    bundle.Name,
		}
		if bundle.ExpiresAt > 0 {
			tokenStorage.Expired = time.Unix(bundle.ExpiresAt, 0).UTC().Format(time.RFC3339)
		}

		metadata := map[string]any{
			"type":         providerName,
			"site":         bundle.Site,
			"base_url":     bundle.BaseURL,
			"plan":         bundle.Plan,
			"team":         bundle.Team,
			"device_id":    bundle.Device,
			"user_id":      bundle.UserID,
			"email":        bundle.Email,
			"nickname":     bundle.Name,
			"timestamp":    time.Now().UnixMilli(),
			"trial_models": bundle.TrialModels,
		}
		if bundle.Key != "" {
			metadata["api_key"] = bundle.Key
		}
		if bundle.StartToken != "" {
			metadata["zcode_token"] = bundle.StartToken
		}
		if bundle.SiteToken != "" {
			metadata["site_token"] = bundle.SiteToken
		}
		if bundle.TeamOrg != "" {
			metadata["team_org"] = bundle.TeamOrg
		}
		if bundle.TeamProject != "" {
			metadata["team_project"] = bundle.TeamProject
		}
		if bundle.ExpiresAt > 0 {
			metadata["expired"] = time.Unix(bundle.ExpiresAt, 0).UTC().Format(time.RFC3339)
		}

		userLabel := displayName
		if strings.TrimSpace(bundle.Email) != "" {
			userLabel = fmt.Sprintf("%s (%s)", displayName, strings.TrimSpace(bundle.Email))
		} else if strings.TrimSpace(bundle.Name) != "" {
			userLabel = fmt.Sprintf("%s (%s)", displayName, strings.TrimSpace(bundle.Name))
		} else if strings.TrimSpace(bundle.UserID) != "" {
			userLabel = fmt.Sprintf("%s (%s)", displayName, strings.TrimSpace(bundle.UserID))
		}
		metadata["label"] = userLabel

		fileName := fmt.Sprintf("%s-%d.json", providerName, time.Now().UnixMilli())
		record := &coreauth.Auth{
			ID:       fileName,
			Provider: providerName,
			FileName: fileName,
			Label:    userLabel,
			Storage:  tokenStorage,
			Metadata: metadata,
			Attributes: map[string]string{
				"site":      bundle.Site,
				"base_url":  bundle.BaseURL,
				"plan":      bundle.Plan,
				"device_id": bundle.Device,
				"user_id":   bundle.UserID,
				"email":     bundle.Email,
				"nickname":  bundle.Name,
			},
		}
		if bundle.Team {
			record.Attributes["team"] = "true"
		}

		if errGuard := guardOAuthSessionPendingForSave(state, providerName); errGuard != nil {
			return
		}
		savedPath, errSave := h.saveTokenRecord(ctx, record)
		if errSave != nil {
			log.Errorf("Failed to save %s tokens: %v", displayName, errSave)
			SetOAuthSessionError(state, "Failed to save authentication tokens")
			return
		}

		log.Infof("%s authentication successful! Token saved to %s", displayName, savedPath)
		CompleteOAuthSession(state)
	}()

	response := gin.H{
		"status":     "ok",
		"url":        session.AuthURL,
		"state":      state,
		"flow":       "cli",
		"expires_in": int(time.Until(session.Deadline).Seconds()),
	}
	c.JSON(http.StatusOK, response)
}
