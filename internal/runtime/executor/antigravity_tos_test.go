package executor

import (
	"testing"
)

// antigravityTOSBody reproduces the verbatim 403 PERMISSION_DENIED signature
// Google returns for a service-scoped Terms-of-Service suspension
// (upstream issue #7515): an ErrorInfo reason=TOS_VIOLATION plus appeal
// metadata.
const antigravityTOSBody = `{
	"error": {
		"code": 403,
		"message": "This service has been disabled in this account for violation of Terms of Service. Please submit an appeal to continue using this product.",
		"status": "PERMISSION_DENIED",
		"details": [
			{
				"@type": "type.googleapis.com/google.rpc.ErrorInfo",
				"reason": "TOS_VIOLATION",
				"domain": "cloudcode-pa.googleapis.com",
				"metadata": {
					"uiMessage": "true",
					"appeal_url": "https://forms.gle/hGzM9MEUv2azZsrb9",
					"appeal_url_link_text": "Submit Appeal"
				}
			}
		]
	}
}`

func TestParseAntigravityTOSViolation(t *testing.T) {
	t.Run("matches the verbatim tos signature and extracts the appeal url", func(t *testing.T) {
		violation := parseAntigravityTOSViolation([]byte(antigravityTOSBody))
		if violation == nil {
			t.Fatal("parseAntigravityTOSViolation() = nil, want a TOS violation match")
		}
		if violation.AppealURL != "https://forms.gle/hGzM9MEUv2azZsrb9" {
			t.Fatalf("AppealURL = %q, want %q", violation.AppealURL, "https://forms.gle/hGzM9MEUv2azZsrb9")
		}
		if violation.AppealURLText != "Submit Appeal" {
			t.Fatalf("AppealURLText = %q, want %q", violation.AppealURLText, "Submit Appeal")
		}
	})

	t.Run("ignores a plain 403 permission denied without a tos reason", func(t *testing.T) {
		body := `{"error":{"code":403,"status":"PERMISSION_DENIED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"SOME_OTHER_REASON"}]}}`
		if violation := parseAntigravityTOSViolation([]byte(body)); violation != nil {
			t.Fatalf("parseAntigravityTOSViolation() = %+v, want nil for a non-TOS 403", violation)
		}
	})

	t.Run("ignores a 429 resource exhausted", func(t *testing.T) {
		body := `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"RATE_LIMIT_EXCEEDED"}]}}`
		if violation := parseAntigravityTOSViolation([]byte(body)); violation != nil {
			t.Fatalf("parseAntigravityTOSViolation() = %+v, want nil for a 429", violation)
		}
	})

	t.Run("ignores an empty or malformed body", func(t *testing.T) {
		if violation := parseAntigravityTOSViolation(nil); violation != nil {
			t.Fatalf("parseAntigravityTOSViolation(nil) = %+v, want nil", violation)
		}
		if violation := parseAntigravityTOSViolation([]byte("not json")); violation != nil {
			t.Fatalf("parseAntigravityTOSViolation(invalid) = %+v, want nil", violation)
		}
	})
}

func TestNewAntigravityStatusErrMarksTOSViolation(t *testing.T) {
	t.Run("marks a tos 403 as credential scoped", func(t *testing.T) {
		err := newAntigravityStatusErr(403, []byte(antigravityTOSBody))
		if !err.credentialScoped {
			t.Fatal("newAntigravityStatusErr(TOS).credentialScoped = false, want true")
		}
		if !err.IsTerminalTOSViolation() {
			t.Fatal("newAntigravityStatusErr(TOS).IsTerminalTOSViolation() = false, want true")
		}
		metadata := err.TOSAppealMetadata()
		if metadata["tos_appeal_url"] != "https://forms.gle/hGzM9MEUv2azZsrb9" {
			t.Fatalf("tos_appeal_url = %q, want the appeal form url", metadata["tos_appeal_url"])
		}
		if metadata["tos_appeal_url_text"] != "Submit Appeal" {
			t.Fatalf("tos_appeal_url_text = %q, want %q", metadata["tos_appeal_url_text"], "Submit Appeal")
		}
	})

	t.Run("leaves a plain 403 untouched", func(t *testing.T) {
		err := newAntigravityStatusErr(403, []byte(`{"error":{"code":403,"status":"PERMISSION_DENIED"}}`))
		if err.IsTerminalTOSViolation() {
			t.Fatal("newAntigravityStatusErr(plain 403).IsTerminalTOSViolation() = true, want false")
		}
		if err.credentialScoped {
			t.Fatal("newAntigravityStatusErr(plain 403).credentialScoped = true, want false")
		}
		if metadata := err.TOSAppealMetadata(); metadata != nil {
			t.Fatalf("newAntigravityStatusErr(plain 403).TOSAppealMetadata() = %+v, want nil", metadata)
		}
	})

	t.Run("leaves a 429 untouched", func(t *testing.T) {
		err := newAntigravityStatusErr(429, []byte(`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED"}}`))
		if err.IsTerminalTOSViolation() {
			t.Fatal("newAntigravityStatusErr(429).IsTerminalTOSViolation() = true, want false")
		}
	})
}
