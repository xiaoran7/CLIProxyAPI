package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

func fetchSingleAccountQuota(ctx context.Context, eng *engine.Engine, cpa *CpaClient, authIndex, provider, projectId, email, filename string) ([]CpaQuotaGroup, string, string, error) {
	prov := strings.ToLower(strings.TrimSpace(provider))
	targetProject := strings.TrimSpace(projectId)
	authIdx := strings.TrimSpace(authIndex)
	fname := strings.TrimSpace(filename)

	// If any critical routing identifier is missing, resolve it from /auth-files
	if authIdx == "" || prov == "" || (prov == "antigravity" && targetProject == "") {
		var authFilesResp CpaAuthFilesResponse
		if err := cpa.RequestJSON(ctx, http.MethodGet, "/auth-files", nil, "v0", &authFilesResp); err == nil {
			for _, f := range authFilesResp.Files {
				match := false
				if authIdx != "" && strings.EqualFold(strings.TrimSpace(f.AuthIndex), authIdx) {
					match = true
				} else if fname != "" && (strings.EqualFold(f.Name, fname) || strings.EqualFold(f.ID, fname)) {
					match = true
				}
				if match {
					if authIdx == "" && strings.TrimSpace(f.AuthIndex) != "" {
						authIdx = strings.TrimSpace(f.AuthIndex)
					}
					if fname == "" && f.Name != "" {
						fname = f.Name
					}
					if prov == "" {
						p := f.Provider
						if p == "" {
							p = f.Type
						}
						prov = strings.ToLower(strings.TrimSpace(p))
					}
					if targetProject == "" && strings.TrimSpace(f.ProjectID) != "" {
						targetProject = strings.TrimSpace(f.ProjectID)
					}
					if email == "" && f.Email != "" {
						email = f.Email
					}
					break
				}
			}
		}
	}

	if prov == "antigravity" && targetProject == "" {
		targetProject = "aicode-consumers"
	}

	var groups []CpaQuotaGroup
	planType := "standard"
	planName := "标准包"

	if prov == "antigravity" {
		if authIdx == "" {
			return nil, "", "", fmt.Errorf("unable to resolve auth_index for %s", fname)
		}
		apiCallBody, _ := json.Marshal(map[string]interface{}{
			"authIndex": authIdx,
			"method":    "POST",
			"url":       "https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary",
			"header": map[string]string{
				"Authorization": "Bearer $TOKEN$",
				"Content-Type":  "application/json",
				"User-Agent":    "antigravity/cli/1.3.6 (aidev_client; os_type=darwin; arch=arm64)",
			},
			"data": fmt.Sprintf(`{"project":%q}`, targetProject),
		})

		resp, err := cpa.Request(ctx, http.MethodPost, "/api-call", apiCallBody, "application/json", "v0")
		if err != nil {
			return nil, "", "", fmt.Errorf("antigravity quota probe failed: %w", err)
		}

		var callWrapper struct {
			StatusCode int                 `json:"status_code"`
			Header     map[string][]string `json:"header"`
			Body       string              `json:"body"`
			Response   json.RawMessage     `json:"response"`
		}
		if err := json.Unmarshal(resp.Bytes, &callWrapper); err != nil {
			return nil, "", "", fmt.Errorf("unmarshal api-call response failed: %w", err)
		}
		if callWrapper.StatusCode != http.StatusOK {
			return nil, "", "", fmt.Errorf("upstream returned HTTP %d: %s", callWrapper.StatusCode, callWrapper.Body)
		}

		var rawUpstream []byte
		if strings.TrimSpace(callWrapper.Body) != "" {
			rawUpstream = []byte(callWrapper.Body)
		} else if len(callWrapper.Response) > 0 {
			rawUpstream = callWrapper.Response
		}

		groups = parseAntigravityQuotaGroups(rawUpstream)
		planType = "standard"
		planName = "Google Cloud"

	} else if prov == "codex" {
		if authIdx == "" {
			return nil, "", "", fmt.Errorf("unable to resolve auth_index for %s", fname)
		}
		apiCallBody, _ := json.Marshal(map[string]interface{}{
			"authIndex": authIdx,
			"method":    "GET",
			"url":       "https://chatgpt.com/backend-api/wham/usage",
			"header": map[string]string{
				"Authorization": "Bearer $TOKEN$",
				"Content-Type":  "application/json",
				"User-Agent":    "codex-tui/0.149.1 (Mac OS 24.1.0; arm64)",
			},
		})

		resp, err := cpa.Request(ctx, http.MethodPost, "/api-call", apiCallBody, "application/json", "v0")
		if err != nil {
			return nil, "", "", fmt.Errorf("codex quota probe failed: %w", err)
		}

		var callWrapper struct {
			StatusCode int                 `json:"status_code"`
			Header     map[string][]string `json:"header"`
			Body       string              `json:"body"`
			Response   json.RawMessage     `json:"response"`
		}
		if err := json.Unmarshal(resp.Bytes, &callWrapper); err != nil {
			return nil, "", "", fmt.Errorf("unmarshal api-call response failed: %w", err)
		}
		if callWrapper.StatusCode != http.StatusOK {
			return nil, "", "", fmt.Errorf("upstream returned HTTP %d: %s", callWrapper.StatusCode, callWrapper.Body)
		}

		var rawUpstream []byte
		if strings.TrimSpace(callWrapper.Body) != "" {
			rawUpstream = []byte(callWrapper.Body)
		} else if len(callWrapper.Response) > 0 {
			rawUpstream = callWrapper.Response
		}

		var pType, pName string
		groups, pType, pName = parseCodexQuotaGroups(rawUpstream)
		if pType != "" {
			planType = pType
		}
		if pName != "" {
			planName = pName
		}

	} else if prov == "claude" {
		if authIdx == "" {
			return nil, "", "", fmt.Errorf("unable to resolve auth_index for %s", fname)
		}
		apiCallBody, _ := json.Marshal(map[string]interface{}{
			"authIndex": authIdx,
			"method":    "GET",
			"url":       "https://api.anthropic.com/api/oauth/usage",
			"header": map[string]string{
				"Authorization":  "Bearer $TOKEN$",
				"Content-Type":   "application/json",
				"anthropic-beta": "oauth-2025-04-20",
				"User-Agent":     "claude-cli/1.0.0",
			},
		})

		resp, err := cpa.Request(ctx, http.MethodPost, "/api-call", apiCallBody, "application/json", "v0")
		if err != nil {
			return nil, "", "", fmt.Errorf("claude quota probe failed: %w", err)
		}

		var callWrapper struct {
			StatusCode int                 `json:"status_code"`
			Header     map[string][]string `json:"header"`
			Body       string              `json:"body"`
			Response   json.RawMessage     `json:"response"`
		}
		if err := json.Unmarshal(resp.Bytes, &callWrapper); err != nil {
			return nil, "", "", fmt.Errorf("unmarshal api-call response failed: %w", err)
		}
		if callWrapper.StatusCode == http.StatusOK {
			var rawUpstream []byte
			if strings.TrimSpace(callWrapper.Body) != "" {
				rawUpstream = []byte(callWrapper.Body)
			} else if len(callWrapper.Response) > 0 {
				rawUpstream = callWrapper.Response
			}
			groups = parseClaudeQuotaGroups(rawUpstream)
			planType = "standard"
			planName = "Anthropic Claude"
		}

	} else {
		// Native WorkBuddy, Qoder, Copilot-Tencent, and plugin declarative quota probes via /quota/fetch
		reqBody, _ := json.Marshal(map[string]string{
			"auth_index": authIdx,
			"authIndex":  authIdx,
			"provider":   prov,
		})
		resp, err := cpa.Request(ctx, http.MethodPost, "/quota/fetch", reqBody, "application/json", "v0")
		if err == nil && resp != nil && resp.StatusCode == http.StatusOK && len(resp.Bytes) > 0 {
			groups, planType, planName = parsePluginQuotaResponse(resp.Bytes, prov)
		}

		// Fallback to /auth-files if /quota/fetch yielded no groups
		if len(groups) == 0 {
			var authFilesResp CpaAuthFilesResponse
			if err := cpa.RequestJSON(ctx, http.MethodGet, "/auth-files", nil, "v0", &authFilesResp); err == nil {
				for _, f := range authFilesResp.Files {
					if (authIdx != "" && strings.EqualFold(strings.TrimSpace(f.AuthIndex), authIdx)) ||
						(fname != "" && (strings.EqualFold(f.Name, fname) || strings.EqualFold(f.ID, fname))) {
						if f.Quota != nil && len(f.Quota.Groups) > 0 {
							groups = f.Quota.Groups
							planType = "standard"
							planName = "标准包"
						}
						break
					}
				}
			}
		}
	}

	if len(groups) > 0 {
		totCreds, remCreds, usedCreds, expAt := extractAccountCredits(groups)
		accountQuotaCacheMu.Lock()
		cached := CachedAccountQuota{
			Groups:           groups,
			PlanType:         planType,
			PlanName:         planName,
			UpdatedAt:        time.Now(),
			TotalCredits:     totCreds,
			RemainingCredits: remCreds,
			UsedCredits:      usedCreds,
			ExpiresAt:        expAt,
		}
		if authIdx != "" {
			accountQuotaCache[authIdx] = cached
		}
		if authIndex != "" {
			accountQuotaCache[authIndex] = cached
		}
		if fname != "" {
			accountQuotaCache[fname] = cached
		}
		if filename != "" {
			accountQuotaCache[filename] = cached
		}
		accountQuotaCacheMu.Unlock()

		if eng != nil && eng.DB() != nil {
			nowUnix := time.Now().Unix()
			for _, g := range groups {
				for _, b := range g.Buckets {
					if b.RemainingPercent != nil {
						winName := fmt.Sprintf("%s:%s", g.Name, b.Window)
						var resetAt *int64
						if b.ResetTime != "" {
							if t, err := time.Parse(time.RFC3339, b.ResetTime); err == nil {
								u := t.Unix()
								resetAt = &u
							}
						}
						_, _ = eng.DB().InsertQuotaSnapshot(ctx, &db.QuotaSnapshotRow{
							TS:               nowUnix,
							AuthIndex:        authIdx,
							Provider:         prov,
							AccountEmail:     &email,
							WindowName:       &winName,
							RemainingPercent: float64(*b.RemainingPercent),
							ResetAt:          resetAt,
						})
					}
				}
			}
		}
	}

	return groups, planType, planName, nil
}
