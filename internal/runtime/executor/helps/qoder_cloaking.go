// Package helps provides shared utilities and helpers for runtime executors.
package helps

import (
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// StripQoderForbiddenHeaders removes client billing, internal proxy, and tracking headers
// that could trigger upstream WAF or gateway rejection.
func StripQoderForbiddenHeaders(req *http.Request) {
	if req == nil {
		return
	}
	req.Header.Del("x-anthropic-billing-header")
	req.Header.Del("anthropic-version")
	req.Header.Del("x-client")
	req.Header.Del("x-app-name")
	req.Header.Del("x-app-version")
	req.Header.Del("x-zcode-app-version")
	req.Header.Del("x-zcode-agent")
	req.Header.Del("x-zcode-session-type")
	req.Header.Del("x-zcode-trace-id")
	req.Header.Del("x-machine-id")
	req.Header.Del("x-session-id")
}

// SanitizeQoderPayload normalizes model IDs (e.g. stripping -free suffixes)
// to ensure compliance with upstream Qoder gateway routing.
func SanitizeQoderPayload(payload []byte) []byte {
	if len(payload) == 0 {
		return payload
	}
	if modelRes := gjson.GetBytes(payload, "model"); modelRes.Exists() {
		m := strings.TrimSpace(modelRes.String())
		if strings.HasSuffix(strings.ToLower(m), "-free") {
			cleanModel := m[:len(m)-len("-free")]
			if updated, err := sjson.SetBytes(payload, "model", cleanModel); err == nil {
				payload = updated
			}
		}
	}
	return payload
}
