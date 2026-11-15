package auth

import (
	"fmt"
	"testing"
)

// fakeTOSError mimics the executor error that carries a terminal Antigravity
// TOS suspension through the conductor.
type fakeTOSError struct {
	metadata map[string]string
}

func (e fakeTOSError) Error() string { return "tos violation" }

func (e fakeTOSError) IsTerminalTOSViolation() bool { return true }

func (e fakeTOSError) TOSAppealMetadata() map[string]string { return e.metadata }

func TestIsTOSViolationResultError(t *testing.T) {
	if isTOSViolationResultError(nil) {
		t.Fatal("isTOSViolationResultError(nil) = true, want false")
	}
	if !isTOSViolationResultError(&Error{Code: tosViolationErrorCode}) {
		t.Fatalf("isTOSViolationResultError(%q) = false, want true", tosViolationErrorCode)
	}
	if isTOSViolationResultError(&Error{Code: "quota"}) {
		t.Fatal("isTOSViolationResultError(quota) = true, want false")
	}
}

func TestIsTerminalTOSViolationError(t *testing.T) {
	if isTerminalTOSViolationError(nil) {
		t.Fatal("isTerminalTOSViolationError(nil) = true, want false")
	}
	if !isTerminalTOSViolationError(fakeTOSError{}) {
		t.Fatal("isTerminalTOSViolationError(fake) = false, want true")
	}
	if !isTerminalTOSViolationError(fmt.Errorf("wrapped: %w", fakeTOSError{})) {
		t.Fatal("isTerminalTOSViolationError(wrapped) = false, want true")
	}
	if isTerminalTOSViolationError(fmt.Errorf("plain")) {
		t.Fatal("isTerminalTOSViolationError(plain) = true, want false")
	}
}

func TestTOSAppealMetadataFromError(t *testing.T) {
	metadata := tosAppealMetadataFromError(fakeTOSError{metadata: map[string]string{
		"tos_appeal_url":      "https://forms.gle/example",
		"tos_appeal_url_text": "Submit Appeal",
	}})
	if metadata["tos_appeal_url"] != "https://forms.gle/example" {
		t.Fatalf("tos_appeal_url = %q, want the appeal form url", metadata["tos_appeal_url"])
	}
	if tosAppealMetadataFromError(fmt.Errorf("plain")) != nil {
		t.Fatal("tosAppealMetadataFromError(plain) != nil, want nil")
	}
}

func TestResultErrorFromErrorClassifiesTOSViolation(t *testing.T) {
	err := resultErrorFromError(fakeTOSError{metadata: map[string]string{
		"tos_appeal_url":      "https://forms.gle/example",
		"tos_appeal_url_text": "Submit Appeal",
	}})
	if err == nil {
		t.Fatal("resultErrorFromError() = nil, want an error")
	}
	if err.Code != tosViolationErrorCode {
		t.Fatalf("Code = %q, want %q", err.Code, tosViolationErrorCode)
	}
	if err.Metadata["tos_appeal_url"] != "https://forms.gle/example" {
		t.Fatalf("Metadata[tos_appeal_url] = %q, want the appeal form url", err.Metadata["tos_appeal_url"])
	}
}

func TestPersistTOSAppealMetadata(t *testing.T) {
	auth := &Auth{}
	persistTOSAppealMetadata(auth, &Error{Code: tosViolationErrorCode, Metadata: map[string]string{
		"tos_appeal_url":      "https://forms.gle/example",
		"tos_appeal_url_text": "Submit Appeal",
		"blank":               "   ",
	}})
	if auth.Metadata["tos_appeal_url"] != "https://forms.gle/example" {
		t.Fatalf("Metadata[tos_appeal_url] = %v, want the appeal form url", auth.Metadata["tos_appeal_url"])
	}
	if auth.Metadata["tos_appeal_url_text"] != "Submit Appeal" {
		t.Fatalf("Metadata[tos_appeal_url_text] = %v, want %q", auth.Metadata["tos_appeal_url_text"], "Submit Appeal")
	}
	if _, ok := auth.Metadata["blank"]; ok {
		t.Fatal("Metadata[blank] was persisted, want blank values dropped")
	}
}

func TestErrorClonePreservesMetadata(t *testing.T) {
	original := &Error{
		Code:       tosViolationErrorCode,
		Message:    "tos",
		HTTPStatus: 403,
		Metadata:   map[string]string{"tos_appeal_url": "https://forms.gle/example"},
	}
	clone := original.Clone()
	if clone.Metadata["tos_appeal_url"] != "https://forms.gle/example" {
		t.Fatalf("clone.Metadata[tos_appeal_url] = %q, want the appeal form url", clone.Metadata["tos_appeal_url"])
	}
	clone.Metadata["tos_appeal_url"] = "mutated"
	if original.Metadata["tos_appeal_url"] == "mutated" {
		t.Fatal("clone shares the Metadata map with the original, want an independent copy")
	}
}
