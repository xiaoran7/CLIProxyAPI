package zcode

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

// Window is one allowance window: a Coding Plan limit or a Start Plan bucket.
type Window struct {
	// Name is the window's label, e.g. "5 hours", "Weekly", or a bucket's name.
	Name string
	// UsedPercent is how much of the window is spent, 0-100.
	UsedPercent float64
	// Used and Total are the window's counts, when the upstream reports them.
	Used  float64
	Total float64
	// HasCount reports that Used and Total are meaningful.
	HasCount bool
	// ResetsAt is when the window refills, zero when unstated.
	ResetsAt time.Time
	// Span is the window's length in seconds, zero when unstated.
	Span int
	// Models lists the models a Start Plan bucket serves.
	Models []string
	// Spent reports a bucket past its allowance.
	Spent bool
	// Aside reports a window that is shown but holds nothing back, such as the
	// MCP tool-call count or a gift plan waiting to be claimed.
	Aside bool
	// Display is the upstream's own rendering of the count, when it gave one.
	Display string
	// Plan names the gift plan a bucket belongs to.
	Plan string
	// Gift reports a bucket that spends ZCode's Start Plan rather than a
	// coding plan. The two are separate pools: a request spends one, never both.
	Gift bool
}

// Usage is everything a credential's quota read produced.
type Usage struct {
	// Plan is the plan's name, e.g. "GLM Coding Pro" or "Start Plan".
	Plan string
	// Windows are the plan's allowance windows.
	Windows []Window
	// Until is when the plan itself ends, zero when unstated.
	Until time.Time
	// Renew reports the plan's renewal mode: "auto", "off", or "".
	Renew string
	// Resets reports how many allowance resets the plan still holds.
	Resets *ResetAllowance
	// Error carries a user-facing explanation when no plan could be read.
	Error string
}

// ResetAllowance is how many resets a plan still holds.
type ResetAllowance struct {
	// Count is the total number of resets still available.
	Count int
	// FiveHour and Weekly split it by window.
	FiveHour int
	Weekly   int
	// Until is the soonest reset expiry, zero when unstated.
	Until time.Time
	// Team reports a team member's resets.
	Team bool
}

// QueryCodingPlan reads a Coding Plan key's allowance.
func QueryCodingPlan(ctx context.Context, client *http.Client, baseURL, key string) (*Usage, error) {
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("zcode: no coding plan key")
	}
	root := originOf(baseURL)
	data, errLimit := callJSON(ctx, client, http.MethodGet, root+QuotaLimitPath, callOptions{auth: key})
	if errLimit != nil {
		return nil, fmt.Errorf("zcode: read coding plan usage: %w", errLimit)
	}

	usage := &Usage{Windows: limitWindows(data)}
	if level := strings.TrimSpace(data.Get("level").String()); level != "" {
		r, size := utf8.DecodeRuneInString(level)
		usage.Plan = "GLM Coding " + string(unicode.ToUpper(r)) + level[size:]
	}
	if usage.Plan == "" {
		usage.Plan = "GLM Coding Plan"
	}

	if subs, errSubs := callJSON(ctx, client, http.MethodGet, root+SubscriptionListPath, callOptions{auth: key}); errSubs == nil {
		until, renew := termOf(subs)
		usage.Until = until
		usage.Renew = renew
	}
	usage.Resets = readResets(ctx, client, root+PackageResetListPath+"?targetType=PERSONAL", key, nil)
	return usage, nil
}

// QueryTeamPlan reads a team seat's allowance.
func QueryTeamPlan(ctx context.Context, client *http.Client, baseURL, site, key, siteToken, org, project string) (*Usage, error) {
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("zcode: no team plan key")
	}
	root := originOf(baseURL)
	headers := teamHeaders(site, org, project)
	data, errLimit := callJSON(ctx, client, http.MethodGet, root+QuotaLimitPath+"?type=2", callOptions{auth: key, headers: headers})
	if errLimit != nil {
		return nil, fmt.Errorf("zcode: read team plan usage: %w", errLimit)
	}
	usage := &Usage{Plan: "GLM Coding Team", Windows: limitWindows(data)}

	if siteToken != "" {
		if detail, errDetail := callJSON(ctx, client, http.MethodGet, root+TeamSubscribeDetailPath, callOptions{auth: siteToken, headers: headers}); errDetail == nil {
			hasSubscription := detail.Get("hasSubscription")
			status := strings.ToUpper(strings.TrimSpace(detail.Get("status").String()))
			grant := strings.ToUpper(strings.TrimSpace(detail.Get("memberGrantStatus").String()))
			usable := (!hasSubscription.Exists() || hasSubscription.Bool()) && status == "EFFECTIVE" && grant == "VALID"
			if usable {
				if name := strings.TrimSpace(detail.Get("productName").String()); name != "" {
					usage.Plan = name
				}
				usage.Until = parsePlanTime(detail.Get("subscribeEndTime"))
			}
		}
		usage.Resets = readResets(ctx, client, root+PackageResetListPath+"?targetType=TEAM", siteToken, headers)
		if usage.Resets != nil {
			usage.Resets.Team = true
		}
	}
	return usage, nil
}

// StartPlan is what ZCode's gift can serve right now.
type StartPlan struct {
	// Name is the soonest-ending live plan's name.
	Name string
	// LiveModels lists the models a live, unspent bucket serves.
	LiveModels []string
	// AllModels lists every model the live buckets declare.
	AllModels []string
	// TTL bounds how long this answer stays usable.
	TTL time.Duration
}

// StartPlanModels reads ZCode's gift balance and reports the models it serves.
func StartPlanModels(ctx context.Context, client *http.Client, jwt, device string) (*StartPlan, error) {
	if strings.TrimSpace(jwt) == "" {
		return nil, errNotSignedIn
	}
	if JWTExpired(jwt, nowFunc()) {
		return nil, errStartTokenExpired
	}
	data, errCall := callJSON(ctx, client, http.MethodGet, zcodeOrigin+BillingBalancePath+"?app_version="+AppVersion, callOptions{
		auth:   "Bearer " + jwt,
		device: device,
	})
	if errCall != nil {
		return nil, fmt.Errorf("zcode: read Start Plan balance: %w", errCall)
	}

	now := startNow(data)
	plans := data.Get("plans").Array()
	balances := data.Get("balances").Array()

	live := map[string]struct{}{}
	all := map[string]struct{}{}
	ttl := 10 * time.Minute
	anyPlan := false
	name := ""
	for _, plan := range plans {
		status := strings.ToLower(strings.TrimSpace(plan.Get("status").String()))
		ends := startNumber(plan.Get("ends_at"))
		if status != "active" || (ends > 0 && ends <= now) {
			continue
		}
		anyPlan = true
		if ends > 0 {
			ttl = minDuration(ttl, time.Duration((ends-now)*float64(time.Second)))
		}
		if name == "" {
			name = strings.TrimSpace(plan.Get("name").String())
		}
		for _, balance := range balances {
			if !samePlan(balance, plan) {
				continue
			}
			expires := startNumber(balance.Get("expires_at"))
			if expires > 0 && expires <= now {
				continue
			}
			if expires > 0 {
				ttl = minDuration(ttl, time.Duration((expires-now)*float64(time.Second)))
			}
			spent := giftSpent(balance)
			for _, model := range bucketModels(balance) {
				all[strings.ToLower(model)] = struct{}{}
				if !spent {
					live[strings.ToLower(model)] = struct{}{}
				}
			}
		}
	}
	if !anyPlan {
		return nil, nil
	}
	if name == "" {
		name = "Start Plan"
	}
	return &StartPlan{
		Name:       name,
		LiveModels: sortedKeys(live),
		AllModels:  sortedKeys(all),
		TTL:        ttl,
	}, nil
}

// QueryStartPlan reads a gift-only account's allowance.
func QueryStartPlan(ctx context.Context, client *http.Client, jwt, device string) (*Usage, error) {
	if strings.TrimSpace(jwt) == "" {
		return nil, errNotSignedIn
	}
	if JWTExpired(jwt, nowFunc()) {
		return nil, errStartTokenExpired
	}
	data, errCall := callJSON(ctx, client, http.MethodGet, zcodeOrigin+BillingBalancePath+"?app_version="+AppVersion, callOptions{
		auth:   "Bearer " + jwt,
		device: device,
	})
	if errCall != nil {
		return nil, fmt.Errorf("zcode: read Start Plan balance: %w", errCall)
	}

	now := startNow(data)
	plans := data.Get("plans").Array()
	usage := &Usage{}
	soonest := time.Time{}
	for _, plan := range plans {
		status := strings.ToLower(strings.TrimSpace(plan.Get("status").String()))
		ends := startNumber(plan.Get("ends_at"))
		if status != "active" || (ends > 0 && ends <= now) {
			continue
		}
		if usage.Plan == "" {
			usage.Plan = strings.TrimSpace(plan.Get("name").String())
		}
		if ends > 0 {
			end := time.Unix(int64(ends), 0)
			if soonest.IsZero() || end.Before(soonest) {
				soonest = end
				usage.Plan = strings.TrimSpace(plan.Get("name").String())
			}
		}
	}
	if usage.Plan == "" {
		usage.Error = "this account has no GLM Coding Plan, and ZCode's Start Plan has ended or was never started"
		return usage, nil
	}
	usage.Until = soonest
	usage.Renew = "off"

	for _, balance := range data.Get("balances").Array() {
		owner, ok := ownerPlan(balance, plans, now)
		if !ok {
			continue
		}
		window, okWindow := startWindow(balance, owner)
		if !okWindow {
			continue
		}
		usage.Windows = append(usage.Windows, window)
	}
	return usage, nil
}

// startWindow renders one balance bucket as a window.
func startWindow(balance, owner gjson.Result) (Window, bool) {
	total := startNumber(balance.Get("total_units"))
	left := startNumber(balance.Get("remaining_units"))
	used := startNumber(balance.Get("used_units"))
	if total == 0 && left == 0 && used == 0 {
		return Window{}, false
	}
	models := bucketModels(balance)
	name := strings.TrimSpace(balance.Get("show_name").String())
	if name == "" {
		name = strings.Join(models, ", ")
	}
	if name == "" {
		name = "Credits"
	}

	window := Window{
		Name:   name,
		Models: models,
		Plan:   strings.TrimSpace(owner.Get("name").String()),
		Spent:  giftSpent(balance),
		Gift:   true,
	}
	if total > 0 {
		if used == 0 && left > 0 {
			used = total - left
		}
		window.UsedPercent = 100 * used / total
		window.Used = used
		window.Total = total
		window.HasCount = true
		window.Display = compactNumber(used) + " / " + compactNumber(total)
	}
	if window.Spent {
		window.UsedPercent = maxFloat(window.UsedPercent, 100)
	}
	if expires := startNumber(balance.Get("expires_at")); expires > 0 {
		window.ResetsAt = time.Unix(int64(expires), 0)
	}
	window.Span = bucketSpan(balance, owner)
	return window, true
}

// bucketSpan reads a bucket's window length from its plan's entitlements, or
// from its own period when the entitlement is unstated.
func bucketSpan(balance, owner gjson.Result) int {
	entitlement := strings.TrimSpace(balance.Get("entitlement_id").String())
	for _, item := range owner.Get("entitlements").Array() {
		if strings.TrimSpace(item.Get("entitlement_id").String()) == entitlement {
			if span := periodSeconds(strings.TrimSpace(item.Get("period").String())); span > 0 {
				return span
			}
		}
	}
	start := startNumber(balance.Get("period_start"))
	end := startNumber(balance.Get("period_end"))
	if start > 0 && end > start {
		return int(end - start)
	}
	return 0
}

// periodSeconds reads an entitlement's period name.
func periodSeconds(period string) int {
	lower := strings.ToLower(period)
	switch {
	case strings.Contains(lower, "day"):
		return 86400
	case strings.Contains(lower, "week"):
		return 7 * 86400
	case strings.Contains(lower, "month"):
		return 30 * 86400
	default:
		return 0
	}
}

// limitWindows renders /api/monitor/usage/quota/limit's limits as windows.
//
// A limit whose whole is reported as 0 has no cap and is set aside; so is the
// month's MCP tool-call count, which ZCode shows but never stops the models on.
func limitWindows(data gjson.Result) []Window {
	var windows []Window
	for _, item := range data.Get("limits").Array() {
		span := spanSeconds(int(item.Get("unit").Int()), item.Get("number").Float())
		window := Window{Name: windowName(span)}
		if strings.EqualFold(strings.TrimSpace(item.Get("type").String()), "TIME_LIMIT") {
			window.Name = "MCP · Month"
			window.Aside = true
		}
		percentage := item.Get("percentage")
		if percentage.Exists() {
			window.UsedPercent = percentage.Float()
		}
		if limitUsage := item.Get("usage"); limitUsage.Exists() {
			total := limitUsage.Float()
			if total == 0 {
				window.UsedPercent = 0
				window.Aside = true
			} else if total > 0 {
				switch {
				case item.Get("remaining").Exists():
					used := total - item.Get("remaining").Float()
					window.UsedPercent = 100 * used / total
					window.Used = used
				case item.Get("currentValue").Exists():
					used := item.Get("currentValue").Float()
					if !percentage.Exists() {
						window.UsedPercent = 100 * used / total
					}
					window.Used = used
				}
				window.Total = total
				window.HasCount = true
				window.Display = compactNumber(window.Used) + " / " + compactNumber(total)
			}
		}
		if reset := item.Get("nextResetTime").Int(); reset > 0 {
			window.ResetsAt = time.Unix(reset, 0)
		}
		window.Span = span
		windows = append(windows, window)
	}
	return windows
}

// spanSeconds reads a limit's window length. Unit 3 counts hours, 6 weeks, and
// 1, 4, 5 minutes, days and months by the same count; a team's five hours come
// with no count.
func spanSeconds(unit int, count float64) int {
	n := int(count)
	if unit == 3 && n <= 0 {
		n = 5
	}
	if n < 1 {
		n = 1
	}
	switch unit {
	case 1:
		return 60 * n
	case 3:
		return 3600 * n
	case 4:
		return 86400 * n
	case 5:
		return 30 * 86400 * n
	case 6:
		return 7 * 86400 * n
	default:
		return 0
	}
}

// windowName labels a window by its length.
func windowName(span int) string {
	switch {
	case span == 0:
		return "Credits"
	case span < 86400:
		return fmt.Sprintf("%d hours", span/3600)
	case span == 7*86400:
		return "Weekly"
	case span >= 28*86400:
		return "Monthly"
	default:
		return fmt.Sprintf("%d days", span/86400)
	}
}

// termOf reads how long a paid plan runs: the valid subscription's next
// renewal, or the end of its validity.
func termOf(subs gjson.Result) (time.Time, string) {
	for _, item := range subs.Array() {
		if !strings.EqualFold(strings.TrimSpace(item.Get("status").String()), "VALID") {
			continue
		}
		auto := item.Get("autoRenew")
		renew := "off"
		if (auto.Type == gjson.True) || (auto.Type == gjson.Number && auto.Int() == 1) {
			renew = "auto"
		}
		if until := parseBeijing(item.Get("nextRenewTime").String()); !until.IsZero() {
			return until, renew
		}
		if renew == "off" {
			dates := beijingDatePattern.FindAllString(item.Get("valid").String(), -1)
			if len(dates) > 0 {
				if until := parseBeijing(dates[len(dates)-1]); !until.IsZero() {
					return until, renew
				}
			}
		}
		return time.Time{}, ""
	}
	return time.Time{}, ""
}

// readResets reads the resets a plan still holds, or nil when none can be told.
func readResets(ctx context.Context, client *http.Client, url, auth string, headers map[string]string) *ResetAllowance {
	root, errCall := callJSON(ctx, client, http.MethodGet, url, callOptions{auth: auth, headers: headers})
	if errCall != nil {
		return nil
	}
	fiveHour := countAvailable(root.Get("fiveHourResets"))
	weekly := countAvailable(root.Get("weekResets"))
	if fiveHour+weekly == 0 {
		return nil
	}
	allowance := &ResetAllowance{Count: fiveHour + weekly, FiveHour: fiveHour, Weekly: weekly}
	soonest := time.Time{}
	for _, list := range []gjson.Result{root.Get("fiveHourResets"), root.Get("weekResets")} {
		for _, item := range list.Array() {
			if !item.Get("available").Bool() {
				continue
			}
			if until := parsePlanTime(item.Get("expireTime")); !until.IsZero() {
				if soonest.IsZero() || until.Before(soonest) {
					soonest = until
				}
			}
		}
	}
	allowance.Until = soonest
	return allowance
}

func countAvailable(list gjson.Result) int {
	count := 0
	for _, item := range list.Array() {
		if item.Get("available").Bool() {
			count++
		}
	}
	return count
}

// giftSpent reports a bucket with nothing left to spend. Older balances omit
// remaining_units but still tell used and total.
func giftSpent(balance gjson.Result) bool {
	remaining := balance.Get("remaining_units")
	if remaining.Exists() {
		return remaining.Float() <= 0
	}
	total := balance.Get("total_units")
	used := balance.Get("used_units")
	if total.Exists() && used.Exists() {
		return used.Float() >= total.Float()
	}
	return false
}

// bucketModels reads a bucket's model capabilities.
func bucketModels(balance gjson.Result) []string {
	var models []string
	seen := map[string]struct{}{}
	for _, raw := range balance.Get("capabilities").Array() {
		value := strings.TrimSpace(raw.String())
		if value == "" || (strings.Contains(value, ":") && !strings.HasPrefix(value, "model:")) {
			continue
		}
		value = strings.TrimPrefix(value, "model:")
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[strings.ToLower(value)]; ok {
			continue
		}
		seen[strings.ToLower(value)] = struct{}{}
		models = append(models, value)
	}
	return models
}

// samePlan matches a bucket to the plan that owns it.
func samePlan(balance, plan gjson.Result) bool {
	balancePlanID := strings.TrimSpace(balance.Get("user_plan_id").String())
	planPlanID := strings.TrimSpace(plan.Get("user_plan_id").String())
	if balancePlanID != "" && planPlanID != "" {
		return balancePlanID == planPlanID
	}
	return strings.TrimSpace(balance.Get("plan_id").String()) == strings.TrimSpace(plan.Get("plan_id").String())
}

// ownerPlan finds the live plan a bucket belongs to.
func ownerPlan(balance gjson.Result, plans []gjson.Result, now float64) (gjson.Result, bool) {
	expires := startNumber(balance.Get("expires_at"))
	if expires > 0 && expires <= now {
		return gjson.Result{}, false
	}
	for _, plan := range plans {
		status := strings.ToLower(strings.TrimSpace(plan.Get("status").String()))
		ends := startNumber(plan.Get("ends_at"))
		if status != "active" || (ends > 0 && ends <= now) {
			continue
		}
		if samePlan(balance, plan) {
			return plan, true
		}
	}
	return gjson.Result{}, false
}

// startNow reads the balance's own clock, falling back to this machine's.
func startNow(data gjson.Result) float64 {
	if server := startNumber(data.Get("server_time")); server > 0 {
		return server
	}
	return float64(nowFunc().Unix())
}

// startNumber reads a number the balance gives as a number or a string.
func startNumber(value gjson.Result) float64 {
	switch value.Type {
	case gjson.Number:
		return value.Float()
	case gjson.String:
		text := strings.TrimSpace(value.String())
		if text == "" {
			return 0
		}
		parsed, errParse := strconv.ParseFloat(text, 64)
		if errParse != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

// originOf returns the scheme and host of a base URL.
func originOf(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return OriginZAI
	}
	if idx := strings.Index(trimmed, "://"); idx > 0 {
		rest := trimmed[idx+3:]
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			rest = rest[:slash]
		}
		return trimmed[:idx+3] + rest
	}
	return OriginZAI
}

// compactNumber renders a count whole, else to two decimal places.
func compactNumber(value float64) string {
	if value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	text := strconv.FormatFloat(value, 'f', 2, 64)
	text = strings.TrimRight(text, "0")
	return strings.TrimSuffix(text, ".")
}

func minDuration(a, b time.Duration) time.Duration {
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return keys
}
