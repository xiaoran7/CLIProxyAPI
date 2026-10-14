// Package workbuddy provides authentication and token management for WorkBuddy and CodeBuddy API.
package workbuddy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	CheckinOwnApp     = "own-app"    // Checkin requires native client app
	CheckinFailed     = "failed"     // Network or temporary failure
)

// WorkBuddy check-in specific API error codes.
const (
	wbCheckinAlready    = 1001 // Already checked in today
	wbCheckinIneligible = 1002 // Account ineligible
	wbCheckinEnded      = 1003 // Campaign ended
)

// Beijing fixed timezone (CST / UTC+8) for daily check-in boundaries.
var beijingZone = time.FixedZone("CST", 8*60*60)

// TodayBeijing returns the current date string in Beijing time (YYYY-MM-DD).
func TodayBeijing() string {
	return time.Now().In(beijingZone).Format("2006-01-02")
}

// CheckinResult holds the outcome of a check-in attempt.
type CheckinResult struct {
	AuthID  string    `json:"auth_id,omitempty"`
	User    string    `json:"user,omitempty"`
	Day     string    `json:"day"`
	Outcome string    `json:"outcome"`
	Credit  float64   `json:"credit,omitempty"`
	Streak  int       `json:"streak,omitempty"`
	Message string    `json:"message,omitempty"`
	Realm   string    `json:"realm,omitempty"`
	At      time.Time `json:"at"`
}

// Checkin performs the daily check-in flow for a single WorkBuddy account.
func Checkin(ctx context.Context, client *http.Client, auth *cliproxyauth.Auth) (*CheckinResult, error) {
	if auth == nil {
		return nil, errors.New("workbuddy checkin: auth is nil")
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	today := TodayBeijing()
	result := &CheckinResult{
		AuthID: auth.ID,
		User:   extractNickname(auth),
		Day:    today,
		At:     time.Now().UTC(),
	}

	// Only mainland China (cn realm) has the daily check-in campaign.
	realm := ResolveRealm(auth)
	result.Realm = realm
	if !strings.EqualFold(realm, RealmCN) {
		result.Outcome = CheckinInactive
		result.Message = "WorkBuddy (intl) does not have check-in activity"
		return result, nil
	}

	token := extractAccessToken(auth)
	if token == "" {
		result.Outcome = CheckinFailed
		result.Message = "missing access token"
		return result, errors.New("workbuddy checkin: missing access token")
	}

	baseURL := ResolveBaseURL(realm)
	if auth.Attributes != nil {
		if bu := strings.TrimSpace(auth.Attributes["base_url"]); bu != "" {
			baseURL = strings.TrimRight(bu, "/")
		}
	}

	// Step 1: Query activity status
	statusURL := baseURL + "/v2/billing/meter/checkin-activity-status"
	reqStatus, errReq := http.NewRequestWithContext(ctx, http.MethodPost, statusURL, bytes.NewReader([]byte("{}")))
	if errReq != nil {
		result.Outcome = CheckinFailed
		result.Message = errReq.Error()
		return result, errReq
	}
	ApplyHeaders(reqStatus, auth, token)

	respStatus, errDo := client.Do(reqStatus)
	if errDo != nil {
		result.Outcome = CheckinFailed
		result.Message = errDo.Error()
		return result, errDo
	}
	defer func() { _ = respStatus.Body.Close() }()

	bodyStatus, errRead := io.ReadAll(respStatus.Body)
	if errRead != nil {
		result.Outcome = CheckinFailed
		result.Message = errRead.Error()
		return result, errRead
	}

	if respStatus.StatusCode == http.StatusUnauthorized {
		return handleUnauthorizedCheckin(ctx, client, baseURL, auth, token, result)
	}

	if respStatus.StatusCode != http.StatusOK {
		result.Outcome = CheckinFailed
		result.Message = fmt.Sprintf("HTTP %d: %s", respStatus.StatusCode, string(bodyStatus))
		return result, fmt.Errorf("workbuddy checkin status error: %s", result.Message)
	}

	// Parse activity status response
	code := gjson.GetBytes(bodyStatus, "code").Int()
	if code != 0 {
		msg := gjson.GetBytes(bodyStatus, "msg").String()
		switch code {
		case wbCheckinAlready:
			result.Outcome = CheckinDone
			result.Message = msg
			return result, nil
		case wbCheckinIneligible:
			result.Outcome = CheckinIneligible
			result.Message = msg
			return result, nil
		case wbCheckinEnded:
			result.Outcome = CheckinInactive
			result.Message = msg
			return result, nil
		default:
			result.Outcome = CheckinFailed
			result.Message = msg
			return result, fmt.Errorf("workbuddy checkin status code %d: %s", code, msg)
		}
	}

	dataStatus := gjson.GetBytes(bodyStatus, "data")
	isActive := dataStatus.Get("active").Bool()
	if !isActive {
		result.Outcome = CheckinInactive
		result.Message = "checkin campaign inactive"
		return result, nil
	}

	todayCheckedIn := dataStatus.Get("today_checked_in").Bool()
	streakDays := int(dataStatus.Get("streak_days").Int())
	todayCredit := dataStatus.Get("today_credit").Float()
	if todayCredit == 0 {
		todayCredit = dataStatus.Get("daily_credit").Float()
	}

	if todayCheckedIn {
		result.Outcome = CheckinDone
		result.Credit = todayCredit
		result.Streak = streakDays
		result.Message = "already checked in today"
		return result, nil
	}

	// Step 2: Perform daily check-in claim
	claimURL := baseURL + "/v2/billing/meter/daily-checkin"
	reqClaim, errClaimReq := http.NewRequestWithContext(ctx, http.MethodPost, claimURL, bytes.NewReader([]byte("{}")))
	if errClaimReq != nil {
		result.Outcome = CheckinFailed
		result.Message = errClaimReq.Error()
		return result, errClaimReq
	}
	ApplyHeaders(reqClaim, auth, token)

	respClaim, errClaimDo := client.Do(reqClaim)
	if errClaimDo != nil {
		result.Outcome = CheckinFailed
		result.Message = errClaimDo.Error()
		return result, errClaimDo
	}
	defer func() { _ = respClaim.Body.Close() }()

	bodyClaim, _ := io.ReadAll(respClaim.Body)
	if respClaim.StatusCode == http.StatusUnauthorized {
		return handleUnauthorizedCheckin(ctx, client, baseURL, auth, token, result)
	}

	claimCode := gjson.GetBytes(bodyClaim, "code").Int()
	claimMsg := gjson.GetBytes(bodyClaim, "msg").String()
	if claimCode != 0 {
		switch claimCode {
		case wbCheckinAlready:
			result.Outcome = CheckinDone
			result.Streak = streakDays
			result.Message = claimMsg
			return result, nil
		case wbCheckinIneligible:
			result.Outcome = CheckinIneligible
			result.Message = claimMsg
			return result, nil
		case wbCheckinEnded:
			result.Outcome = CheckinInactive
			result.Message = claimMsg
			return result, nil
		default:
			result.Outcome = CheckinFailed
			result.Message = claimMsg
			return result, fmt.Errorf("workbuddy checkin claim error code %d: %s", claimCode, claimMsg)
		}
	}

	claimData := gjson.GetBytes(bodyClaim, "data")
	result.Outcome = CheckinClaimed
	result.Credit = claimData.Get("credit").Float()
	result.Streak = int(claimData.Get("streak_days").Int())
	result.Message = claimData.Get("message").String()
	if result.Message == "" {
		result.Message = "checkin claimed successfully"
	}

	return result, nil
}

// handleUnauthorizedCheckin handles 401 response from checkin endpoint gracefully.
func handleUnauthorizedCheckin(ctx context.Context, client *http.Client, baseURL string, auth *cliproxyauth.Auth, token string, result *CheckinResult) (*CheckinResult, error) {
	// Probe user quota summary to see if the credential itself is healthy
	summaryURL := baseURL + UserPointsPath
	reqSummary, errReq := http.NewRequestWithContext(ctx, http.MethodGet, summaryURL, nil)
	if errReq == nil {
		ApplyHeaders(reqSummary, auth, token)
		if respSummary, errDo := client.Do(reqSummary); errDo == nil {
			_ = respSummary.Body.Close()
			if respSummary.StatusCode == http.StatusOK {
				// Token is valid for normal API use, but check-in requires native app device token
				result.Outcome = CheckinOwnApp
				result.Message = "WorkBuddy check-in requires official desktop app"
				return result, nil
			}
		}
	}

	result.Outcome = CheckinFailed
	result.Message = "authentication failed (401 Unauthorized)"
	return result, errors.New("workbuddy checkin: 401 unauthorized")
}

// CheckinEngine manages scheduled background daily check-ins for all WorkBuddy accounts.
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

// NewCheckinEngine initializes a new checkin engine with persistence.
func NewCheckinEngine(storeDir string, authGetter func() []*cliproxyauth.Auth) *CheckinEngine {
	if storeDir == "" {
		storeDir = "."
	}
	storePath := filepath.Join(storeDir, "workbuddy_checkins.json")
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

// RunCheckinAll executes check-in for all eligible WorkBuddy accounts.
func (e *CheckinEngine) RunCheckinAll(ctx context.Context) map[string]CheckinResult {
	if e.authGetter == nil {
		return nil
	}

	auths := e.authGetter()
	results := make(map[string]CheckinResult)
	today := TodayBeijing()

	for _, a := range auths {
		if a == nil {
			continue
		}
		provider := strings.ToLower(strings.TrimSpace(a.Provider))
		authKind := strings.ToLower(strings.TrimSpace(a.Attributes["auth_kind"]))
		isWorkbuddy := provider == "workbuddy" || provider == "workbuddy-cn" || authKind == "workbuddy"
		if !isWorkbuddy {
			continue
		}

		// Only cn realm participates
		if !strings.EqualFold(ResolveRealm(a), RealmCN) {
			continue
		}

		// Check if already checked in today
		e.mu.RLock()
		existing, ok := e.records[a.ID]
		e.mu.RUnlock()

		if ok && existing.Day == today && (existing.Outcome == CheckinClaimed || existing.Outcome == CheckinDone || existing.Outcome == CheckinOwnApp || existing.Outcome == CheckinIneligible || existing.Outcome == CheckinInactive) {
			results[a.ID] = existing
			continue
		}

		// Perform check-in
		res, err := Checkin(ctx, e.client, a)
		if err != nil {
			log.WithError(err).Warnf("workbuddy checkin failed for account %s (%s)", a.ID, extractNickname(a))
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

// isInternalCheckinKey checks if the key belongs to an internal persistence file rather than a user account.
func isInternalCheckinKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	base := filepath.Base(lower)
	return strings.HasSuffix(base, "checkins.json") || base == "session_affinity.json" || strings.HasSuffix(lower, "checkins.json") || lower == "session_affinity.json"
}

// GetStatus returns the today check-in status for an authID or all records.
func (e *CheckinEngine) GetStatus(authID string) (CheckinResult, bool) {
	if isInternalCheckinKey(authID) {
		return CheckinResult{}, false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	res, ok := e.records[authID]
	return res, ok
}

// GetAllRecords returns all recorded check-ins.
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
	e.mu.Lock()
	defer e.mu.Unlock()
	e.records[authID] = res
	e.saveRecordsLocked()
}

type checkinStorePayload struct {
	Enabled *bool                    `json:"enabled,omitempty"`
	Records map[string]CheckinResult `json:"records"`
}

func (e *CheckinEngine) loadRecords() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enabled = true
	data, err := os.ReadFile(e.storePath)
	if err != nil {
		return
	}
	var payload checkinStorePayload
	if errUnmarshal := json.Unmarshal(data, &payload); errUnmarshal == nil && (payload.Enabled != nil || payload.Records != nil) {
		if payload.Records != nil {
			e.records = payload.Records
		}
		if payload.Enabled != nil {
			e.enabled = *payload.Enabled
		}
	} else {
		// Fallback for legacy format: map[string]CheckinResult
		_ = json.Unmarshal(data, &e.records)
	}
	if e.records == nil {
		e.records = make(map[string]CheckinResult)
	}
	var dirty bool
	for k := range e.records {
		if isInternalCheckinKey(k) {
			delete(e.records, k)
			dirty = true
		}
	}
	if dirty {
		e.saveRecordsLocked()
	}
}

func (e *CheckinEngine) saveRecordsLocked() {
	if e.storePath == "" {
		return
	}
	dir := filepath.Dir(e.storePath)
	_ = os.MkdirAll(dir, 0o755)

	payload := checkinStorePayload{
		Enabled: &e.enabled,
		Records: e.records,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return
	}
	tmpFile := e.storePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmpFile, e.storePath); err != nil {
		_ = os.Remove(tmpFile)
		_ = os.WriteFile(e.storePath, data, 0o600)
	}
}

var (
	globalEngineMu sync.RWMutex
	globalEngine   *CheckinEngine
)

// InitGlobalCheckinEngine initializes and starts the global WorkBuddy check-in engine.
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
