package management

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	workbuddyauth "github.com/router-for-me/CLIProxyAPI/v8/internal/auth/workbuddy"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

func (h *Handler) RequestWorkBuddyToken(c *gin.Context) {
	realm := workbuddyauth.RealmIntl
	if q := strings.TrimSpace(c.Query("realm")); q != "" {
		if strings.EqualFold(q, workbuddyauth.RealmCN) {
			realm = workbuddyauth.RealmCN
		}
	}
	h.requestWorkBuddyTokenWithRealm(c, realm)
}

func (h *Handler) RequestWorkBuddyCNToken(c *gin.Context) {
	h.requestWorkBuddyTokenWithRealm(c, workbuddyauth.RealmCN)
}

func (h *Handler) requestWorkBuddyTokenWithRealm(c *gin.Context, realm string) {
	ctx := context.Background()
	ctx = PopulateAuthContext(ctx, c)

	isCN := strings.EqualFold(realm, workbuddyauth.RealmCN)
	displayName := "WorkBuddy (en)"
	providerName := "workbuddy"
	filePrefix := "workbuddy"
	statePrefix := "wb"
	if isCN {
		displayName = "WorkBuddy (cn)"
		providerName = "workbuddy-cn"
		filePrefix = "workbuddy-cn"
		statePrefix = "wb-cn"
	}

	state := fmt.Sprintf("%s-%d", statePrefix, time.Now().UnixNano())

	client := &http.Client{Timeout: 30 * time.Second}
	if h.cfg != nil {
		sdkCfg := h.cfg.SDKConfig
		client = util.SetProxy(&sdkCfg, client)
	}

	session, errStart := workbuddyauth.StartLogin(ctx, client, realm, "CLI")
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

		bundle, errWait := workbuddyauth.WaitForAuthorization(pollCtx, client, realm, session.State)
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

		tokenStorage := &workbuddyauth.WorkBuddyTokenStorage{
			Type:         providerName,
			AccessToken:  bundle.AccessToken,
			RefreshToken: bundle.RefreshToken,
			Realm:        bundle.Realm,
			Domain:       bundle.Domain,
			BaseURL:      bundle.BaseURL,
			UserID:       bundle.UserID,
		}
		if bundle.ExpiresAt > 0 {
			tokenStorage.Expired = time.Unix(bundle.ExpiresAt, 0).UTC().Format(time.RFC3339)
		}

		metadata := map[string]any{
			"type":          providerName,
			"access_token":  bundle.AccessToken,
			"refresh_token": bundle.RefreshToken,
			"realm":         bundle.Realm,
			"domain":        bundle.Domain,
			"base_url":      bundle.BaseURL,
			"user_id":       bundle.UserID,
			"nickname":      bundle.Nickname,
			"timestamp":     time.Now().UnixMilli(),
		}
		if bundle.ExpiresAt > 0 {
			metadata["expired"] = time.Unix(bundle.ExpiresAt, 0).UTC().Format(time.RFC3339)
		}

		userLabel := displayName
		if strings.TrimSpace(bundle.Nickname) != "" {
			userLabel = fmt.Sprintf("%s (%s)", displayName, strings.TrimSpace(bundle.Nickname))
		} else if strings.TrimSpace(bundle.UserID) != "" {
			userLabel = fmt.Sprintf("%s (%s)", displayName, strings.TrimSpace(bundle.UserID))
		}
		metadata["label"] = userLabel

		fileName := fmt.Sprintf("%s-%d.json", filePrefix, time.Now().UnixMilli())
		record := &coreauth.Auth{
			ID:       fileName,
			Provider: providerName,
			FileName: fileName,
			Label:    userLabel,
			Storage:  tokenStorage,
			Metadata: metadata,
			Attributes: map[string]string{
				"base_url": bundle.BaseURL,
				"realm":    bundle.Realm,
				"domain":   bundle.Domain,
			},
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
		"flow":       "device",
		"expires_in": 600,
	}
	c.JSON(http.StatusOK, response)
}
