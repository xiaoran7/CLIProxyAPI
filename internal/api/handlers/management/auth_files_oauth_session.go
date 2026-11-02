package management

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/pluginhost"
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v8/sdk/auth"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	log "github.com/sirupsen/logrus"
)

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

	session, ok := oauthSessions.Get(state)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"status": "error", "error": "unknown or expired state"})
		return
	}
	if session.Completed {
		resp := gin.H{"status": "ok"}
		if session.AuthID != "" {
			resp["auth_id"] = session.AuthID
			resp["saved"] = session.Saved
			var adoptedAuth *coreauth.Auth
			var adoptErr string
			if h != nil && h.authManager != nil {
				if candidate, okAuth := h.authManager.GetByID(session.AuthID); okAuth && candidate != nil {
					if session.SavedGrantHash != "" {
						if authGrantFingerprint(candidate) == session.SavedGrantHash ||
							candidate.AdoptedGrantHash == session.SavedGrantHash {
							adoptedAuth = candidate
							if !session.Adopted {
								oauthSessions.MarkAdopted(state, candidate.CredentialVersion, candidate.RegistrationEpoch)
								session.Adopted = true
								session.AdoptedVersion = candidate.CredentialVersion
								session.AdoptedEpoch = candidate.RegistrationEpoch
								session.AdoptError = ""
							}
						} else if session.Adopted {
							adoptErr = "Credential was replaced in active runtime after login"
						}
					} else if session.Adopted {
						if (session.AdoptedVersion == 0 && session.AdoptedEpoch == 0) ||
							(candidate.RegistrationEpoch == session.AdoptedEpoch && candidate.CredentialVersion == session.AdoptedVersion) {
							adoptedAuth = candidate
						}
					}
				}
			}
			if adoptErr == "" {
				adoptErr = session.AdoptError
			}
			if adoptedAuth != nil {
				nowUTC := time.Now().UTC()
				resp["adopted"] = true
				resp["recovery_stage"] = adoptedAuth.RecoveryStageAt(nowUTC)
				resp["renewal_state"] = adoptedAuth.RenewalStateAt(nowUTC)
				resp["requires_sign_in"] = adoptedAuth.RequiresSignIn
				resp["inference_verified"] = adoptedAuth.InferenceVerified
				resp["renewal_verified"] = adoptedAuth.RenewalVerified
				resp["quota_blocked"] = adoptedAuth.RecoveryStageAt(nowUTC) == coreauth.RecoveryStageQuotaBlocked
			} else if session.Saved {
				resp["adopted"] = false
				resp["recovery_stage"] = coreauth.RecoveryStageCredentialSaved
				if adoptErr != "" {
					resp["adoption_error"] = adoptErr
				}
			}
			if session.VerifyError != "" {
				resp["verification_error"] = session.VerifyError
			}
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	if session.Status != "" {
		c.JSON(http.StatusOK, gin.H{"status": "error", "error": session.Status})
		return
	}
	if session.Committing || session.SIWCCommitting {
		c.JSON(http.StatusOK, gin.H{"status": "wait"})
		return
	}
	provider := session.Provider
	isPlugin := session.Source == oauthSessionSourcePlugin
	metadata := session.Metadata
	sessionProxyURL := strings.TrimSpace(session.ProxyURL)
	h.mu.Lock()
	host := h.pluginHost
	h.mu.Unlock()
	if isPlugin && host != nil && host.HasAuthProvider(provider) {
		ctx := PopulateAuthContext(context.Background(), c)
		if sessionProxyURL != "" {
			ctx = pluginhost.WithOAuthProxyURL(ctx, sessionProxyURL)
		}
		resp, handled, errPoll := host.PollLogin(ctx, provider, state, metadata)
		if handled {
			if errPoll != nil {
				message := redactSessionProxyInString(strings.TrimSpace(errPoll.Error()), sessionProxyURL)
				writePluginPollFailureStatus(c, state, message)
				return
			}
			switch resp.Status {
			case "", pluginapi.AuthLoginStatusPending:
				writeNonCommitPluginPollStatus(c, state, true)
				return
			case pluginapi.AuthLoginStatusError:
				message := redactSessionProxyInString(strings.TrimSpace(resp.Message), sessionProxyURL)
				writePluginPollFailureStatus(c, state, message)
				return
			case pluginapi.AuthLoginStatusSuccess:
				if errGuard := claimOAuthSessionCommit(state, provider); errGuard != nil {
					writeNonCommitPluginPollStatus(c, state, false)
					return
				}
				records := pluginLoginPollAuthsWithProvider(host, provider, resp)
				if len(records) == 0 {
					FailOAuthSessionCommit(state, "Authentication failed")
					c.JSON(http.StatusOK, gin.H{"status": "error", "error": "Authentication failed"})
					return
				}
				if target, okTarget := GetOAuthSessionTarget(state); okTarget && target.AuthID != "" {
					if !applyPluginOAuthRecoveryTarget(target, provider, records) {
						FailOAuthSessionCommit(state, "Authenticated account does not match target credential")
						c.JSON(http.StatusOK, gin.H{"status": "error", "error": "Authenticated account does not match target credential"})
						return
					}
				}
				for _, record := range records {
					applySessionProxyToAuthRecord(record, sessionProxyURL)
				}
				if errSave := h.savePluginLoginRecords(ctx, records); errSave != nil {
					log.WithError(errSave).WithField("provider", provider).Error("failed to save plugin auth tokens")
					FailOAuthSessionCommit(state, "Failed to save authentication tokens")
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

func writePluginPollFailureStatus(c *gin.Context, state, message string) {
	errStatus, completed, committing, ok := oauthSessions.FailPendingPoll(state, message)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"status": "error", "error": "unknown or expired state"})
		return
	}
	if completed {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return
	}
	if committing {
		c.JSON(http.StatusOK, gin.H{"status": "wait"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "error", "error": errStatus})
}

func writeNonCommitPluginPollStatus(c *gin.Context, state string, allowPendingWait bool) {
	session, ok := oauthSessions.Get(state)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"status": "error", "error": "unknown or expired state"})
		return
	}
	if session.Completed {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return
	}
	if session.Status != "" {
		c.JSON(http.StatusOK, gin.H{"status": "error", "error": session.Status})
		return
	}
	if session.Committing || session.SIWCCommitting || allowPendingWait {
		c.JSON(http.StatusOK, gin.H{"status": "wait"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "error", "error": "unknown or expired state"})
}

func pluginLoginPollAuths(host *pluginhost.Host, resp pluginapi.AuthLoginPollResponse) []*coreauth.Auth {
	return pluginLoginPollAuthsWithProvider(host, "", resp)
}

func pluginLoginPollAuthsWithProvider(host *pluginhost.Host, provider string, resp pluginapi.AuthLoginPollResponse) []*coreauth.Auth {
	if host == nil {
		return nil
	}
	authDatas := resp.Auths
	if len(authDatas) == 0 {
		authDatas = []pluginapi.AuthData{resp.Auth}
	}
	records := make([]*coreauth.Auth, 0, len(authDatas))
	trimmedSessionProvider := strings.ToLower(strings.TrimSpace(provider))
	for _, authData := range authDatas {
		if strings.TrimSpace(authData.Provider) == "" && trimmedSessionProvider != "" {
			authData.Provider = trimmedSessionProvider
		}
		effectivePortal := trimmedSessionProvider
		for _, key := range []string{"oauth_provider", "portal"} {
			if authData.Attributes != nil {
				if v := strings.ToLower(strings.TrimSpace(authData.Attributes[key])); v != "" {
					effectivePortal = v
					break
				}
			}
			if authData.Metadata != nil {
				if raw, ok := authData.Metadata[key].(string); ok {
					if v := strings.ToLower(strings.TrimSpace(raw)); v != "" {
						effectivePortal = v
						break
					}
				}
			}
		}
		if effectivePortal != "" && !strings.EqualFold(strings.TrimSpace(authData.Provider), effectivePortal) {
			if authData.Metadata == nil {
				authData.Metadata = make(map[string]any)
			}
			if _, exists := authData.Metadata["oauth_provider"]; !exists {
				authData.Metadata["oauth_provider"] = effectivePortal
			}
			if authData.Attributes == nil {
				authData.Attributes = make(map[string]string)
			}
			if strings.TrimSpace(authData.Attributes["oauth_provider"]) == "" {
				authData.Attributes["oauth_provider"] = effectivePortal
			}
		}
		record := host.AuthDataToCoreAuth(authData, "", "")
		if record == nil {
			return nil
		}
		records = append(records, record)
	}
	return records
}

func authIdentityEmailAndAccountID(auth *coreauth.Auth) (email string, accountID string) {
	if auth == nil {
		return "", ""
	}
	if auth.Metadata != nil {
		accountID, _ = auth.Metadata["account_id"].(string)
	}
	if strings.TrimSpace(accountID) == "" && auth.Attributes != nil {
		accountID = auth.Attributes["account_id"]
	}
	accountID = strings.TrimSpace(accountID)
	email = strings.TrimSpace(authEmail(auth))
	if (accountID == "" || email == "") && auth.Storage != nil {
		if rawer, ok := auth.Storage.(interface{ RawJSON() []byte }); ok {
			if raw := rawer.RawJSON(); len(raw) > 0 {
				var rawMap map[string]any
				if json.Unmarshal(raw, &rawMap) == nil {
					if accountID == "" {
						rawAcc, _ := rawMap["account_id"].(string)
						accountID = strings.TrimSpace(rawAcc)
					}
					if email == "" {
						rawEmail, _ := rawMap["email"].(string)
						email = strings.TrimSpace(rawEmail)
					}
				}
			}
		}
	}
	return email, accountID
}

func applyPluginOAuthRecoveryTarget(target oauthRecoveryTarget, provider string, records []*coreauth.Auth) bool {
	if len(records) != 1 || records[0] == nil {
		return false
	}
	record := records[0]
	gotEmail, gotAccountID := authIdentityEmailAndAccountID(record)
	candidateFileName := firstNonEmpty(strings.TrimSpace(record.FileName), strings.TrimSpace(record.ID))

	if !matchesOAuthRecoveryTarget(target, gotEmail, gotAccountID, candidateFileName) {
		return false
	}

	record.ID = target.AuthID
	targetFileName := strings.TrimSpace(target.FileName)
	if targetFileName == "" {
		targetFileName = target.AuthID
	}
	targetPath := strings.TrimSpace(target.Path)
	if targetPath == "" && strings.ContainsAny(target.AuthID, "/\\") {
		targetFileName = target.AuthID
	}
	record.FileName = targetFileName
	if record.Attributes == nil {
		record.Attributes = make(map[string]string)
	}
	if targetPath != "" {
		record.Attributes[coreauth.AttributePath] = targetPath
		record.Attributes[coreauth.AttributeSource] = targetPath
	} else {
		delete(record.Attributes, coreauth.AttributePath)
		record.Attributes[coreauth.AttributeSource] = targetFileName
	}
	if trimmedProvider := strings.ToLower(strings.TrimSpace(provider)); trimmedProvider != "" {
		if record.Metadata == nil {
			record.Metadata = make(map[string]any)
		}
		record.Metadata["oauth_provider"] = trimmedProvider
		record.Attributes["oauth_provider"] = trimmedProvider
		if setter, ok := record.Storage.(interface{ SetMetadata(map[string]any) }); ok {
			setter.SetMetadata(record.Metadata)
		}
	}
	return true
}

type savedPluginRecordRollback struct {
	savedPath          string
	diskPath           string
	existedOnDisk      bool
	prevDiskBytes      []byte
	savedDiskBytes     []byte
	hadRuntimeAuth     bool
	hadRecordIDAuth    bool
	hadFileDerivedAuth bool
	prevAuth           *coreauth.Auth
	record             *coreauth.Auth
}

func (h *Handler) candidatePluginRecordPath(record *coreauth.Auth) string {
	if h == nil || record == nil {
		return ""
	}
	if record.Attributes != nil {
		if p := strings.TrimSpace(record.Attributes[coreauth.AttributePath]); p != "" {
			return p
		}
	}
	name := firstNonEmpty(strings.TrimSpace(record.FileName), strings.TrimSpace(record.ID))
	if name == "" {
		return ""
	}
	if filepath.IsAbs(name) {
		return name
	}
	if cfg := h.configSnapshot(); cfg != nil && strings.TrimSpace(cfg.AuthDir) != "" {
		return filepath.Join(cfg.AuthDir, name)
	}
	return ""
}

func (h *Handler) savePluginLoginRecords(ctx context.Context, records []*coreauth.Auth) error {
	if h != nil {
		h.pluginLoginSaveMu.Lock()
		defer h.pluginLoginSaveMu.Unlock()
	}
	rollbacks := make([]savedPluginRecordRollback, 0, len(records))
	for _, record := range records {
		var rb savedPluginRecordRollback
		candidatePath := h.candidatePluginRecordPath(record)
		if candidatePath != "" {
			prevBytes, errRead := os.ReadFile(candidatePath)
			if errRead == nil {
				rb.existedOnDisk = true
				rb.prevDiskBytes = prevBytes
			} else if !os.IsNotExist(errRead) {
				h.rollbackSavedPluginLoginRecords(ctx, rollbacks)
				return fmt.Errorf("read existing auth file %s: %w", candidatePath, errRead)
			}
		}
		if h != nil && h.authManager != nil && record != nil {
			if recID := strings.TrimSpace(record.ID); recID != "" {
				if _, ok := h.authManager.GetByID(recID); ok {
					rb.hadRecordIDAuth = true
				}
			}
			if fileID := h.authIDForPath(candidatePath); fileID != "" {
				if _, ok := h.authManager.GetByID(fileID); ok {
					rb.hadFileDerivedAuth = true
				}
			}
			for _, candidateID := range []string{strings.TrimSpace(record.ID), h.authIDForPath(candidatePath)} {
				if candidateID == "" {
					continue
				}
				if existing, ok := h.authManager.GetByID(candidateID); ok && existing != nil {
					rb.hadRuntimeAuth = true
					rb.prevAuth = existing.Clone()
					break
				}
			}
		}
		savedPath, errSave := h.saveTokenRecord(ctx, record)
		rb.savedPath = strings.TrimSpace(savedPath)
		rb.diskPath = firstNonEmpty(rb.savedPath, candidatePath)
		rb.record = record
		if record != nil && record.Storage != nil {
			if rawer, ok := record.Storage.(interface{ RawJSON() []byte }); ok {
				rb.savedDiskBytes = rawer.RawJSON()
			}
		}
		if rb.savedPath != "" || rb.existedOnDisk || rb.hadRuntimeAuth {
			rollbacks = append(rollbacks, rb)
		}
		if errSave != nil {
			h.rollbackSavedPluginLoginRecords(ctx, rollbacks)
			return errSave
		}
	}
	return nil
}

func jsonPayloadsEqualOrBytesEqual(left, right []byte) bool {
	if strings.TrimSpace(string(left)) == strings.TrimSpace(string(right)) {
		return true
	}
	var leftVal, rightVal any
	if json.Unmarshal(left, &leftVal) != nil || json.Unmarshal(right, &rightVal) != nil {
		return false
	}
	return reflect.DeepEqual(leftVal, rightVal)
}

var errPluginRollbackSuperseded = errors.New("plugin login rollback superseded")

func (h *Handler) restoreOverwrittenPluginRecord(ctx context.Context, rb savedPluginRecordRollback, diskPath string) {
	if diskPath != "" && len(rb.prevDiskBytes) > 0 {
		if errWrite := os.WriteFile(diskPath, rb.prevDiskBytes, 0o600); errWrite != nil {
			log.WithError(errWrite).WithField("path", diskPath).Warn("failed to restore overwritten plugin auth token")
		}
	}
	if h == nil || (rb.prevAuth == nil && len(rb.prevDiskBytes) == 0) {
		return
	}
	store := h.tokenStoreWithBaseDir()
	if store == nil {
		return
	}
	if _, isFileStore := store.(*sdkAuth.FileTokenStore); isFileStore && diskPath != "" && len(rb.prevDiskBytes) > 0 {
		return
	}
	var diskMeta map[string]any
	if len(rb.prevDiskBytes) > 0 {
		_ = json.Unmarshal(rb.prevDiskBytes, &diskMeta)
	}
	var restoreRecord *coreauth.Auth
	if rb.prevAuth != nil {
		restoreRecord = rb.prevAuth.Clone()
		if len(diskMeta) > 0 {
			restoreRecord.Metadata = diskMeta
		}
	} else {
		provider := ""
		if diskMeta != nil {
			provider, _ = diskMeta["type"].(string)
		}
		if provider == "" && rb.record != nil {
			provider = rb.record.Provider
		}
		fileName := filepath.Base(diskPath)
		id := h.authIDForPath(diskPath)
		if rb.record != nil {
			if fileName == "" || fileName == "." {
				fileName = rb.record.FileName
			}
			if id == "" {
				id = rb.record.ID
			}
		}
		restoreRecord = &coreauth.Auth{
			ID:       firstNonEmpty(id, fileName),
			FileName: fileName,
			Provider: provider,
			Metadata: diskMeta,
			Attributes: map[string]string{
				coreauth.AttributePath: diskPath,
			},
		}
	}
	if len(diskMeta) > 0 {
		if disabled, ok := diskMeta["disabled"].(bool); ok {
			restoreRecord.Disabled = disabled
			if disabled {
				restoreRecord.Status = coreauth.StatusDisabled
			} else if restoreRecord.Status == coreauth.StatusDisabled {
				restoreRecord.Status = coreauth.StatusActive
			}
		}
	}
	if len(rb.prevDiskBytes) > 0 && h.pluginHost != nil {
		if restored := h.pluginHost.AuthDataToCoreAuth(pluginapi.AuthData{
			Provider:    restoreRecord.Provider,
			ID:          restoreRecord.ID,
			FileName:    restoreRecord.FileName,
			Label:       restoreRecord.Label,
			Prefix:      restoreRecord.Prefix,
			ProxyURL:    restoreRecord.ProxyURL,
			Disabled:    restoreRecord.Disabled,
			StorageJSON: rb.prevDiskBytes,
			Metadata:    restoreRecord.Metadata,
			Attributes:  restoreRecord.Attributes,
		}, diskPath, restoreRecord.FileName); restored != nil {
			restoreRecord.Storage = restored.Storage
		}
	}
	if _, errRestore := store.Save(ctx, restoreRecord); errRestore != nil {
		log.WithError(errRestore).WithField("path", diskPath).Warn("failed to restore overwritten plugin auth token in store")
	}
}

func (h *Handler) rollbackSavedPluginLoginRecords(ctx context.Context, rollbacks []savedPluginRecordRollback) {
	for i := len(rollbacks) - 1; i >= 0; i-- {
		rb := rollbacks[i]
		diskPath := strings.TrimSpace(rb.diskPath)
		savedPath := strings.TrimSpace(rb.savedPath)
		if rb.hadRuntimeAuth && rb.prevAuth != nil && h != nil && h.authManager != nil {
			_, _, errModify := h.authManager.Modify(coreauth.WithSkipPersist(ctx), rb.prevAuth.ID, 0, func(current *coreauth.Auth) error {
				currFP := authGrantFingerprint(current)
				if currFP != "" && currFP != authGrantFingerprint(rb.record) && currFP != authGrantFingerprint(rb.prevAuth) {
					return errPluginRollbackSuperseded
				}
				if rb.existedOnDisk && diskPath != "" {
					currentBytes, errRead := os.ReadFile(diskPath)
					if errRead != nil {
						return errPluginRollbackSuperseded
					}
					matchesSaved := len(rb.savedDiskBytes) == 0 || jsonPayloadsEqualOrBytesEqual(currentBytes, rb.savedDiskBytes)
					if !matchesSaved && !jsonPayloadsEqualOrBytesEqual(currentBytes, rb.prevDiskBytes) {
						return errPluginRollbackSuperseded
					}
					if matchesSaved {
						h.restoreOverwrittenPluginRecord(ctx, rb, diskPath)
					}
				} else if savedPath != "" {
					if diskPath != "" {
						if currentBytes, errRead := os.ReadFile(diskPath); errRead == nil && len(rb.savedDiskBytes) > 0 && !jsonPayloadsEqualOrBytesEqual(currentBytes, rb.savedDiskBytes) {
							return errPluginRollbackSuperseded
						}
					}
					h.restoreOverwrittenPluginRecord(ctx, rb, diskPath)
				}
				*current = *rb.prevAuth.Clone()
				return nil
			})
			if !errors.Is(errModify, errPluginRollbackSuperseded) {
				removeNewID := func(extraID string) {
					if extraID == "" || extraID == rb.prevAuth.ID {
						return
					}
					_ = h.authManager.RemoveIf(ctx, extraID, func(current *coreauth.Auth) bool {
						if current == nil {
							return false
						}
						currFP := authGrantFingerprint(current)
						return currFP == "" || currFP == authGrantFingerprint(rb.record)
					})
				}
				if !rb.hadFileDerivedAuth {
					removeNewID(h.authIDForPath(savedPath))
				}
				if !rb.hadRecordIDAuth && rb.record != nil {
					removeNewID(strings.TrimSpace(rb.record.ID))
				}
			}
			continue
		}

		if !rb.hadRuntimeAuth && h != nil && h.authManager != nil && rb.record != nil {
			primaryID := firstNonEmpty(h.authIDForPath(savedPath), strings.TrimSpace(rb.record.ID))
			if primaryID != "" {
				superseded := false
				diskHandled := false
				removeMatching := func(current *coreauth.Auth) bool {
					if current != nil {
						currFP := authGrantFingerprint(current)
						if currFP != "" && currFP != authGrantFingerprint(rb.record) {
							superseded = true
							return false
						}
					}
					if !diskHandled {
						if rb.existedOnDisk && diskPath != "" {
							if currentBytes, errRead := os.ReadFile(diskPath); errRead == nil {
								matchesSaved := len(rb.savedDiskBytes) == 0 || jsonPayloadsEqualOrBytesEqual(currentBytes, rb.savedDiskBytes)
								if !matchesSaved && !jsonPayloadsEqualOrBytesEqual(currentBytes, rb.prevDiskBytes) {
									superseded = true
									return false
								}
								if matchesSaved {
									h.restoreOverwrittenPluginRecord(ctx, rb, diskPath)
								}
							}
						} else if savedPath != "" {
							if diskPath != "" {
								if currentBytes, errRead := os.ReadFile(diskPath); errRead == nil && len(rb.savedDiskBytes) > 0 && !jsonPayloadsEqualOrBytesEqual(currentBytes, rb.savedDiskBytes) {
									superseded = true
									return false
								}
							}
							if errDelete := h.deleteTokenRecord(ctx, savedPath); errDelete != nil {
								log.WithError(errDelete).WithField("path", savedPath).Warn("failed to roll back plugin auth token")
							}
						}
						diskHandled = true
					}
					return current != nil
				}
				_ = h.authManager.RemoveIf(ctx, primaryID, removeMatching)
				if !superseded {
					if altID := strings.TrimSpace(rb.record.ID); altID != "" && altID != primaryID {
						_ = h.authManager.RemoveIf(ctx, altID, removeMatching)
					}
				}
				continue
			}
		}

		diskSuperseded := false
		if rb.existedOnDisk && diskPath != "" {
			currentBytes, errRead := os.ReadFile(diskPath)
			if errRead == nil {
				matchesSaved := len(rb.savedDiskBytes) == 0 || jsonPayloadsEqualOrBytesEqual(currentBytes, rb.savedDiskBytes)
				if !matchesSaved {
					if !jsonPayloadsEqualOrBytesEqual(currentBytes, rb.prevDiskBytes) {
						diskSuperseded = true
					}
				} else {
					h.restoreOverwrittenPluginRecord(ctx, rb, diskPath)
				}
			}
		} else if savedPath != "" {
			if diskPath != "" {
				if currentBytes, errRead := os.ReadFile(diskPath); errRead == nil && len(rb.savedDiskBytes) > 0 && !jsonPayloadsEqualOrBytesEqual(currentBytes, rb.savedDiskBytes) {
					diskSuperseded = true
				}
			}
			if !diskSuperseded {
				if errDelete := h.deleteTokenRecord(ctx, savedPath); errDelete != nil {
					log.WithError(errDelete).WithField("path", savedPath).Warn("failed to roll back plugin auth token")
				}
			}
		}
		if !diskSuperseded && !rb.hadRuntimeAuth && savedPath != "" {
			h.removeAuthsForPath(ctx, savedPath, savedPath)
		}
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
