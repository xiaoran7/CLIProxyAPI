package zcode

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	// PollInterval is how long to wait between sign-in polls.
	PollInterval = 2 * time.Second
	// LoginTimeout bounds an interactive sign-in.
	LoginTimeout = 5 * time.Minute
	// DefaultPollInterval is used when ZCode does not name one.
	DefaultPollInterval = time.Second
)

// AuthSession is an initiated ZCode CLI sign-in.
type AuthSession struct {
	// AuthURL is the page the user opens to sign in.
	AuthURL string
	// FlowID identifies the flow the callback polls.
	FlowID string
	// PollToken authorizes the poll calls; it is not the account's token.
	PollToken string
	// Device is the machine id the flow was started for.
	Device string
	// Site is the realm being signed in to.
	Site string
	// Interval is how often to poll.
	Interval time.Duration
	// Deadline is when the flow expires.
	Deadline time.Time
}

// TokenBundle is a completed sign-in.
type TokenBundle struct {
	// Site is the realm the account signed in to.
	Site string
	// Device is the machine id the flow used.
	Device string
	// Key is the minted coding plan key, empty for a Start-Plan-only account.
	Key string
	// StartToken is ZCode's session JWT, empty when the flow returned none.
	StartToken string
	// SiteToken is the Z.ai or BigModel account token, kept for team key minting.
	SiteToken string
	// BaseURL is the Messages base URL the key is served from.
	BaseURL string
	// Plan is the plan the account holds, e.g. "GLM Coding Pro".
	Plan string
	// Team reports that the credential is a seat on a team's plan.
	Team bool
	// TeamOrg and TeamProject identify the team project holding the seat.
	TeamOrg     string
	TeamProject string
	// Email, Name, and UserID identify the account.
	Email  string
	Name   string
	UserID string
	// ExpiresAt is when the Start Plan token runs out, in Unix seconds.
	ExpiresAt int64
	// TrialModels lists the models the Start Plan currently serves.
	TrialModels []string
}

// StartLogin opens ZCode's CLI sign-in flow for a realm.
func StartLogin(site string) (*AuthSession, error) {
	site = normalizeSite(site)
	pollToken, errToken := newPollToken()
	if errToken != nil {
		return nil, errToken
	}
	device := uuid.NewString()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	data, errCall := callJSON(ctx, nil, http.MethodPost, OriginZCode+OAuthInitPath, callOptions{
		auth:   "Bearer " + pollToken,
		body:   map[string]any{"provider": site},
		device: device,
	})
	if errCall != nil {
		return nil, fmt.Errorf("zcode: start sign-in: %w", errCall)
	}

	flowID := strings.TrimSpace(data.Get("flow_id").String())
	authorizeURL := strings.TrimSpace(data.Get("authorize_url").String())
	parsed, errParse := url.Parse(authorizeURL)
	if flowID == "" || errParse != nil || parsed.Scheme != "https" {
		return nil, errors.New("zcode: ZCode gave no sign-in page")
	}
	// Where Z.ai or BigModel sends the browser back, as ZCode sets it.
	back := OriginZCode + "/app/oauth/login?" + url.Values{
		"redirect":    {"zcode://oauth/callback"},
		"app_version": {AppVersion},
	}.Encode()
	param := "redirect_uri"
	if site == SiteBigModel {
		param = "redirect"
	}
	query := parsed.Query()
	query.Set(param, back)
	parsed.RawQuery = query.Encode()

	interval := time.Duration(data.Get("poll_interval_sec").Int()) * time.Second
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	deadline := time.Now().Add(LoginTimeout)
	if expires := data.Get("expires_at").Int(); expires > 0 {
		deadline = time.Unix(expires, 0)
	}

	return &AuthSession{
		AuthURL:   parsed.String(),
		FlowID:    flowID,
		PollToken: pollToken,
		Device:    device,
		Site:      site,
		Interval:  interval,
		Deadline:  deadline,
	}, nil
}

// PollResult is one poll of a sign-in flow.
type PollResult struct {
	// Pending reports that the user has not finished signing in.
	Pending bool
	// Failed reports that the sign-in was declined.
	Failed bool
	// Token is the realm's account token once the flow is ready.
	Token string
	// StartToken is ZCode's own session token, when the flow returned one.
	StartToken string
}

// Poll asks a sign-in flow once.
func Poll(ctx context.Context, client *http.Client, session *AuthSession) (PollResult, error) {
	if session == nil || session.FlowID == "" {
		return PollResult{}, errors.New("zcode: sign-in session is incomplete")
	}
	url := OriginZCode + OAuthPollPath + url.PathEscape(session.FlowID)
	data, errCall := callJSON(ctx, client, http.MethodGet, url, callOptions{
		auth:   "Bearer " + session.PollToken,
		device: session.Device,
	})
	if errCall != nil {
		var apiErr *APIError
		if errors.As(errCall, &apiErr) && apiErr.IsClientError() {
			return PollResult{Failed: true}, errCall
		}
		return PollResult{Pending: true}, nil
	}

	status := strings.ToLower(strings.TrimSpace(data.Get("status").String()))
	startToken := strings.TrimSpace(data.Get("token").String())

	var token string
	if session.Site == SiteBigModel {
		token = firstNonEmpty(
			data.Get("bigmodel.access_token").String(),
			data.Get("bigmodel.accessToken").String(),
		)
	} else {
		token = firstNonEmpty(data.Get("zai.access_token").String())
	}

	switch status {
	case "", "pending":
		return PollResult{Pending: true}, nil
	case "failed":
		return PollResult{Failed: true}, nil
	case "ready":
		if token == "" {
			return PollResult{}, fmt.Errorf("zcode: sign-in finished without a %s token", session.Site)
		}
		return PollResult{Token: token, StartToken: startToken}, nil
	default:
		return PollResult{}, fmt.Errorf("zcode: unexpected sign-in status %q", status)
	}
}

// WaitForAuthorization polls until the flow completes, fails, or expires.
func WaitForAuthorization(ctx context.Context, client *http.Client, session *AuthSession) (*TokenBundle, error) {
	if session == nil {
		return nil, errors.New("zcode: sign-in session is nil")
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, LoginTimeout)
		defer cancel()
	}
	interval := session.Interval
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var consecutiveErrors int
	const maxConsecutiveErrors = 15

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if !session.Deadline.IsZero() && time.Now().After(session.Deadline) {
				return nil, errors.New("zcode: the sign-in expired; start again")
			}
			result, errPoll := Poll(ctx, client, session)
			if errPoll != nil {
				if result.Failed {
					return nil, fmt.Errorf("zcode: the sign-in was declined on %s: %w", session.Site, errPoll)
				}
				consecutiveErrors++
				if consecutiveErrors >= maxConsecutiveErrors {
					return nil, fmt.Errorf("zcode: polling aborted after %d consecutive errors: %w", consecutiveErrors, errPoll)
				}
				continue
			}
			consecutiveErrors = 0
			if result.Pending {
				continue
			}
			if result.Failed {
				return nil, fmt.Errorf("zcode: the sign-in was declined on %s", session.Site)
			}
			return CompleteSignIn(ctx, client, session.Site, session.Device, result.Token, result.StartToken)
		}
	}
}

// CompleteSignIn turns a realm token into the credential the gateway stores.
//
// It mints (or finds) the account's coding plan key in its default project and
// reads the plan it holds. An account with no plan of its own falls back to a
// seat on a team's plan, then to ZCode's Start Plan, exactly as ZCode's own
// sign-in does.
func CompleteSignIn(ctx context.Context, client *http.Client, site, device, siteToken, startToken string) (*TokenBundle, error) {
	site = normalizeSite(site)
	bundle := &TokenBundle{
		Site:       site,
		Device:     device,
		SiteToken:  siteToken,
		StartToken: startToken,
		BaseURL:    SiteBaseURL(site),
	}
	if startToken != "" {
		bundle.ExpiresAt = jwtExpiryUnix(startToken)
	}

	var failures []string
	if siteToken != "" {
		key, plan, teamOrg, teamProject, errMint := mintKey(ctx, client, site, siteToken)
		switch {
		case errMint != nil:
			failures = append(failures, errMint.Error())
		case key != "":
			bundle.Key = key
			bundle.Plan = plan
			bundle.Team = teamOrg != "" && teamProject != ""
			bundle.TeamOrg = teamOrg
			bundle.TeamProject = teamProject
		}
	}

	if bundle.Key == "" && startToken != "" {
		plans, errStart := StartPlanModels(ctx, client, startToken, device)
		if errStart != nil {
			failures = append(failures, errStart.Error())
		} else if plans != nil {
			bundle.Plan = plans.Name
			bundle.TrialModels = plans.LiveModels
		}
	}

	if bundle.Key == "" && bundle.Plan == "" {
		if len(failures) > 0 {
			return nil, fmt.Errorf("zcode: this %s account has no usable GLM Coding Plan: %s", site, strings.Join(failures, "; "))
		}
		return nil, fmt.Errorf("zcode: this %s account has no GLM Coding Plan, of its own or a team's, and ZCode's Start Plan has ended or was never started", site)
	}

	if bundle.ExpiresAt > 0 {
		bundle.TrialModels = append([]string(nil), bundle.TrialModels...)
	}
	return bundle, nil
}

// mintKey finds or creates the account's coding plan key and reports the plan.
func mintKey(ctx context.Context, client *http.Client, site, siteToken string) (key, plan, teamOrg, teamProject string, err error) {
	auth, errAuth := bizAuth(ctx, client, site, siteToken)
	if errAuth != nil {
		return "", "", "", "", errAuth
	}
	origin := SiteOrigin(site)
	info, errInfo := callJSON(ctx, client, http.MethodGet, origin+CustomerInfoPath, callOptions{auth: auth})
	if errInfo != nil {
		return "", "", "", "", fmt.Errorf("%s account: %w", siteLabel(site), errInfo)
	}

	org, project, ok := defaultProject(info)
	if !ok {
		return "", "", "", "", fmt.Errorf("this %s account has no project for an API key", siteLabel(site))
	}
	keysURL := origin + "/api/biz/v1/organization/" + url.PathEscape(org) + "/projects/" + url.PathEscape(project) + "/api_keys"
	key, errKey := projectKey(ctx, client, site, keysURL, auth, nil, CodingPlanKeyName, "")
	if errKey != nil {
		return "", "", "", "", fmt.Errorf("GLM Coding Plan: %w", errKey)
	}
	plan, errPlan := subscriptionPlan(ctx, client, SiteBaseURL(site), key)
	if errPlan != nil {
		return "", "", "", "", fmt.Errorf("GLM Coding Plan: %w", errPlan)
	}
	if plan != "" {
		return key, plan, "", "", nil
	}

	// No plan of its own: try a seat on a team's plan.
	teamOrg, teamProject, teamPlan, errTeam := teamSeat(ctx, client, site, info, auth)
	if errTeam != nil {
		return "", "", "", "", errTeam
	}
	if teamPlan == "" {
		return "", "", "", "", errors.New("this account holds no GLM Coding Plan of its own and has no seat on a team's")
	}
	teamKeysURL := origin + "/api/biz/v1/organization/" + url.PathEscape(teamOrg) + "/projects/" + url.PathEscape(teamProject) + "/api_keys"
	teamKey, errTeamKey := projectKey(ctx, client, site, teamKeysURL, auth, teamHeaders(site, teamOrg, teamProject), TeamPlanKeyName, TeamProjectType)
	if errTeamKey != nil {
		return "", "", "", "", fmt.Errorf("the team's GLM Coding Plan: %w", errTeamKey)
	}
	return teamKey, teamPlan, teamOrg, teamProject, nil
}

// subscriptionPlan names the coding plan a key holds, "" for none.
func subscriptionPlan(ctx context.Context, client *http.Client, baseURL, key string) (string, error) {
	data, errCall := callJSON(ctx, client, http.MethodGet, originOf(baseURL)+SubscriptionListPath, callOptions{auth: key})
	if errCall != nil {
		return "", errCall
	}
	for _, item := range data.Array() {
		if strings.EqualFold(strings.TrimSpace(item.Get("status").String()), "VALID") {
			return strings.TrimSpace(item.Get("productName").String()), nil
		}
	}
	return "", nil
}

// defaultProject picks the account's default organization and project, as
// ZCode's key minting does.
func defaultProject(info gjson.Result) (org, project string, ok bool) {
	namedOrg := ""
	namedOrgProject := ""
	anyOrg := ""
	anyProject := ""
	for _, o := range info.Get("organizations").Array() {
		orgID := strings.TrimSpace(o.Get("organizationId").String())
		if orgID == "" {
			continue
		}
		projects := o.Get("projects").Array()
		projectID := ""
		for _, p := range projects {
			if strings.TrimSpace(p.Get("projectId").String()) == "" {
				continue
			}
			if strings.TrimSpace(p.Get("projectType").String()) == TeamProjectType {
				continue
			}
			id := strings.TrimSpace(p.Get("projectId").String())
			if strings.Contains(p.Get("projectName").String(), "默认项目") {
				projectID = id
				break
			}
			if projectID == "" {
				projectID = id
			}
		}
		if projectID == "" {
			continue
		}
		if strings.Contains(o.Get("organizationName").String(), "默认机构") {
			namedOrg = orgID
			namedOrgProject = projectID
			break
		}
		if anyOrg == "" {
			anyOrg = orgID
			anyProject = projectID
		}
	}
	if namedOrg != "" {
		return namedOrg, namedOrgProject, true
	}
	if anyOrg != "" {
		return anyOrg, anyProject, true
	}
	return "", "", false
}

// teamSeat finds the first team project whose plan is in force and gives the
// account a seat on it.
func teamSeat(ctx context.Context, client *http.Client, site string, info gjson.Result, auth string) (org, project, plan string, err error) {
	origin := SiteOrigin(site)
	lastErr := ""
	for _, o := range info.Get("organizations").Array() {
		orgID := strings.TrimSpace(o.Get("organizationId").String())
		if orgID == "" {
			continue
		}
		for _, p := range o.Get("projects").Array() {
			projectID := strings.TrimSpace(p.Get("projectId").String())
			if projectID == "" || strings.TrimSpace(p.Get("projectType").String()) != TeamProjectType {
				continue
			}
			detail, errDetail := callJSON(ctx, client, http.MethodGet, origin+TeamSubscribeDetailPath, callOptions{
				auth:    auth,
				headers: teamHeaders(site, orgID, projectID),
			})
			if errDetail != nil {
				lastErr = errDetail.Error()
				continue
			}
			hasSubscription := detail.Get("hasSubscription")
			status := strings.ToUpper(strings.TrimSpace(detail.Get("status").String()))
			grant := strings.ToUpper(strings.TrimSpace(detail.Get("memberGrantStatus").String()))
			if hasSubscription.Exists() && !hasSubscription.Bool() {
				continue
			}
			if status != "EFFECTIVE" || grant != "VALID" {
				if grant == "UNASSIGNED" && status == "EFFECTIVE" {
					lastErr = fmt.Sprintf("this %s account is in a team with a GLM Coding Plan but has no seat on it yet; ask the team's admin for a seat, then sign in again", siteLabel(site))
				} else if status == "EXPIRED" {
					lastErr = fmt.Sprintf("the %s team's GLM Coding Plan this account is in has expired", siteLabel(site))
				}
				continue
			}
			name := strings.TrimSpace(detail.Get("productName").String())
			if name == "" {
				name = "GLM Coding Team"
			}
			return orgID, projectID, name, nil
		}
	}
	if lastErr != "" {
		return "", "", "", errors.New(lastErr)
	}
	return "", "", "", nil
}

// projectKey finds the project's key by name, creates it when missing, and
// reads its secret as `<id>.<secret>`.
func projectKey(ctx context.Context, client *http.Client, site, keysURL, auth string, headers map[string]string, name, keyType string) (string, error) {
	label := siteLabel(site)
	list, errList := callJSON(ctx, client, http.MethodGet, keysURL, callOptions{auth: auth, headers: headers})
	if errList != nil {
		return "", fmt.Errorf("%s API keys: %w", label, errList)
	}

	keyID := ""
	for _, item := range list.Array() {
		if strings.TrimSpace(item.Get("name").String()) != name {
			continue
		}
		if keyType != "" && strings.TrimSpace(item.Get("keyType").String()) != keyType {
			continue
		}
		if id := strings.TrimSpace(item.Get("apiKey").String()); id != "" {
			keyID = id
		}
	}
	if keyID == "" {
		body := map[string]any{"name": name}
		if keyType != "" {
			body["keyType"] = keyType
		}
		created, errCreate := callJSON(ctx, client, http.MethodPost, keysURL, callOptions{auth: auth, headers: headers, body: body})
		if errCreate != nil {
			return "", fmt.Errorf("%s API key: %w", label, errCreate)
		}
		keyID = strings.TrimSpace(created.Get("apiKey").String())
	}
	if keyID == "" {
		return "", fmt.Errorf("%s gave no API key", label)
	}

	copied, errCopy := callJSON(ctx, client, http.MethodGet, keysURL+"/copy/"+url.PathEscape(keyID), callOptions{auth: auth, headers: headers})
	if errCopy != nil {
		if keyType != "" {
			// A team's key with no secret goes as it is.
			return keyID, nil
		}
		return "", fmt.Errorf("%s API key: %w", label, errCopy)
	}
	secret := strings.TrimSpace(copied.Get("secretKey").String())
	if secret == "" {
		if keyType != "" {
			return keyID, nil
		}
		return "", fmt.Errorf("%s gave no API key", label)
	}
	return keyID + "." + secret, nil
}

// bizAuth turns a realm token into the Authorization the business API wants:
// Z.ai's token is exchanged for a business bearer, BigModel's goes bare.
func bizAuth(ctx context.Context, client *http.Client, site, siteToken string) (string, error) {
	if normalizeSite(site) == SiteBigModel {
		return siteToken, nil
	}
	data, errLogin := callJSON(ctx, client, http.MethodPost, OriginZAI+ZAILoginPath, callOptions{
		body: map[string]any{"token": siteToken},
	})
	if errLogin != nil {
		return "", fmt.Errorf("Z.ai sign-in: %w", errLogin)
	}
	access := strings.TrimSpace(data.Get("access_token").String())
	if access == "" {
		return "", errors.New("Z.ai sign-in: no token")
	}
	return "Bearer " + access, nil
}

// teamHeaders are the organization headers a team project's calls carry.
func teamHeaders(site, org, project string) map[string]string {
	return map[string]string{
		"Bigmodel-Organization": org,
		"Bigmodel-Project":      project,
		"Set-Language":          siteLanguage(site),
		"Accept-Language":       "en-US,en",
	}
}

func siteLanguage(site string) string {
	if normalizeSite(site) == SiteBigModel {
		return "zh"
	}
	return "en"
}

func siteLabel(site string) string {
	if normalizeSite(site) == SiteBigModel {
		return "BigModel"
	}
	return "Z.ai"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// newPollToken makes the bearer a sign-in flow is polled with.
func newPollToken() (string, error) {
	buf := make([]byte, 32)
	if _, errRead := rand.Read(buf); errRead != nil {
		return "", fmt.Errorf("zcode: generate poll token: %w", errRead)
	}
	return hex.EncodeToString(buf), nil
}

// base64URLDecode decodes a JWT segment.
func base64URLDecode(segment string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(segment, "="))
}
