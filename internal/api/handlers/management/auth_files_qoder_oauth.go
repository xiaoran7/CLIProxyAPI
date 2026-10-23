package management

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	qoderauth "github.com/router-for-me/CLIProxyAPI/v8/internal/auth/qoder"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

// RequestQoderToken handles Qoder OAuth device login flow (international realm).
func (h *Handler) RequestQoderToken(c *gin.Context) {
	realm := qoderauth.RealmIntl
	if q := strings.TrimSpace(c.Query("realm")); q != "" {
		if strings.EqualFold(q, qoderauth.RealmCN) {
			realm = qoderauth.RealmCN
		}
	}
	h.requestQoderTokenWithRealm(c, realm)
}

// RequestQoderCNToken handles Qoder Mainland China OAuth device login flow.
func (h *Handler) RequestQoderCNToken(c *gin.Context) {
	h.requestQoderTokenWithRealm(c, qoderauth.RealmCN)
}

func (h *Handler) requestQoderTokenWithRealm(c *gin.Context, realm string) {
	ctx := context.Background()
	ctx = PopulateAuthContext(ctx, c)

	isCN := strings.EqualFold(realm, qoderauth.RealmCN)
	displayName := "Qoder (en)"
	providerName := "qoder"
	filePrefix := "qoder"
	statePrefix := "qoder"
	if isCN {
		displayName = "Qoder (cn)"
		providerName = "qoder-cn"
		filePrefix = "qoder-cn"
		statePrefix = "qoder-cn"
	}

	state := fmt.Sprintf("%s-%d", statePrefix, time.Now().UnixNano())

	client := &http.Client{Timeout: 30 * time.Second}
	if h.cfg != nil {
		sdkCfg := h.cfg.SDKConfig
		client = util.SetProxy(&sdkCfg, client)
	}

	session, errStart := qoderauth.StartLogin(realm)
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

		bundle, errWait := qoderauth.WaitForAuthorization(pollCtx, client, realm, session.Nonce, session.Verifier, session.MachineID)
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

		tokenStorage := &qoderauth.QoderTokenStorage{
			Type:          providerName,
			AccessToken:   bundle.AccessToken,
			RefreshToken:  bundle.RefreshToken,
			DeviceToken:   bundle.DeviceToken,
			DeviceRefresh: bundle.DeviceRefresh,
			JobToken:      bundle.JobToken,
			JobRefresh:    bundle.JobRefresh,
			Realm:         bundle.Realm,
			BaseURL:       bundle.BaseURL,
			OpenAPIBase:   bundle.OpenAPIBase,
			UserID:        bundle.UserID,
			Nickname:      bundle.Nickname,
			Email:         bundle.Email,
			MachineID:     bundle.MachineID,
			DeviceChat:    bundle.DeviceChat,
		}
		if bundle.ExpiresAt > 0 {
			tokenStorage.Expired = time.Unix(bundle.ExpiresAt, 0).UTC().Format(time.RFC3339)
		}

		metadata := map[string]any{
			"type":           providerName,
			"access_token":   bundle.AccessToken,
			"refresh_token":  bundle.RefreshToken,
			"device_token":   bundle.DeviceToken,
			"device_refresh": bundle.DeviceRefresh,
			"job_token":      bundle.JobToken,
			"job_refresh":    bundle.JobRefresh,
			"realm":          bundle.Realm,
			"base_url":       bundle.BaseURL,
			"openapi_base":   bundle.OpenAPIBase,
			"user_id":        bundle.UserID,
			"nickname":       bundle.Nickname,
			"email":          bundle.Email,
			"machine_id":     bundle.MachineID,
			"device_chat":    bundle.DeviceChat,
			"timestamp":      time.Now().UnixMilli(),
		}
		if bundle.ExpiresAt > 0 {
			metadata["expired"] = time.Unix(bundle.ExpiresAt, 0).UTC().Format(time.RFC3339)
		}

		userLabel := displayName
		if strings.TrimSpace(bundle.Email) != "" {
			userLabel = fmt.Sprintf("%s (%s)", displayName, strings.TrimSpace(bundle.Email))
		} else if strings.TrimSpace(bundle.Nickname) != "" {
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
				"user_id":  bundle.UserID,
				"email":    bundle.Email,
				"nickname": bundle.Nickname,
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
