package zcode

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

var (
	// errNoCredential reports an auth with neither a coding plan key nor a Start Plan token.
	errNoCredential = errors.New("zcode: the credential holds no coding plan key; sign in again")
	// errNoStartToken reports a trial entry on an auth without ZCode's session token.
	errNoStartToken = errors.New("zcode: this credential has no ZCode session token, so it cannot spend the Start Plan")
	// errStartTokenExpired reports a Start Plan token past its expiry. ZCode's
	// session token cannot be refreshed; the account must sign in again.
	errStartTokenExpired = errors.New("zcode: ZCode's sign-in has expired; sign in to ZCode again")
)

// RefusalKind classifies a Start Plan answer that turned a request away.
type RefusalKind int

const (
	// RefusalNone reports that the answer is not a Start Plan refusal.
	RefusalNone RefusalKind = iota
	// RefusalBlocked reports zcode.z.ai's anti-bot refusal: a 405, an edge
	// block page, or error code 3012.
	RefusalBlocked
	// RefusalQuota reports the gift saying it has nothing left to spend.
	RefusalQuota
)

// StartBlockedHint explains a Start Plan refusal, mirroring the reference
// implementation's wording so operators can act on it.
const StartBlockedHint = "ZCode's Start Plan still turned this request away, though it is sent the way the ZCode app sends it; it can be a network block of this IP, or ZCode checking for something new. Use an account with a GLM Coding Plan, or add another provider to this group"

// giftSpentCodes are the JSON codes a spent gift answers with, even inside HTTP 200.
var giftSpentCodes = map[string]struct{}{
	"1113": {},
	"1005": {},
}

// ClassifyRefusal reads a Start Plan answer and reports what it means.
//
// blocked is zcode.z.ai's anti-bot refusal (HTTP 405, an edge block page, or
// code 3012). quota is the gift saying it is spent: any 429, or the 1113 /
// 1005 codes at either the top level or under error.code.
func ClassifyRefusal(status int, body []byte) RefusalKind {
	if status == 429 {
		return RefusalQuota
	}
	if status >= 400 && (status == 405 || isEdgeBlocked(body) || isStartRefused(body)) {
		return RefusalBlocked
	}
	if isGiftSpent(status, body) {
		return RefusalQuota
	}
	return RefusalNone
}

// isEdgeBlocked reports an edge block page, whose words read as a firewall's.
func isEdgeBlocked(body []byte) bool {
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "request has been blocked") || strings.Contains(lower, "errors.aliyun.com")
}

// isStartRefused reports ZCode's own anti-bot code for an unusual request.
func isStartRefused(body []byte) bool {
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "unusual activity") {
		return true
	}
	code := strings.TrimSpace(gjson.GetBytes(body, "code").String())
	if code == "" {
		code = strings.TrimSpace(gjson.GetBytes(body, "error.code").String())
	}
	return code == "3012"
}

// isGiftSpent reports the gift saying it has nothing left to spend, including
// the codes a spent gift returns inside an HTTP 200.
func isGiftSpent(status int, body []byte) bool {
	if status == 429 {
		return true
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	for _, path := range []string{"code", "error.code"} {
		if _, ok := giftSpentCodes[strings.TrimSpace(gjson.GetBytes(body, path).String())]; ok {
			return true
		}
	}
	return false
}

// RefusalMessage renders the message a Start Plan refusal should carry.
func RefusalMessage(kind RefusalKind, upstream string) string {
	switch kind {
	case RefusalBlocked:
		if upstream == "" {
			return StartBlockedHint
		}
		return upstream + " — " + StartBlockedHint
	case RefusalQuota:
		if upstream == "" {
			return "ZCode's Start Plan has no quota left for this model"
		}
		return upstream
	default:
		return upstream
	}
}

// TrialExhaustedMessage is the answer a trial entry gives when the gift cannot
// serve it, matching what the reference client returns to the agent.
func TrialExhaustedMessage(model string, known bool) string {
	if known {
		return fmt.Sprintf("ZCode's limited quota for %s is exhausted or temporarily unavailable", model)
	}
	return fmt.Sprintf("ZCode has no active limited quota for %s", model)
}
