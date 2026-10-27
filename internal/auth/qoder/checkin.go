// Package qoder provides authentication, COSY request signing, and token management
// for Qoder Global and Qoder Mainland China APIs.
package qoder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// Checkin outcome statuses matching standard conventions.
const (
	CheckinClaimed    = "claimed"    // Claimed successfully today
	CheckinDone       = "done"       // Already checked in today
	CheckinIneligible = "ineligible" // Account ineligible for check-in
	CheckinInactive   = "inactive"   // Campaign is inactive or ended
	CheckinFailed     = "failed"     // Network or temporary failure
)

const (
	// CampaignsPath lists user campaigns and check-in promotions.
	CampaignsPath = "/sash/api/v1/me/campaigns"
	// CampaignClaimPath claims benefits from a campaign.
	CampaignClaimPath = "/sash/api/v1/me/campaigns/%s/claim"

	// DailyCheckinClaimPath is the direct daily check-in claim endpoint for Qoder / QoderWork.
	DailyCheckinClaimPath = "/sash/api/v1/me/daily-check-in/claim"
	// DailyCheckinAltPath is the alternative direct daily check-in endpoint.
	DailyCheckinAltPath = "/sash/api/v1/me/daily-checkin"
	// DailyCheckinStatusPath queries direct daily check-in activity status.
	DailyCheckinStatusPath = "/sash/api/v1/me/daily-check-in/status"

	// CosyClientTypeDesktop represents the official desktop client environment with check-in enabled.
	CosyClientTypeDesktop = "10"
)

var beijingZone = time.FixedZone("CST", 8*60*60)

// TodayBeijing returns the current date string in Beijing time (YYYY-MM-DD).
func TodayBeijing() string {
	return time.Now().In(beijingZone).Format("2006-01-02")
}

// CheckinResult holds the outcome of a check-in attempt.
type CheckinResult struct {
	AuthID  string    `json:"auth_id,omitempty"`
	User    string    `json:"user,omitempty"`
	Realm   string    `json:"realm"`
	Date    string    `json:"date"`
	Day     string    `json:"day,omitempty"`
	Status  string    `json:"status"`
	Outcome string    `json:"outcome,omitempty"`
	Points  int64     `json:"points,omitempty"`
	Credit  float64   `json:"credit,omitempty"`
	Message string    `json:"message,omitempty"`
	Checked time.Time `json:"checked_at"`
	At      time.Time `json:"at,omitempty"`
}

func syncCheckinResult(result *CheckinResult) {
	if result == nil {
		return
	}
	if result.Outcome == "" {
		result.Outcome = result.Status
	}
	if result.Status == "" {
		result.Status = result.Outcome
	}
	if result.Day == "" {
		result.Day = result.Date
	}
	if result.Date == "" {
		result.Date = result.Day
	}
	if result.Credit == 0 && result.Points > 0 {
		result.Credit = float64(result.Points)
	}
	if result.Points == 0 && result.Credit > 0 {
		result.Points = int64(result.Credit)
	}
	if result.At.IsZero() {
		result.At = result.Checked
	}
	if result.Checked.IsZero() {
		result.Checked = result.At
	}
}

// setQoderClientHeaders injects standard desktop client and COSY protocol headers required for campaigns.
func setQoderClientHeaders(req *http.Request, token string, machineID string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Cosy-Clienttype", CosyClientTypeDesktop)
	req.Header.Set("Cosy-ClientType", CosyClientTypeDesktop)
	req.Header.Set("Cosy-Machinetype", CosyMachineType)
	req.Header.Set("Cosy-Version", CosyVersion)
	if machineID != "" {
		req.Header.Set("Cosy-Machinetoken", machineID)
		req.Header.Set("Cosy-Machineid", machineID)
		req.Header.Set("Cosy-Machinecode", machineID)
	}
}

// Checkin performs the daily check-in flow for a single Qoder account.
func Checkin(ctx context.Context, httpClient *http.Client, auth *cliproxyauth.Auth) (*CheckinResult, error) {
	if auth == nil {
		return nil, fmt.Errorf("qoder: auth is nil")
	}

	realm := ResolveRealm(auth)
	uid := ResolveUserID(auth)
	today := TodayBeijing()
	result := &CheckinResult{
		AuthID:  auth.ID,
		User:    uid,
		Realm:   realm,
		Date:    today,
		Day:     today,
		Checked: time.Now(),
		At:      time.Now(),
	}
	defer func() {
		syncCheckinResult(result)
	}()

	token := AccessToken(auth)
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		result.Status = CheckinFailed
		result.Outcome = CheckinFailed
		result.Message = "no usable access token"
		return result, fmt.Errorf("qoder: no access token")
	}

	openAPIBase := ResolveOpenAPIBaseURLForAuth(realm, auth)
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	machineID := ResolveMachineID(auth)

	// Step 1: Query campaigns list via OpenAPI
	listURL := openAPIBase + CampaignsPath
	reqList, errList := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if errList != nil {
		result.Status = CheckinFailed
		result.Outcome = CheckinFailed
		result.Message = errList.Error()
		return result, errList
	}
	setQoderClientHeaders(reqList, token, machineID)

	respList, errDoList := client.Do(reqList)
	if errDoList == nil && respList != nil {
		defer func() {
			_ = respList.Body.Close()
		}()

		if respList.StatusCode == http.StatusOK {
			bodyList, errRead := io.ReadAll(respList.Body)
			if errRead == nil {
				parsedList := gjson.ParseBytes(bodyList)
				code := parsedList.Get("code").Int()
				if code == 0 || code == 200 {
					campaigns := parsedList.Get("data.campaigns").Array()
					if len(campaigns) == 0 {
						campaigns = parsedList.Get("data").Array()
					}

					var targetCampaignID string
					var alreadyClaimed bool
					for _, c := range campaigns {
						status := strings.ToLower(strings.TrimSpace(c.Get("status").String()))
						claimed := c.Get("claimed").Bool()
						if claimed || status == "claimed" || status == "done" || status == "finished" {
							alreadyClaimed = true
							continue
						}
						id := strings.TrimSpace(c.Get("id").String())
						if id == "" {
							id = strings.TrimSpace(c.Get("campaign_id").String())
						}
						if id != "" {
							targetCampaignID = id
							break
						}
					}

					if targetCampaignID != "" {
						// Claim campaign reward
						claimURL := fmt.Sprintf(openAPIBase+CampaignClaimPath, targetCampaignID)
						reqClaim, errClaim := http.NewRequestWithContext(ctx, http.MethodPost, claimURL, bytes.NewReader([]byte("{}")))
						if errClaim == nil {
							setQoderClientHeaders(reqClaim, token, machineID)
							reqClaim.Header.Set("Content-Type", "application/json")

							respClaim, errDoClaim := client.Do(reqClaim)
							if errDoClaim == nil && respClaim != nil {
								defer func() {
									_ = respClaim.Body.Close()
								}()
								bodyClaim, _ := io.ReadAll(respClaim.Body)
								parsedClaim := gjson.ParseBytes(bodyClaim)
								claimCode := parsedClaim.Get("code").Int()

								if claimCode == 0 || claimCode == 200 {
									pts := parsedClaim.Get("data.points").Int()
									if pts == 0 {
										pts = parsedClaim.Get("data.credits").Int()
									}
									if pts == 0 {
										pts = 100 // default Qoder daily credit reward
									}
									result.Status = CheckinClaimed
									result.Outcome = CheckinClaimed
									result.Points = pts
									result.Credit = float64(pts)
									result.Message = "successfully claimed"
									return result, nil
								}

								msg := parsedClaim.Get("msg").String()
								if msg == "" {
									msg = parsedClaim.Get("message").String()
								}
								if strings.Contains(strings.ToLower(msg), "already") || claimCode == 1001 || claimCode == 10001 {
									result.Status = CheckinDone
									result.Outcome = CheckinDone
									result.Message = "already claimed today"
									return result, nil
								}
								if strings.Contains(strings.ToLower(msg), "ineligible") || claimCode == 1002 {
									result.Status = CheckinIneligible
									result.Outcome = CheckinIneligible
									result.Message = msg
									return result, nil
								}
							}
						}
					} else if alreadyClaimed {
						result.Status = CheckinDone
						result.Outcome = CheckinDone
						result.Message = "already claimed today"
						return result, nil
					}
				}
			}
		} else if respList.StatusCode == http.StatusUnauthorized {
			result.Status = CheckinFailed
			result.Outcome = CheckinFailed
			result.Message = "unauthorized: invalid or expired access token"
			return result, fmt.Errorf("qoder: unauthorized token")
		}
	}

	// Step 2: Fallback to direct daily check-in claim endpoints
	directEndpoints := []string{
		openAPIBase + DailyCheckinClaimPath,
		openAPIBase + DailyCheckinAltPath,
	}

	for _, ep := range directEndpoints {
		reqDirect, errDirect := http.NewRequestWithContext(ctx, http.MethodPost, ep, bytes.NewReader([]byte("{}")))
		if errDirect != nil {
			continue
		}
		setQoderClientHeaders(reqDirect, token, machineID)
		reqDirect.Header.Set("Content-Type", "application/json")

		respDirect, errDoDirect := client.Do(reqDirect)
		if errDoDirect != nil || respDirect == nil {
			continue
		}
		bodyDirect, _ := io.ReadAll(respDirect.Body)
		_ = respDirect.Body.Close()

		if respDirect.StatusCode == http.StatusOK {
			parsedDirect := gjson.ParseBytes(bodyDirect)
			code := parsedDirect.Get("code").Int()
			if code == 0 || code == 200 {
				pts := parsedDirect.Get("data.points").Int()
				if pts == 0 {
					pts = parsedDirect.Get("data.credits").Int()
				}
				if pts == 0 {
					pts = 100
				}
				result.Status = CheckinClaimed
				result.Outcome = CheckinClaimed
				result.Points = pts
				result.Credit = float64(pts)
				result.Message = "successfully claimed"
				return result, nil
			}

			msg := parsedDirect.Get("msg").String()
			if msg == "" {
				msg = parsedDirect.Get("message").String()
			}
			msgLower := strings.ToLower(msg)
			if strings.Contains(msgLower, "already") || strings.Contains(msg, "已签到") || strings.Contains(msg, "已领取") || code == 1001 || code == 10001 {
				result.Status = CheckinDone
				result.Outcome = CheckinDone
				result.Message = "already claimed today"
				return result, nil
			}
			if strings.Contains(msgLower, "ineligible") || strings.Contains(msgLower, "not eligible") || code == 1002 {
				result.Status = CheckinIneligible
				result.Outcome = CheckinIneligible
				result.Message = msg
				return result, nil
			}
			if strings.Contains(msgLower, "ended") || strings.Contains(msgLower, "inactive") || code == 1003 {
				result.Status = CheckinInactive
				result.Outcome = CheckinInactive
				result.Message = msg
				return result, nil
			}
		} else if respDirect.StatusCode == http.StatusUnauthorized {
			result.Status = CheckinFailed
			result.Outcome = CheckinFailed
			result.Message = "unauthorized: invalid or expired access token"
			return result, fmt.Errorf("qoder: unauthorized token")
		}
	}

	// If no campaigns and direct endpoint was not available, treat as inactive or completed
	result.Status = CheckinDone
	result.Outcome = CheckinDone
	result.Message = "no active check-in campaigns available or already checked in"
	return result, nil
}

// DoCheckin is an alias for Checkin for backward compatibility.
func DoCheckin(ctx context.Context, httpClient *http.Client, auth *cliproxyauth.Auth) (*CheckinResult, error) {
	return Checkin(ctx, httpClient, auth)
}

// isInternalCheckinKey checks if the key belongs to an internal persistence file or envelope key.
func isInternalCheckinKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	base := filepath.Base(lower)
	if lower == "enabled" || lower == "records" || base == "enabled" || base == "records" {
		return true
	}
	return strings.HasSuffix(base, "checkins.json") || base == "session_affinity.json" || strings.HasSuffix(lower, "checkins.json") || lower == "session_affinity.json"
}

// CheckinEngine manages scheduled background daily check-ins for all Qoder accounts.
type CheckinEngine struct {
	mu         sync.RWMutex
	storePath  string
	records    map[string]CheckinResult // key: authID
	authGetter func() []*cliproxyauth.Auth
	client     *http.Client
	stopCh     chan struct{}
	running    bool
	enabled    bool
}

// NewCheckinEngine initializes a new Qoder checkin engine with persistence.
func NewCheckinEngine(storeDir string, authGetter func() []*cliproxyauth.Auth) *CheckinEngine {
	if storeDir == "" {
		storeDir = "."
	}
	storePath := filepath.Join(storeDir, "qoder_checkins.json")
	engine := &CheckinEngine{
		storePath:  storePath,
		records:    make(map[string]CheckinResult),
		authGetter: authGetter,
		client:     &http.Client{Timeout: 30 * time.Second},
		stopCh:     make(chan struct{}),
		enabled:    true,
	}
	engine.loadRecords()
	return engine
}

// IsEnabled returns whether the automated background check-in scheduler is enabled.
func (e *CheckinEngine) IsEnabled() bool {
	if e == nil {
		return false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.enabled
}

// SetEnabled toggles the automated check-in engine on or off and persists state.
func (e *CheckinEngine) SetEnabled(enabled bool) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enabled = enabled
	e.saveRecordsLocked()
}

// Start launches the daily background scheduler.
func (e *CheckinEngine) Start() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.stopCh = make(chan struct{})
	e.mu.Unlock()

	go e.runLoop()
}

// Stop cleanly terminates the background loop.
func (e *CheckinEngine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return
	}
	e.running = false
	close(e.stopCh)
}

func (e *CheckinEngine) runLoop() {
	e.mu.RLock()
	stopCh := e.stopCh
	e.mu.RUnlock()

	// Run initial check-in cycle 5 seconds after startup if enabled
	select {
	case <-time.After(5 * time.Second):
		if e.IsEnabled() {
			e.RunCheckinAll(context.Background())
		}
	case <-stopCh:
		return
	}

	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if e.IsEnabled() {
				e.RunCheckinAll(context.Background())
			}
		case <-stopCh:
			return
		}
	}
}

// RunCheckinAll executes check-in for all eligible Qoder accounts.
func (e *CheckinEngine) RunCheckinAll(ctx context.Context) map[string]CheckinResult {
	if e.authGetter == nil {
		return nil
	}

	auths := e.authGetter()
	results := make(map[string]CheckinResult)
	today := TodayBeijing()

	for _, a := range auths {
		if a == nil || a.Disabled {
			continue
		}
		if isInternalCheckinKey(a.ID) || isInternalCheckinKey(a.FileName) {
			continue
		}
		provider := strings.ToLower(strings.TrimSpace(a.Provider))
		authKind := strings.ToLower(strings.TrimSpace(a.Attributes["auth_kind"]))
		isQoder := provider == "qoder" || provider == "qoder-cn" || provider == "qoder-intl" || provider == "qoder-en" || authKind == "qoder"
		if !isQoder {
			continue
		}

		// Check if already checked in today
		e.mu.RLock()
		existing, ok := e.records[a.ID]
		e.mu.RUnlock()

		if ok && (existing.Day == today || existing.Date == today) &&
			(existing.Outcome == CheckinClaimed || existing.Outcome == CheckinDone ||
				existing.Outcome == CheckinIneligible || existing.Outcome == CheckinInactive) {
			results[a.ID] = existing
			continue
		}

		// Perform check-in
		res, err := Checkin(ctx, e.client, a)
		if err != nil {
			log.WithError(err).Warnf("qoder checkin failed for account %s (%s)", a.ID, ResolveUserID(a))
		}
		if res != nil {
			results[a.ID] = *res
			e.mu.Lock()
			e.records[a.ID] = *res
			e.saveRecordsLocked()
			e.mu.Unlock()
		}
	}

	return results
}

// GetStatus returns the today check-in status for an authID.
func (e *CheckinEngine) GetStatus(authID string) (CheckinResult, bool) {
	if isInternalCheckinKey(authID) {
		return CheckinResult{}, false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	res, ok := e.records[authID]
	return res, ok
}

// GetAllRecords returns a copy of all check-in records excluding internal keys.
func (e *CheckinEngine) GetAllRecords() map[string]CheckinResult {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make(map[string]CheckinResult, len(e.records))
	for k, v := range e.records {
		if isInternalCheckinKey(k) {
			continue
		}
		out[k] = v
	}
	return out
}

// RecordCheckin stores a check-in result into the engine memory and disk store.
func (e *CheckinEngine) RecordCheckin(authID string, res CheckinResult) {
	if e == nil || isInternalCheckinKey(authID) {
		return
	}
	syncCheckinResult(&res)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.records[authID] = res
	e.saveRecordsLocked()
}

type qoderCheckinStorageEnvelope struct {
	Enabled *bool                    `json:"enabled,omitempty"`
	Records map[string]CheckinResult `json:"records,omitempty"`
}

func (e *CheckinEngine) loadRecords() {
	e.mu.Lock()
	defer e.mu.Unlock()

	data, err := os.ReadFile(e.storePath)
	if err != nil {
		return
	}

	var envelope qoderCheckinStorageEnvelope
	if err := json.Unmarshal(data, &envelope); err == nil && (envelope.Records != nil || envelope.Enabled != nil) {
		if envelope.Enabled != nil {
			e.enabled = *envelope.Enabled
		}
		e.records = make(map[string]CheckinResult)
		for k, v := range envelope.Records {
			if !isInternalCheckinKey(k) {
				syncCheckinResult(&v)
				e.records[k] = v
			}
		}
		return
	}

	// Fallback for legacy format: map[string]CheckinResult
	var legacy map[string]CheckinResult
	if err := json.Unmarshal(data, &legacy); err == nil {
		e.records = make(map[string]CheckinResult)
		for k, v := range legacy {
			if !isInternalCheckinKey(k) {
				syncCheckinResult(&v)
				e.records[k] = v
			}
		}
	}
}

func (e *CheckinEngine) saveRecordsLocked() {
	if e.storePath == "" {
		return
	}
	cleanRecords := make(map[string]CheckinResult, len(e.records))
	for k, v := range e.records {
		if !isInternalCheckinKey(k) {
			cleanRecords[k] = v
		}
	}

	envelope := qoderCheckinStorageEnvelope{
		Enabled: &e.enabled,
		Records: cleanRecords,
	}

	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return
	}

	dir := filepath.Dir(e.storePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", e.storePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tmpFile, e.storePath)
}

var (
	globalEngineMu sync.RWMutex
	globalEngine   *CheckinEngine
)

// InitGlobalCheckinEngine initializes and starts the global Qoder check-in engine.
func InitGlobalCheckinEngine(storeDir string, authGetter func() []*cliproxyauth.Auth) *CheckinEngine {
	globalEngineMu.Lock()
	defer globalEngineMu.Unlock()

	if globalEngine != nil {
		globalEngine.Stop()
	}
	globalEngine = NewCheckinEngine(storeDir, authGetter)
	globalEngine.Start()
	return globalEngine
}

// GetGlobalCheckinEngine retrieves the running global check-in engine instance.
func GetGlobalCheckinEngine() *CheckinEngine {
	globalEngineMu.RLock()
	defer globalEngineMu.RUnlock()
	return globalEngine
}
