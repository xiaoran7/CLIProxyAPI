package auth

import (
	"errors"
	"strings"
)

// tosViolationErrorCode marks a terminal Antigravity Terms-of-Service
// suspension on a result error.
//
// Upstream issue #7515: Google answers a service-scoped TOS suspension with
// 403 PERMISSION_DENIED and an ErrorInfo reason=TOS_VIOLATION. The account is
// suspended for inference but its OAuth token and model catalog keep working,
// so the credential must be deactivated rather than cooled down.
const tosViolationErrorCode = "tos_violation"

// isTOSViolationResultError reports whether the failure is a terminal
// Antigravity Terms-of-Service suspension.
func isTOSViolationResultError(err *Error) bool {
	if err == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(err.Code), tosViolationErrorCode)
}

// terminalTOSViolationProvider is implemented by executor errors that carry a
// terminal TOS suspension.
type terminalTOSViolationProvider interface {
	IsTerminalTOSViolation() bool
}

// isTerminalTOSViolationError reports whether the execution error carries a
// terminal TOS suspension. It probes the error chain so a wrapped executor
// error is still recognised.
func isTerminalTOSViolationError(err error) bool {
	if err == nil {
		return false
	}
	var provider terminalTOSViolationProvider
	return errors.As(err, &provider) && provider != nil && provider.IsTerminalTOSViolation()
}

// tosAppealMetadataProvider is implemented by executor errors that carry the
// operator-facing appeal detail of a TOS suspension.
type tosAppealMetadataProvider interface {
	TOSAppealMetadata() map[string]string
}

// tosAppealMetadataFromError extracts the appeal detail carried by an executor
// error, if any.
func tosAppealMetadataFromError(err error) map[string]string {
	if err == nil {
		return nil
	}
	var provider tosAppealMetadataProvider
	if errors.As(err, &provider) && provider != nil {
		return provider.TOSAppealMetadata()
	}
	return nil
}

// persistTOSAppealMetadata copies the appeal detail carried on a TOS_VIOLATION
// result error onto the credential record so operator tooling can surface the
// appeal form for the suspended account.
func persistTOSAppealMetadata(auth *Auth, err *Error) {
	if auth == nil || err == nil || len(err.Metadata) == 0 {
		return
	}
	if auth.Metadata == nil {
		auth.Metadata = make(map[string]any)
	}
	for key, value := range err.Metadata {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			auth.Metadata[key] = trimmed
		}
	}
}
