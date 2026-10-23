package api

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type rawCooldownRecord struct {
	Scope            string    `json:"scope"`
	ModelKey         string    `json:"model_key"`
	Reason           string    `json:"reason"`
	RetryAt          time.Time `json:"retry_at"`
	RemainingSeconds int64     `json:"remaining_seconds"`
	BackoffLevel     *int      `json:"backoff_level"`
	HTTPStatus       *int      `json:"http_status"`
}

// CooldownSummary holds normalized aggregate cooldown data.
type CooldownSummary struct {
	Scope            string   `json:"scope"`
	ScopeLabel       string   `json:"scope_label"`
	ModelCount       int      `json:"model_count"`
	Reason           string   `json:"reason"`
	ReasonLabel      string   `json:"reason_label"`
	RemainingSeconds int64    `json:"remaining_seconds"`
	RemainingText    string   `json:"remaining_text"`
	EarliestRetryAt  *int64   `json:"earliest_retry_at"`
	HTTPStatus       *int     `json:"http_status"`
	BackoffLevel     *int     `json:"backoff_level"`
	ObservedAt       int64    `json:"observed_at"`
	Reasons          []string `json:"reasons"`
	ModelKeys        []string `json:"model_keys"`
}

// AuthHealthInfo holds classified operational health for an upstream credential.
type AuthHealthInfo struct {
	State    string           `json:"state"`
	Label    string           `json:"label"`
	Detail   string           `json:"detail"`
	Blocked  bool             `json:"blocked"`
	Cooldown *CooldownSummary `json:"cooldown"`
}

func formatCooldownRemaining(seconds int64) string {
	if seconds <= 0 {
		return "计时已到期"
	}
	if seconds < 60 {
		return fmt.Sprintf("%d 秒", seconds)
	}
	minutes := (seconds + 59) / 60
	if minutes < 60 {
		return fmt.Sprintf("%d 分钟", minutes)
	}
	hours := minutes / 60
	restMinutes := minutes % 60
	if hours >= 24 {
		days := hours / 24
		return fmt.Sprintf("%d 天 %d 小时", days, hours%24)
	}
	if restMinutes > 0 {
		return fmt.Sprintf("%d 小时 %d 分钟", hours, restMinutes)
	}
	return fmt.Sprintf("%d 小时", hours)
}

func interpretCooldowns(rawList []rawCooldownRecord, observedAtMs int64, nowMs int64) *CooldownSummary {
	if len(rawList) == 0 {
		return nil
	}
	elapsedSec := (nowMs - observedAtMs) / 1000
	if elapsedSec < 0 {
		elapsedSec = 0
	}

	var active []rawCooldownRecord
	var activeRemaining []int64

	for _, r := range rawList {
		rem := r.RemainingSeconds - elapsedSec
		if rem > 0 {
			active = append(active, r)
			activeRemaining = append(activeRemaining, rem)
		}
	}
	if len(active) == 0 {
		return nil
	}

	modelCount := 0
	credentialWide := false
	modelKeyMap := make(map[string]bool)
	reasonMap := make(map[string]bool)

	var earliestRetryAt *int64
	var firstHTTPStatus *int
	var maxBackoffLevel *int
	minRemaining := activeRemaining[0]

	for i, r := range active {
		rem := activeRemaining[i]
		if rem < minRemaining {
			minRemaining = rem
		}
		if r.Scope == "credential" {
			credentialWide = true
		} else if r.Scope == "model" {
			modelCount++
			if r.ModelKey != "" {
				modelKeyMap[r.ModelKey] = true
			}
		}
		if r.Reason != "" {
			reasonMap[r.Reason] = true
		}
		if !r.RetryAt.IsZero() {
			ms := r.RetryAt.UnixMilli()
			if earliestRetryAt == nil || ms < *earliestRetryAt {
				earliestRetryAt = &ms
			}
		}
		if firstHTTPStatus == nil && r.HTTPStatus != nil {
			firstHTTPStatus = r.HTTPStatus
		}
		if r.BackoffLevel != nil {
			if maxBackoffLevel == nil || *r.BackoffLevel > *maxBackoffLevel {
				maxBackoffLevel = r.BackoffLevel
			}
		}
	}

	scope := "models"
	scopeLabel := fmt.Sprintf("%d 个模型冷却", modelCount)
	if credentialWide && modelCount > 0 {
		scope = "credential_and_models"
		scopeLabel = fmt.Sprintf("凭证及 %d 个模型冷却", modelCount)
	} else if credentialWide {
		scope = "credential"
		scopeLabel = "整个凭证"
	}

	severityRank := map[string]int{
		"quota":                1,
		"credential_quota":     2,
		"payment_required":     3,
		"cloudflare_challenge": 4,
		"invalid_grant":        5,
		"unauthorized":         6,
		"model_not_supported":  7,
		"not_found":            8,
		"transient_error":      9,
	}

	reasons := make([]string, 0, len(reasonMap))
	for k := range reasonMap {
		reasons = append(reasons, k)
	}
	sort.Slice(reasons, func(i, j int) bool {
		rI := severityRank[reasons[i]]
		if rI == 0 {
			rI = 99
		}
		rJ := severityRank[reasons[j]]
		if rJ == 0 {
			rJ = 99
		}
		return rI < rJ
	})
	primaryReason := "unknown"
	if len(reasons) > 0 {
		primaryReason = reasons[0]
	}

	reasonLabels := map[string]string{
		"quota":                "限流",
		"credential_quota":     "额度耗尽",
		"cloudflare_challenge": "Cloudflare 验证",
		"invalid_grant":        "授权无效",
		"unauthorized":         "未获授权",
		"payment_required":     "访问受限",
		"not_found":            "资源未找到",
		"model_not_supported":  "模型不受支持",
		"transient_error":      "上游暂时出错",
	}
	reasonLabel := reasonLabels[primaryReason]
	if reasonLabel == "" {
		reasonLabel = "其他或未知原因"
	}

	modelKeys := make([]string, 0, len(modelKeyMap))
	for k := range modelKeyMap {
		modelKeys = append(modelKeys, k)
	}
	sort.Strings(modelKeys)

	return &CooldownSummary{
		Scope:            scope,
		ScopeLabel:       scopeLabel,
		ModelCount:       modelCount,
		Reason:           primaryReason,
		ReasonLabel:      reasonLabel,
		RemainingSeconds: minRemaining,
		RemainingText:    formatCooldownRemaining(minRemaining),
		EarliestRetryAt:  earliestRetryAt,
		HTTPStatus:       firstHTTPStatus,
		BackoffLevel:     maxBackoffLevel,
		ObservedAt:       observedAtMs,
		Reasons:          reasons,
		ModelKeys:        modelKeys,
	}
}

func getAuthHealth(cooldown *CooldownSummary, disabled bool, unavailable bool, status string, statusMsg string) AuthHealthInfo {
	if cooldown != nil {
		return AuthHealthInfo{
			State:    "cooldown",
			Label:    "冷却中",
			Detail:   fmt.Sprintf("%s · %s · 最早结束约 %s", cooldown.ReasonLabel, cooldown.ScopeLabel, cooldown.RemainingText),
			Blocked:  true,
			Cooldown: cooldown,
		}
	}
	statusLower := strings.ToLower(strings.TrimSpace(status))
	msgLower := strings.ToLower(strings.TrimSpace(statusMsg))
	if strings.Contains(msgLower, "token expired") || strings.Contains(msgLower, "credential expired") || strings.Contains(msgLower, "credentials expired") || strings.Contains(msgLower, "令牌已过期") || strings.Contains(msgLower, "凭据已过期") || statusLower == "expired" {
		return AuthHealthInfo{
			State:   "expired",
			Label:   "Token 已过期",
			Detail:  "CPA 报告 token expired，请重新授权或刷新凭据",
			Blocked: true,
		}
	}
	if disabled || statusLower == "disabled" {
		return AuthHealthInfo{
			State:   "disabled",
			Label:   "已停用",
			Detail:  "CPA 已停用此凭据",
			Blocked: true,
		}
	}
	if statusLower == "error" || statusLower == "invalid" || statusLower == "failed" {
		return AuthHealthInfo{
			State:   "error",
			Label:   "凭据异常",
			Detail:  "CPA 报告凭据错误，请检查认证状态",
			Blocked: true,
		}
	}
	if unavailable || statusLower == "unavailable" {
		return AuthHealthInfo{
			State:   "unavailable",
			Label:   "暂不可用",
			Detail:  "CPA 当前标记此凭据不可用",
			Blocked: true,
		}
	}
	return AuthHealthInfo{
		State:   "ready",
		Label:   "可用",
		Detail:  "",
		Blocked: false,
	}
}

func resolveAuthAccountDisplayName(name, authIndex, email, label, nickname string) string {
	nick := strings.TrimSpace(nickname)
	if nick != "" {
		return nick
	}
	rawLabel := strings.TrimSpace(label)
	if rawLabel != "" {
		if idx := strings.LastIndex(rawLabel, "("); idx != -1 && strings.HasSuffix(rawLabel, ")") {
			inner := strings.TrimSpace(rawLabel[idx+1 : len(rawLabel)-1])
			innerLower := strings.ToLower(inner)
			if inner != "" && innerLower != "cn" && innerLower != "intl" && innerLower != "global" && innerLower != "default" && innerLower != "api" {
				return inner
			}
		}
		if !strings.EqualFold(rawLabel, name) {
			return rawLabel
		}
	}
	rawEmail := strings.TrimSpace(email)
	if rawEmail != "" && !strings.EqualFold(rawEmail, name) {
		return rawEmail
	}
	cleanName := strings.TrimSuffix(strings.TrimSpace(name), ".json")
	if cleanName != "" {
		return cleanName
	}
	return authIndex
}
