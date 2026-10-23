package api

import (
	"fmt"
	"strings"
	"time"
)

const (
	defaultAuthPriority = 0
	defaultAuthWeight   = 100
)

var cooldownReasonLabels = map[string]string{
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

func computeHealth(file CpaAuthFile) UpstreamAccountHealth {
	var activeCooldowns []CpaCooldownRecord
	for _, cd := range file.Cooldowns {
		if cd.RemainingSeconds > 0 {
			activeCooldowns = append(activeCooldowns, cd)
		}
	}

	if len(activeCooldowns) > 0 {
		modelCount := 0
		hasCredentialScope := false
		var reasons []string
		var modelKeys []string
		minRemaining := activeCooldowns[0].RemainingSeconds
		primaryReason := activeCooldowns[0].Reason

		for _, cd := range activeCooldowns {
			if cd.Scope == "credential" {
				hasCredentialScope = true
			} else if cd.Scope == "model" {
				modelCount++
				if cd.ModelKey != "" {
					modelKeys = append(modelKeys, cd.ModelKey)
				}
			}
			if cd.RemainingSeconds < minRemaining {
				minRemaining = cd.RemainingSeconds
			}
			reasons = append(reasons, cd.Reason)
		}

		scopeStr := "models"
		scopeLabel := fmt.Sprintf("%d 个模型冷却", modelCount)
		if hasCredentialScope {
			if modelCount > 0 {
				scopeStr = "credential_and_models"
				scopeLabel = fmt.Sprintf("凭证及 %d 个模型冷却", modelCount)
			} else {
				scopeStr = "credential"
				scopeLabel = "整个凭证"
			}
		}

		reasonLabel := cooldownReasonLabels[primaryReason]
		if reasonLabel == "" {
			reasonLabel = "冷却限制"
		}

		remText := formatCooldownRemaining(int64(minRemaining))
		detail := fmt.Sprintf("%s · %s · 最早结束约 %s", reasonLabel, scopeLabel, remText)

		cdSummary := &UpstreamCooldownSummary{
			Scope:            scopeStr,
			ScopeLabel:       scopeLabel,
			ModelCount:       modelCount,
			Reason:           primaryReason,
			ReasonLabel:      reasonLabel,
			RemainingSeconds: minRemaining,
			RemainingText:    remText,
			ObservedAt:       time.Now().UnixMilli(),
			Reasons:          reasons,
			ModelKeys:        modelKeys,
		}

		return UpstreamAccountHealth{
			State:    "cooldown",
			Label:    "冷却中",
			Detail:   detail,
			Blocked:  hasCredentialScope,
			Cooldown: cdSummary,
		}
	}

	statusLower := strings.ToLower(strings.TrimSpace(file.Status))
	msgLower := strings.ToLower(strings.TrimSpace(file.StatusMessage))

	if strings.Contains(statusLower, "expired") || strings.Contains(msgLower, "expired") || strings.Contains(msgLower, "过期") {
		return UpstreamAccountHealth{
			State:   "expired",
			Label:   "Token 已过期",
			Detail:  "CPA 报告 token expired，请重新授权或刷新凭据",
			Blocked: true,
		}
	}

	if file.Disabled || statusLower == "disabled" {
		return UpstreamAccountHealth{
			State:   "disabled",
			Label:   "已停用",
			Detail:  "CPA 已停用此凭据",
			Blocked: true,
		}
	}

	if statusLower == "error" || statusLower == "invalid" || statusLower == "failed" {
		return UpstreamAccountHealth{
			State:   "error",
			Label:   "凭据异常",
			Detail:  "CPA 报告凭据错误，请检查认证状态",
			Blocked: true,
		}
	}

	if file.Unavailable || statusLower == "unavailable" {
		return UpstreamAccountHealth{
			State:   "unavailable",
			Label:   "暂不可用",
			Detail:  "CPA 当前标记此凭据不可用",
			Blocked: true,
		}
	}

	return UpstreamAccountHealth{
		State:   "ready",
		Label:   "可用",
		Detail:  "",
		Blocked: false,
	}
}

func isGenericIdentifier(val, provider string) bool {
	v := strings.ToLower(strings.TrimSpace(val))
	if v == "" {
		return true
	}
	p := strings.ToLower(strings.TrimSpace(provider))
	return v == p || v == "default" || v == "account" || v == "user" || v == "api" || v == "auth"
}

func resolveDisplayName(file CpaAuthFile) string {
	nick := strings.TrimSpace(file.Nickname)
	if nick == "" {
		nick = strings.TrimSpace(file.NickName)
	}
	if nick != "" {
		return nick
	}

	rawLabel := strings.TrimSpace(file.Label)
	if rawLabel != "" && strings.Contains(rawLabel, "(") && strings.HasSuffix(rawLabel, ")") {
		start := strings.LastIndex(rawLabel, "(")
		end := strings.LastIndex(rawLabel, ")")
		if start >= 0 && end > start {
			extracted := strings.TrimSpace(rawLabel[start+1 : end])
			extLower := strings.ToLower(extracted)
			if extLower != "cn" && extLower != "intl" && extLower != "global" && extLower != "default" && extLower != "api" {
				return extracted
			}
		}
	}

	email := strings.TrimSpace(file.Email)
	cleanName := strings.TrimSuffix(strings.TrimSpace(file.Name), ".json")

	if email != "" && !isGenericIdentifier(email, file.Provider) && email != cleanName {
		return email
	}

	if rawLabel != "" && !isGenericIdentifier(rawLabel, file.Provider) && rawLabel != cleanName {
		return rawLabel
	}

	if cleanName != "" {
		return cleanName
	}

	return file.AuthIndex
}
