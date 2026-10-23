// Package zcode provides authentication, key minting, Start Plan dressing, and
// quota management for ZCode (zcode.z.ai), the Z.ai and BigModel GLM Coding
// Plans, and ZCode's free Start Plan.
package zcode

import (
	"strings"
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

const (
	// SiteZAI identifies the Z.ai realm (z.ai / api.z.ai).
	SiteZAI = "zai"
	// SiteBigModel identifies the BigModel realm (bigmodel.cn).
	SiteBigModel = "bigmodel"

	// OriginZCode serves ZCode's sign-in, the Start Plan, and its billing APIs.
	OriginZCode = "https://zcode.z.ai"
	// OriginZAI is Z.ai's business API origin.
	OriginZAI = "https://api.z.ai"
	// OriginBigModel is BigModel's business API origin.
	OriginBigModel = "https://bigmodel.cn"

	// BaseURLZAI is the GLM Coding Plan Messages endpoint on Z.ai.
	BaseURLZAI = "https://api.z.ai/api/anthropic"
	// BaseURLBigModel is the GLM Coding Plan Messages endpoint on BigModel.
	BaseURLBigModel = "https://open.bigmodel.cn/api/anthropic"
	// BaseURLStartPlan is the Start Plan's Messages endpoint on zcode.z.ai.
	BaseURLStartPlan = OriginZCode + "/api/v1/zcode-plan/anthropic"

	// AppVersion is the ZCode client version whose wire shape this package reproduces.
	AppVersion = "3.16.3"
	// UserAgent is the ZCode client's User-Agent.
	UserAgent = "ZCode/" + AppVersion
	// MessagesUserAgent is the User-Agent ZCode's Messages calls carry.
	MessagesUserAgent = UserAgent + " ai-sdk/anthropic/3.0.81"

	// OAuthInitPath starts ZCode's CLI sign-in flow.
	OAuthInitPath = "/api/v1/oauth/cli/init"
	// OAuthPollPath polls a ZCode CLI sign-in flow.
	OAuthPollPath = "/api/v1/oauth/cli/poll/"

	// BillingBalancePath reads the Start Plan's plans and buckets.
	BillingBalancePath = "/api/v1/zcode-plan/billing/balance"
	// BillingPreviewPath lists gift plans ZCode holds but has not claimed.
	BillingPreviewPath = "/api/v1/zcode-plan/billing/preview"

	// CustomerInfoPath lists the account's organizations and projects.
	CustomerInfoPath = "/api/biz/customer/getCustomerInfo"
	// SubscriptionListPath lists the account's coding plan subscriptions.
	SubscriptionListPath = "/api/biz/subscription/list"
	// QuotaLimitPath reads the Coding Plan's usage windows.
	QuotaLimitPath = "/api/monitor/usage/quota/limit"
	// TeamSubscribeDetailPath reads a team seat's plan detail.
	TeamSubscribeDetailPath = "/api/biz/team/subscribe/product/querySubscribeDetail"
	// PackageResetListPath lists the resets a plan still holds.
	PackageResetListPath = "/api/biz/customer-package-reset/list"
	// ZAILoginPath exchanges a Z.ai token for a business token.
	ZAILoginPath = "/api/auth/z/login"

	// CodingPlanKeyName is the API key ZCode mints in the account's default project.
	CodingPlanKeyName = "zcode-api-key"
	// TeamPlanKeyName is the API key ZCode mints in a team project.
	TeamPlanKeyName = "zcode-team-api-key"
	// TeamProjectType marks a project that holds a team plan.
	TeamProjectType = "2"

	// TrialSuffix marks a catalog entry that spends ZCode's gift quota.
	TrialSuffix = "-Trial"
)

// zcodeOrigin is the origin ZCode's own APIs are read from. It is a variable
// so a test can point the billing and sign-in reads at a local server.
var zcodeOrigin = OriginZCode

// nowFunc is the clock the expiry checks read. Tests set it instead of waiting.
var nowFunc = time.Now

// ResolveSite determines which realm an auth belongs to.
func ResolveSite(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return SiteZAI
	}
	if auth.Attributes != nil {
		if s := strings.TrimSpace(auth.Attributes["site"]); s != "" {
			return normalizeSite(s)
		}
		if base := strings.ToLower(auth.Attributes["base_url"]); strings.Contains(base, "bigmodel.cn") {
			return SiteBigModel
		}
	}
	if auth.Metadata != nil {
		if s, ok := auth.Metadata["site"].(string); ok && strings.TrimSpace(s) != "" {
			return normalizeSite(s)
		}
	}
	return normalizeSite(auth.Provider)
}

func normalizeSite(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case SiteBigModel, "zcode-bigmodel", "zcode-cn", "bigmodel-cn", "cn":
		return SiteBigModel
	default:
		return SiteZAI
	}
}

// SiteBaseURL returns the Coding Plan Messages base URL for a site.
func SiteBaseURL(site string) string {
	if normalizeSite(site) == SiteBigModel {
		return BaseURLBigModel
	}
	return BaseURLZAI
}

// SiteOrigin returns the business API origin for a site.
func SiteOrigin(site string) string {
	if normalizeSite(site) == SiteBigModel {
		return OriginBigModel
	}
	return OriginZAI
}

// ResolveBaseURL returns the Coding Plan base URL for an auth, honoring a
// configured base_url override.
func ResolveBaseURL(auth *cliproxyauth.Auth) string {
	base := SiteBaseURL(ResolveSite(auth))
	if auth == nil {
		return base
	}
	if auth.Attributes != nil {
		if bu := strings.TrimSpace(auth.Attributes["base_url"]); bu != "" {
			return strings.TrimRight(bu, "/")
		}
	}
	if auth.Metadata != nil {
		if bu, ok := auth.Metadata["base_url"].(string); ok && strings.TrimSpace(bu) != "" {
			return strings.TrimRight(bu, "/")
		}
	}
	return base
}

// AccessKey returns the account's minted `<id>.<secret>` coding plan key.
func AccessKey(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	for _, k := range []string{"api_key", "key", "coding_plan_key"} {
		if auth.Attributes != nil {
			if v := strings.TrimSpace(auth.Attributes[k]); v != "" {
				return v
			}
		}
		if auth.Metadata != nil {
			if v, ok := auth.Metadata[k].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// StartToken returns ZCode's own session JWT, which the Start Plan spends.
func StartToken(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	for _, k := range []string{"zcode_token", "jwt", "start_token"} {
		if auth.Attributes != nil {
			if v := strings.TrimSpace(auth.Attributes[k]); v != "" {
				return strings.TrimPrefix(v, "Bearer ")
			}
		}
		if auth.Metadata != nil {
			if v, ok := auth.Metadata[k].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimPrefix(strings.TrimSpace(v), "Bearer ")
			}
		}
	}
	return ""
}

// SiteToken returns the Z.ai or BigModel account token kept for team key minting.
func SiteToken(auth *cliproxyauth.Auth) string {
	if auth == nil || auth.Metadata == nil {
		return ""
	}
	if v, ok := auth.Metadata["site_token"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// TeamOrg returns the team project's organization id for a team seat.
func TeamOrg(auth *cliproxyauth.Auth) string {
	if auth == nil || auth.Metadata == nil {
		return ""
	}
	if v, ok := auth.Metadata["team_org"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// TeamProject returns the team project id for a team seat.
func TeamProject(auth *cliproxyauth.Auth) string {
	if auth == nil || auth.Metadata == nil {
		return ""
	}
	if v, ok := auth.Metadata["team_project"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// DeviceID returns the device id a sign-in saved, or "" when none was kept.
func DeviceID(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	if auth.Attributes != nil {
		if v := strings.TrimSpace(auth.Attributes["device_id"]); v != "" {
			return v
		}
	}
	if auth.Metadata != nil {
		if v, ok := auth.Metadata["device_id"].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// PlanName returns the plan the sign-in saved, e.g. "GLM Coding Pro".
func PlanName(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	if auth.Attributes != nil {
		if v := strings.TrimSpace(auth.Attributes["plan"]); v != "" {
			return v
		}
	}
	if auth.Metadata != nil {
		if v, ok := auth.Metadata["plan"].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// IsTeamSeat reports whether the credential is a seat on a team's plan.
func IsTeamSeat(auth *cliproxyauth.Auth) bool {
	if auth == nil {
		return false
	}
	if auth.Attributes != nil && strings.EqualFold(strings.TrimSpace(auth.Attributes["team"]), "true") {
		return true
	}
	return TeamOrg(auth) != "" && TeamProject(auth) != ""
}

// UserLabel returns the account label a sign-in saved.
func UserLabel(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return "ZCode"
	}
	for _, key := range []string{"email", "nickname", "user_id"} {
		if auth.Metadata != nil {
			if v, ok := auth.Metadata[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	if auth.Label != "" {
		return auth.Label
	}
	return "ZCode"
}

// SplitTrialModel reports whether a catalog model name asks for the gift quota,
// returning the upstream model name either way.
func SplitTrialModel(model string) (string, bool) {
	model = strings.TrimSpace(model)
	if len(model) > len(TrialSuffix) && strings.EqualFold(model[len(model)-len(TrialSuffix):], TrialSuffix) {
		return model[:len(model)-len(TrialSuffix)], true
	}
	return model, false
}

// Route describes where one request goes and what credential it carries.
type Route struct {
	// Start reports that the request goes to ZCode's Start Plan.
	Start bool
	// Trial reports that the caller explicitly picked a gift entry.
	Trial bool
	// Site is the realm the account signed in to.
	Site string
	// BaseURL is the Messages base URL to call.
	BaseURL string
	// Model is the upstream model name with any trial suffix removed.
	Model string
	// Key is the coding plan key, empty for a Start Plan request.
	Key string
	// JWT is ZCode's session token, empty for a coding plan request.
	JWT string
	// Device is the device id the Start Plan dressing names.
	Device string
}

// ResolveRoute decides whether a model spends the coding plan or the gift.
//
// A credential with a minted key uses the coding plan unless the model names a
// trial entry; a credential without one spends the Start Plan for everything.
// An explicit `zcode_route` attribute ("start" or "coding") overrides both.
func ResolveRoute(auth *cliproxyauth.Auth, model string) (Route, error) {
	site := ResolveSite(auth)
	raw, trial := SplitTrialModel(model)
	key := AccessKey(auth)
	jwt := StartToken(auth)

	forced := ""
	if auth != nil && auth.Attributes != nil {
		forced = strings.ToLower(strings.TrimSpace(auth.Attributes["zcode_route"]))
	}

	start := key == "" || forced == "start"
	if trial {
		start = true
	}
	if forced == "coding" && key != "" {
		start = false
		trial = false
	}

	route := Route{
		Start:  start,
		Trial:  trial,
		Site:   site,
		Model:  raw,
		Device: DeviceID(auth),
	}
	if start {
		if jwt == "" {
			if trial {
				return Route{}, errNoStartToken
			}
			return Route{}, errNoCredential
		}
		route.BaseURL = BaseURLStartPlan
		route.JWT = jwt
		return route, nil
	}
	route.BaseURL = ResolveBaseURL(auth)
	route.Key = key
	return route, nil
}
