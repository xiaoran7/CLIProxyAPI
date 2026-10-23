package zcode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestClassifyRefusal(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   RefusalKind
	}{
		{"ok answer", 200, `{"id":"msg_1","content":[]}`, RefusalNone},
		{"405 is a refusal", 405, ``, RefusalBlocked},
		{"edge block page", 403, `{"message":"Request has been blocked by WAF"}`, RefusalBlocked},
		{"aliyun edge page", 403, `{"message":"visit errors.aliyun.com for details"}`, RefusalBlocked},
		{"code 3012", 400, `{"code":3012,"msg":"unusual request"}`, RefusalBlocked},
		{"unusual activity", 400, `{"msg":"we saw unusual activity"}`, RefusalBlocked},
		{"a 200 mentioning unusual activity is not a refusal", 200, `{"msg":"we saw unusual activity"}`, RefusalNone},
		{"429 spends nothing", 429, ``, RefusalQuota},
		{"1113 in a 200", 200, `{"code":1113,"msg":"quota exhausted"}`, RefusalQuota},
		{"1005 under error", 200, `{"error":{"code":1005,"message":"no quota"}}`, RefusalQuota},
		{"a plain 400 is not a refusal", 400, `{"error":{"type":"invalid_request_error"}}`, RefusalNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyRefusal(tc.status, []byte(tc.body)); got != tc.want {
				t.Errorf("ClassifyRefusal(%d, %s) = %v, want %v", tc.status, tc.body, got, tc.want)
			}
		})
	}
}

func TestRefusalMessage(t *testing.T) {
	blocked := RefusalMessage(RefusalBlocked, "")
	if blocked != StartBlockedHint {
		t.Errorf("blocked message = %q", blocked)
	}
	withUpstream := RefusalMessage(RefusalBlocked, "edge says no")
	if !strings.HasPrefix(withUpstream, "edge says no") || !strings.Contains(withUpstream, StartBlockedHint) {
		t.Errorf("blocked message dropped the upstream words: %q", withUpstream)
	}
	if got := RefusalMessage(RefusalQuota, ""); got == "" {
		t.Errorf("quota message is empty")
	}
	if got := TrialExhaustedMessage("GLM-5.2", true); !strings.Contains(got, "GLM-5.2") || !strings.Contains(got, "exhausted") {
		t.Errorf("trial message = %q", got)
	}
}

func TestQueryCodingPlan_Windows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case QuotaLimitPath:
			// A coding plan key goes on Authorization as it was minted.
			if r.Header.Get("Authorization") != "id.secret" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":401,"msg":"bad key"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"level": "pro",
					"limits": []any{
						map[string]any{
							"type":          "TOKENS_LIMIT",
							"unit":          3,
							"number":        5,
							"percentage":    42.5,
							"nextResetTime": 1790000000000,
						},
						map[string]any{
							"type":       "TOKENS_LIMIT",
							"unit":       6,
							"number":     1,
							"percentage": 10,
						},
						map[string]any{
							"type":       "TIME_LIMIT",
							"unit":       5,
							"number":     1,
							"percentage": 80,
						},
					},
				},
			})
		case SubscriptionListPath:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": []any{map[string]any{
					"status":        "VALID",
					"autoRenew":     true,
					"nextRenewTime": "2026-11-24 10:00:00",
				}},
			})
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{}})
		}
	}))
	defer srv.Close()

	usage, errQuery := QueryCodingPlan(context.Background(), srv.Client(), srv.URL+"/api/anthropic", "id.secret")
	if errQuery != nil {
		t.Fatalf("QueryCodingPlan error: %v", errQuery)
	}
	if usage.Plan != "GLM Coding Pro" {
		t.Errorf("Plan = %q, want GLM Coding Pro", usage.Plan)
	}
	if len(usage.Windows) != 3 {
		t.Fatalf("windows = %d, want 3", len(usage.Windows))
	}
	fiveHour := usage.Windows[0]
	if fiveHour.Name != "5 hours" {
		t.Errorf("window 0 name = %q, want 5 hours", fiveHour.Name)
	}
	if fiveHour.UsedPercent != 42.5 {
		t.Errorf("window 0 used = %v, want 42.5", fiveHour.UsedPercent)
	}
	if fiveHour.ResetsAt.IsZero() {
		t.Errorf("window 0 has no reset time")
	}
	if got := usage.Windows[1].Name; got != "Weekly" {
		t.Errorf("window 1 name = %q, want Weekly", got)
	}
	if !usage.Windows[2].Aside {
		t.Errorf("the MCP tool-call limit should be set aside")
	}
	if usage.Renew != "auto" {
		t.Errorf("Renew = %q, want auto", usage.Renew)
	}
	if usage.Until.IsZero() {
		t.Errorf("Until is zero")
	}
}

func TestQueryCodingPlan_NoKey(t *testing.T) {
	if _, errQuery := QueryCodingPlan(context.Background(), http.DefaultClient, BaseURLZAI, ""); errQuery == nil {
		t.Fatal("QueryCodingPlan accepted an empty key")
	}
}

func TestQueryStartPlan_Buckets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, BillingBalancePath) {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Device-Mid") != "dev-1" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"code":3012}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"server_time": 1790000000,
				"plans": []any{map[string]any{
					"user_plan_id": "p1",
					"name":         "Start Plan",
					"status":       "active",
					"ends_at":      1790100000,
				}},
				"balances": []any{
					map[string]any{
						"user_plan_id":    "p1",
						"plan_id":         "plan-a",
						"show_name":       "GLM-5.2",
						"total_units":     100,
						"used_units":      25,
						"remaining_units": 75,
						"expires_at":      1790050000,
						"capabilities":    []any{"model:GLM-5.2", "tool:search"},
					},
					map[string]any{
						"user_plan_id":    "p1",
						"plan_id":         "plan-b",
						"show_name":       "GLM-5-Turbo",
						"total_units":     10,
						"used_units":      10,
						"remaining_units": 0,
						"expires_at":      1790050000,
						"capabilities":    []any{"model:GLM-5-Turbo"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	// Point the balance read at the test server.
	old := OriginZCode
	defer func() { setOriginForTest(old) }()
	setOriginForTest(srv.URL)

	usage, errQuery := QueryStartPlan(context.Background(), srv.Client(), testJWT(t, 3600), "dev-1")
	if errQuery != nil {
		t.Fatalf("QueryStartPlan error: %v", errQuery)
	}
	if usage.Plan != "Start Plan" {
		t.Errorf("Plan = %q, want Start Plan", usage.Plan)
	}
	if len(usage.Windows) != 2 {
		t.Fatalf("windows = %d, want 2", len(usage.Windows))
	}
	first := usage.Windows[0]
	if first.Name != "GLM-5.2" {
		t.Errorf("window 0 name = %q", first.Name)
	}
	if first.UsedPercent != 25 {
		t.Errorf("window 0 used = %v, want 25", first.UsedPercent)
	}
	if !first.HasCount || first.Total != 100 {
		t.Errorf("window 0 count = %v/%v", first.Used, first.Total)
	}
	if len(first.Models) != 1 || first.Models[0] != "GLM-5.2" {
		t.Errorf("window 0 models = %v", first.Models)
	}
	if !usage.Windows[1].Spent {
		t.Errorf("an empty bucket should read as spent")
	}
	if usage.Windows[1].UsedPercent != 100 {
		t.Errorf("a spent bucket should read as fully used, got %v", usage.Windows[1].UsedPercent)
	}
}

func TestQueryStartPlan_ExpiredToken(t *testing.T) {
	if _, errQuery := QueryStartPlan(context.Background(), http.DefaultClient, testJWT(t, -60), "dev-1"); errQuery == nil {
		t.Fatal("QueryStartPlan accepted an expired token")
	}
}

func TestQueryQuota_CodingPlanWithGift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == QuotaLimitPath:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"level":  "lite",
					"limits": []any{map[string]any{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 20}},
				},
			})
		case strings.HasPrefix(r.URL.Path, BillingBalancePath):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"server_time": 1790000000,
					"plans":       []any{map[string]any{"user_plan_id": "p1", "name": "Start Plan", "status": "active", "ends_at": 1790100000}},
					"balances": []any{map[string]any{
						"user_plan_id": "p1", "show_name": "GLM-5.2", "total_units": 50, "remaining_units": 40,
						"expires_at": 1790050000, "capabilities": []any{"model:GLM-5.2"},
					}},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{}})
		}
	}))
	defer srv.Close()

	old := OriginZCode
	defer func() { setOriginForTest(old) }()
	setOriginForTest(srv.URL)

	auth := &cliproxyauth.Auth{
		Provider: "zcode",
		Attributes: map[string]string{
			"api_key":     "id.secret",
			"zcode_token": testJWT(t, 3600),
			"base_url":    srv.URL + "/api/anthropic",
			"device_id":   "dev-1",
		},
	}
	resp, errQuery := QueryQuota(context.Background(), srv.Client(), auth)
	if errQuery != nil {
		t.Fatalf("QueryQuota error: %v", errQuery)
	}
	if resp.Subscription == nil || resp.Subscription.Plan != "GLM Coding Lite" {
		t.Fatalf("plan = %+v", resp.Subscription)
	}
	// The coding plan's window and the gift's bucket are separate pools.
	if len(resp.Groups) != 2 {
		t.Fatalf("groups = %d, want 2: %+v", len(resp.Groups), resp.Groups)
	}
	if resp.Groups[0].DisplayName != "GLM Coding Lite" {
		t.Errorf("group 0 = %q", resp.Groups[0].DisplayName)
	}
	if resp.Groups[1].DisplayName != "ZCode Start Plan" {
		t.Errorf("group 1 = %q, want ZCode Start Plan", resp.Groups[1].DisplayName)
	}
	if got := resp.Groups[0].Buckets[0].RemainingFraction; got != 0.8 {
		t.Errorf("remaining fraction = %v, want 0.8", got)
	}
}

func TestQueryQuota_StartOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"server_time": 1790000000,
				"plans":       []any{map[string]any{"user_plan_id": "p1", "name": "Start Plan", "status": "active", "ends_at": 1790100000}},
				"balances": []any{map[string]any{
					"user_plan_id": "p1", "show_name": "GLM-5.2", "total_units": 50, "remaining_units": 25,
					"expires_at": 1790050000, "capabilities": []any{"model:GLM-5.2"},
				}},
			},
		})
	}))
	defer srv.Close()

	old := OriginZCode
	defer func() { setOriginForTest(old) }()
	setOriginForTest(srv.URL)

	auth := &cliproxyauth.Auth{
		Provider:   "zcode",
		Attributes: map[string]string{"zcode_token": testJWT(t, 3600), "device_id": "dev-1"},
	}
	resp, errQuery := QueryQuota(context.Background(), srv.Client(), auth)
	if errQuery != nil {
		t.Fatalf("QueryQuota error: %v", errQuery)
	}
	if len(resp.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(resp.Groups))
	}
	if resp.Groups[0].DisplayName != "Start Plan" {
		t.Errorf("group = %q, want Start Plan", resp.Groups[0].DisplayName)
	}
	if got := resp.Groups[0].Buckets[0].RemainingFraction; got != 0.5 {
		t.Errorf("remaining = %v, want 0.5", got)
	}
}

func TestWindowNameAndSpan(t *testing.T) {
	cases := []struct {
		unit  int
		count float64
		span  int
		name  string
	}{
		{3, 5, 5 * 3600, "5 hours"},
		{3, 0, 5 * 3600, "5 hours"},
		{6, 1, 7 * 86400, "Weekly"},
		{4, 1, 86400, "1 days"},
		{5, 1, 30 * 86400, "Monthly"},
		{0, 0, 0, "Credits"},
	}
	for _, tc := range cases {
		span := spanSeconds(tc.unit, tc.count)
		if span != tc.span {
			t.Errorf("spanSeconds(%d, %v) = %d, want %d", tc.unit, tc.count, span, tc.span)
		}
		if got := windowName(span); got != tc.name {
			t.Errorf("windowName(%d) = %q, want %q", span, got, tc.name)
		}
	}
}

func TestGiftSpent(t *testing.T) {
	if !giftSpent(mustResult(t, `{"remaining_units":0}`)) {
		t.Error("a zero remaining balance should read as spent")
	}
	if giftSpent(mustResult(t, `{"remaining_units":5}`)) {
		t.Error("a balance with units left should not read as spent")
	}
	if !giftSpent(mustResult(t, `{"total_units":10,"used_units":10}`)) {
		t.Error("an older balance at its total should read as spent")
	}
}

func TestJWTExpired(t *testing.T) {
	if JWTExpired(testJWT(t, 3600), timeNowForTest()) {
		t.Error("a live token read as expired")
	}
	if !JWTExpired(testJWT(t, -3600), timeNowForTest()) {
		t.Error("an expired token read as live")
	}
	if JWTExpired("not-a-jwt", timeNowForTest()) {
		t.Error("an unreadable token should not be treated as expired")
	}
}
