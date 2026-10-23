package management

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/pluginhost"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	log "github.com/sirupsen/logrus"
)

// watchOAuthSessionCancel cancels pollCtx once the OAuth session is no longer pending.
func watchOAuthSessionCancel(pollCtx context.Context, cancel context.CancelFunc, state, provider string) {
	if cancel == nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-pollCtx.Done():
			return
		case <-ticker.C:
			if !IsOAuthSessionPending(state, provider) {
				cancel()
				return
			}
		}
	}
}

// CancelAuthSession cancels a pending OAuth session identified by state.
// Protected by management auth. Safe for both callback and device-code flows:
// waiters check IsOAuthSessionPending and exit without saving credentials.
func (h *Handler) CancelAuthSession(c *gin.Context) {
	state := strings.TrimSpace(c.Query("state"))
	if state == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "missing state"})
		return
	}
	if err := ValidateOAuthState(state); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "invalid state"})
		return
	}
	cancelled := CancelOAuthSession(state)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "cancelled": cancelled})
}

func (h *Handler) GetAuthStatus(c *gin.Context) {
	state := strings.TrimSpace(c.Query("state"))
	if state == "" {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return
	}
	if err := ValidateOAuthState(state); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "invalid state"})
		return
	}

	provider, status, isPlugin, metadata, completed, ok := GetOAuthSessionDetails(state)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"status": "error", "error": "unknown or expired state"})
		return
	}
	if completed {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return
	}
	if status != "" {
		c.JSON(http.StatusOK, gin.H{"status": "error", "error": status})
		return
	}
	h.mu.Lock()
	host := h.pluginHost
	h.mu.Unlock()
	if isPlugin && host != nil && host.HasAuthProvider(provider) {
		ctx := PopulateAuthContext(context.Background(), c)
		resp, handled, errPoll := host.PollLogin(ctx, provider, state, metadata)
		if handled {
			if errPoll != nil {
				message := strings.TrimSpace(errPoll.Error())
				if message == "" {
					message = "Authentication failed"
				}
				SetOAuthSessionError(state, message)
				c.JSON(http.StatusOK, gin.H{"status": "error", "error": message})
				return
			}
			switch resp.Status {
			case "", pluginapi.AuthLoginStatusPending:
				c.JSON(http.StatusOK, gin.H{"status": "wait"})
				return
			case pluginapi.AuthLoginStatusError:
				message := strings.TrimSpace(resp.Message)
				if message == "" {
					message = "Authentication failed"
				}
				SetOAuthSessionError(state, message)
				c.JSON(http.StatusOK, gin.H{"status": "error", "error": message})
				return
			case pluginapi.AuthLoginStatusSuccess:
				records := pluginLoginPollAuths(host, resp)
				if len(records) == 0 {
					SetOAuthSessionError(state, "Authentication failed")
					c.JSON(http.StatusOK, gin.H{"status": "error", "error": "Authentication failed"})
					return
				}
				if errSave := h.savePluginLoginRecords(ctx, records); errSave != nil {
					log.WithError(errSave).WithField("provider", provider).Error("failed to save plugin auth tokens")
					SetOAuthSessionError(state, "Failed to save authentication tokens")
					c.JSON(http.StatusOK, gin.H{"status": "error", "error": "Failed to save authentication tokens"})
					return
				}
				CompleteOAuthSession(state)
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
				return
			default:
				c.JSON(http.StatusOK, gin.H{"status": "wait"})
				return
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "wait"})
}

func pluginLoginPollAuths(host *pluginhost.Host, resp pluginapi.AuthLoginPollResponse) []*coreauth.Auth {
	if host == nil {
		return nil
	}
	authDatas := resp.Auths
	if len(authDatas) == 0 {
		authDatas = []pluginapi.AuthData{resp.Auth}
	}
	records := make([]*coreauth.Auth, 0, len(authDatas))
	for _, authData := range authDatas {
		record := host.AuthDataToCoreAuth(authData, "", "")
		if record == nil {
			return nil
		}
		records = append(records, record)
	}
	return records
}

func (h *Handler) savePluginLoginRecords(ctx context.Context, records []*coreauth.Auth) error {
	savedPaths := make([]string, 0, len(records))
	for _, record := range records {
		savedPath, errSave := h.saveTokenRecord(ctx, record)
		if strings.TrimSpace(savedPath) != "" {
			savedPaths = append(savedPaths, savedPath)
		}
		if errSave != nil {
			h.rollbackSavedTokenRecords(ctx, savedPaths)
			return errSave
		}
	}
	return nil
}

func (h *Handler) rollbackSavedTokenRecords(ctx context.Context, savedPaths []string) {
	for i := len(savedPaths) - 1; i >= 0; i-- {
		path := strings.TrimSpace(savedPaths[i])
		if path == "" {
			continue
		}
		if errDelete := h.deleteTokenRecord(ctx, path); errDelete != nil {
			log.WithError(errDelete).WithField("path", path).Warn("failed to roll back plugin auth token")
		}
		h.removeAuthsForPath(ctx, path, path)
	}
}

// PopulateAuthContext extracts request info and adds it to the context
func PopulateAuthContext(ctx context.Context, c *gin.Context) context.Context {
	info := &coreauth.RequestInfo{
		Query:   c.Request.URL.Query(),
		Headers: c.Request.Header,
	}
	return coreauth.WithRequestInfo(ctx, info)
}
