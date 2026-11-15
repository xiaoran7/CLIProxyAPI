package executor

import (
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// antigravityTOSViolation captures the parsed state of a 403 PERMISSION_DENIED
// carrying an ErrorInfo reason=TOS_VIOLATION (upstream issue #7515).
//
// The suspension is service-scoped: the OAuth token and the model catalog keep
// working while inference is refused. A cooldown therefore cannot express it,
// because a cooldown expires and hands the dead account back to the rotation;
// the credential has to be deactivated instead.
type antigravityTOSViolation struct {
	AppealURL     string
	AppealURLText string
}

// parseAntigravityTOSViolation extracts the TOS_VIOLATION signature from a
// Google RPC error body. It returns nil for every other shape, including
// transient 429 quota errors and 403s that carry a different ErrorInfo reason.
func parseAntigravityTOSViolation(body []byte) *antigravityTOSViolation {
	if len(body) == 0 {
		return nil
	}
	if code := int(gjson.GetBytes(body, "error.code").Int()); code != http.StatusForbidden {
		return nil
	}
	if status := strings.TrimSpace(gjson.GetBytes(body, "error.status").String()); !strings.EqualFold(status, "PERMISSION_DENIED") {
		return nil
	}
	details := gjson.GetBytes(body, "error.details")
	if !details.Exists() || !details.IsArray() {
		return nil
	}
	for _, detail := range details.Array() {
		if detail.Get("@type").String() != "type.googleapis.com/google.rpc.ErrorInfo" {
			continue
		}
		if reason := strings.TrimSpace(detail.Get("reason").String()); !strings.EqualFold(reason, "TOS_VIOLATION") {
			continue
		}
		return &antigravityTOSViolation{
			AppealURL:     strings.TrimSpace(detail.Get("metadata.appeal_url").String()),
			AppealURLText: strings.TrimSpace(detail.Get("metadata.appeal_url_link_text").String()),
		}
	}
	return nil
}
