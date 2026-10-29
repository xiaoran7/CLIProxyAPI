package management

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	qoderauth "github.com/router-for-me/CLIProxyAPI/v8/internal/auth/qoder"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
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

		// Deduplicate against existing credential files
		var fileName string
		if existing := h.findExistingQoderCredential(providerName, bundle.UserID, bundle.Email, bundle.Nickname); existing != nil && existing.FileName != "" {
			fileName = existing.FileName
			log.Infof("%s: found existing credential file %s for user %s, updating in place", displayName, fileName, bundle.UserID)
		} else if strings.TrimSpace(bundle.UserID) != "" {
			cleanID := sanitizeCredentialSlug(bundle.UserID)
			fileName = fmt.Sprintf("%s-%s.json", filePrefix, cleanID)
		} else if strings.TrimSpace(bundle.Email) != "" {
			cleanEmail := sanitizeCredentialSlug(bundle.Email)
			fileName = fmt.Sprintf("%s-%s.json", filePrefix, cleanEmail)
		} else {
			fileName = fmt.Sprintf("%s-%d.json", filePrefix, time.Now().UnixMilli())
		}

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

// findExistingQoderCredential searches in-memory credentials and auth directory on disk
// for an existing credential matching the Qoder account.
func (h *Handler) findExistingQoderCredential(providerName, userID, email, nickname string) *coreauth.Auth {
	cleanUID := strings.TrimSpace(userID)
	cleanEmail := strings.TrimSpace(email)
	cleanNick := strings.TrimSpace(nickname)

	// 1. Search in-memory authManager if available
	if h != nil && h.authManager != nil {
		for _, a := range h.authManager.List() {
			if a == nil {
				continue
			}
			isQoder := strings.EqualFold(a.Provider, providerName) ||
				strings.HasPrefix(strings.ToLower(a.FileName), providerName+"-") ||
				strings.HasPrefix(strings.ToLower(a.ID), providerName+"-")
			if !isQoder {
				continue
			}

			// Match by user_id
			if cleanUID != "" {
				if a.Attributes != nil && a.Attributes["user_id"] == cleanUID {
					return a
				}
				if a.Metadata != nil {
					if uid, ok := a.Metadata["user_id"].(string); ok && uid == cleanUID {
						return a
					}
					if uid, ok := a.Metadata["uid"].(string); ok && uid == cleanUID {
						return a
					}
				}
				if ts, ok := a.Storage.(*qoderauth.QoderTokenStorage); ok && ts != nil && ts.UserID == cleanUID {
					return a
				}
			}

			// Match by email
			if cleanEmail != "" {
				if a.Attributes != nil && strings.EqualFold(a.Attributes["email"], cleanEmail) {
					return a
				}
				if a.Metadata != nil {
					if em, ok := a.Metadata["email"].(string); ok && strings.EqualFold(em, cleanEmail) {
						return a
					}
				}
				if ts, ok := a.Storage.(*qoderauth.QoderTokenStorage); ok && ts != nil && strings.EqualFold(ts.Email, cleanEmail) {
					return a
				}
			}

			// Match by nickname if user_id and email are empty
			if cleanUID == "" && cleanEmail == "" && cleanNick != "" {
				if a.Attributes != nil && strings.EqualFold(a.Attributes["nickname"], cleanNick) {
					return a
				}
				if a.Metadata != nil {
					if nick, ok := a.Metadata["nickname"].(string); ok && strings.EqualFold(nick, cleanNick) {
						return a
					}
				}
				if ts, ok := a.Storage.(*qoderauth.QoderTokenStorage); ok && ts != nil && strings.EqualFold(ts.Nickname, cleanNick) {
					return a
				}
			}
		}
	}

	// 2. Search on disk in AuthDir if authManager did not find a match
	if h != nil && h.cfg != nil && strings.TrimSpace(h.cfg.AuthDir) != "" {
		entries, err := os.ReadDir(h.cfg.AuthDir)
		if err == nil {
			prefixLower := strings.ToLower(providerName) + "-"
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
					continue
				}
				nameLower := strings.ToLower(entry.Name())
				if !strings.HasPrefix(nameLower, prefixLower) && !strings.HasPrefix(nameLower, "qoder-") {
					continue
				}
				filePath := filepath.Join(h.cfg.AuthDir, entry.Name())
				data, errRead := os.ReadFile(filePath)
				if errRead != nil || len(data) == 0 {
					continue
				}
				parsed := gjson.ParseBytes(data)
				fileUID := strings.TrimSpace(parsed.Get("user_id").String())
				if fileUID == "" {
					fileUID = strings.TrimSpace(parsed.Get("uid").String())
				}
				fileEmail := strings.TrimSpace(parsed.Get("email").String())
				fileNick := strings.TrimSpace(parsed.Get("nickname").String())

				if cleanUID != "" && fileUID == cleanUID {
					return &coreauth.Auth{
						ID:       entry.Name(),
						FileName: entry.Name(),
						Provider: providerName,
					}
				}
				if cleanEmail != "" && strings.EqualFold(fileEmail, cleanEmail) {
					return &coreauth.Auth{
						ID:       entry.Name(),
						FileName: entry.Name(),
						Provider: providerName,
					}
				}
				if cleanUID == "" && cleanEmail == "" && cleanNick != "" && strings.EqualFold(fileNick, cleanNick) {
					return &coreauth.Auth{
						ID:       entry.Name(),
						FileName: entry.Name(),
						Provider: providerName,
					}
				}
			}
		}
	}

	return nil
}

// sanitizeCredentialSlug sanitizes an account identifier to safe alphanumeric, dot, underscore, dash.
func sanitizeCredentialSlug(s string) string {
	s = strings.TrimSpace(s)
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == '@' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	res := strings.Trim(sb.String(), "._-")
	if len(res) > 48 {
		res = res[:48]
	}
	if res == "" {
		res = "account"
	}
	return res
}
