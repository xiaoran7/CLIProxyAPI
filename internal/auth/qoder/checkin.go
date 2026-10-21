// Package qoder provides authentication, COSY request signing, and token management
// for Qoder Global and Qoder Mainland China APIs.
package qoder

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/tidwall/gjson"
)

// Check-in outcome statuses matching standard conventions.
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
	result.Outcome = result.Status
	result.Day = result.Date
	result.Credit = float64(result.Points)
	result.At = result.Checked
}

// DoCheckin attempts daily check-in and benefit claim for a Qoder credential.
func DoCheckin(ctx context.Context, httpClient *http.Client, auth *cliproxyauth.Auth) (*CheckinResult, error) {
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
		Checked: time.Now(),
	}
	defer func() {
		syncCheckinResult(result)
	}()

	token := AccessToken(auth)
	if token == "" {
		result.Status = CheckinFailed
		result.Message = "no usable access token"
		return result, fmt.Errorf("qoder: no access token")
	}

	openAPIBase := ResolveOpenAPIBaseURL(realm)
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	// 1. Fetch campaigns
	listURL := openAPIBase + CampaignsPath
	reqList, errList := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if errList != nil {
		result.Status = CheckinFailed
		result.Message = errList.Error()
		return result, errList
	}
	reqList.Header.Set("Authorization", "Bearer "+token)
	reqList.Header.Set("Accept", "application/json")
	reqList.Header.Set("User-Agent", DefaultUserAgent)
	machineID := ResolveMachineID(auth)
	reqList.Header.Set("Cosy-Machinetoken", machineID)
	reqList.Header.Set("Cosy-Machineid", machineID)
	reqList.Header.Set("Cosy-Machinetype", CosyMachineType)

	respList, errDoList := client.Do(reqList)
	if errDoList != nil {
		result.Status = CheckinFailed
		result.Message = errDoList.Error()
		return result, errDoList
	}
	defer func() {
		_ = respList.Body.Close()
	}()

	bodyList, errRead := io.ReadAll(respList.Body)
	if errRead != nil {
		result.Status = CheckinFailed
		result.Message = errRead.Error()
		return result, errRead
	}

	parsedList := gjson.ParseBytes(bodyList)
	campaigns := parsedList.Get("data.campaigns").Array()
	if len(campaigns) == 0 {
		campaigns = parsedList.Get("data").Array()
	}
	if len(campaigns) == 0 {
		result.Status = CheckinDone
		result.Message = "no active check-in campaigns available or already checked in"
		return result, nil
	}

	// Find eligible checkin campaign
	var targetCampaignID string
	var alreadyClaimed bool
	for _, c := range campaigns {
		status := c.Get("status").String()
		claimed := c.Get("claimed").Bool()
		if claimed || status == "claimed" || status == "done" {
			alreadyClaimed = true
			continue
		}
		id := c.Get("id").String()
		if id == "" {
			id = c.Get("campaign_id").String()
		}
		if id != "" {
			targetCampaignID = id
			break
		}
	}

	if targetCampaignID == "" {
		if alreadyClaimed {
			result.Status = CheckinDone
			result.Message = "already claimed today"
			return result, nil
		}
		result.Status = CheckinInactive
		result.Message = "no eligible daily campaign"
		return result, nil
	}

	// 2. Claim benefit
	claimURL := fmt.Sprintf(openAPIBase+CampaignClaimPath, targetCampaignID)
	reqClaim, errClaim := http.NewRequestWithContext(ctx, http.MethodPost, claimURL, bytes.NewReader([]byte("{}")))
	if errClaim != nil {
		result.Status = CheckinFailed
		result.Message = errClaim.Error()
		return result, errClaim
	}
	reqClaim.Header.Set("Authorization", "Bearer "+token)
	reqClaim.Header.Set("Content-Type", "application/json")
	reqClaim.Header.Set("Accept", "application/json")
	reqClaim.Header.Set("User-Agent", DefaultUserAgent)
	reqClaim.Header.Set("Cosy-Machinetoken", machineID)
	reqClaim.Header.Set("Cosy-Machineid", machineID)
	reqClaim.Header.Set("Cosy-Machinetype", CosyMachineType)

	respClaim, errDoClaim := client.Do(reqClaim)
	if errDoClaim != nil {
		result.Status = CheckinFailed
		result.Message = errDoClaim.Error()
		return result, errDoClaim
	}
	defer func() {
		_ = respClaim.Body.Close()
	}()

	bodyClaim, _ := io.ReadAll(respClaim.Body)
	parsedClaim := gjson.ParseBytes(bodyClaim)
	code := parsedClaim.Get("code").Int()

	if code == 0 || code == 200 {
		result.Status = CheckinClaimed
		result.Points = parsedClaim.Get("data.points").Int()
		result.Message = "successfully claimed"
		return result, nil
	}

	msg := parsedClaim.Get("msg").String()
	if strings.Contains(msg, "already") || code == 1001 {
		result.Status = CheckinDone
		result.Message = "already claimed today"
		return result, nil
	}
	if strings.Contains(msg, "ineligible") || code == 1002 {
		result.Status = CheckinIneligible
		result.Message = msg
		return result, nil
	}

	result.Status = CheckinFailed
	result.Message = fmt.Sprintf("claim failed with code %d: %s", code, msg)
	return result, nil
}
