package redact

import (
	"strings"
)

// StreamRestorer wraps chunk-by-chunk stream processing, handling placeholders
// split across SSE chunk boundaries and token events.
type StreamRestorer struct {
	redactor                 *SessionRedactor
	pendingTokenPrefix       string
	pendingTargetPlaceholder string
	pendingSecret            string
}

// NewStreamRestorer creates a new stream restorer bound to a redactor.
func NewStreamRestorer(redactor *SessionRedactor) *StreamRestorer {
	return &StreamRestorer{
		redactor: redactor,
	}
}

// RestoreChunk processes an incoming stream chunk. It restores complete placeholders,
// and if a placeholder is split across chunks/events, it buffers the prefix and restores
// the secret as soon as the suffix arrives.
func (s *StreamRestorer) RestoreChunk(chunk []byte) []byte {
	if s.redactor == nil || !s.redactor.HasBindings() || len(chunk) == 0 {
		return chunk
	}

	text := string(chunk)

	// 1. If we are waiting for the suffix of a split placeholder
	if s.pendingTargetPlaceholder != "" {
		expectedSuffix := strings.TrimPrefix(s.pendingTargetPlaceholder, s.pendingTokenPrefix)
		if strings.Contains(text, expectedSuffix) {
			text = strings.Replace(text, expectedSuffix, s.pendingSecret, 1)
			s.pendingTokenPrefix = ""
			s.pendingTargetPlaceholder = ""
			s.pendingSecret = ""
		} else {
			// Check if text ends with a further prefix of the expected suffix
			for l := len(expectedSuffix) - 1; l >= 1; l-- {
				partial := expectedSuffix[:l]
				if strings.Contains(text, partial) {
					text = strings.Replace(text, partial, "", 1)
					s.pendingTokenPrefix += partial
					break
				}
			}
		}
	}

	// 2. Restore any complete placeholders in this chunk
	text = s.redactor.Restore(text)

	// 3. Check if there is an unclosed placeholder prefix starting with "{{" in this chunk
	lastOpen := strings.LastIndex(text, "{{")
	if lastOpen != -1 {
		candTail := text[lastOpen:]
		// If it already has "}}", then Restore() would have handled it or it's not a placeholder
		if !strings.Contains(candTail, "}}") {
			// Extract potential prefix, ignoring trailing JSON framing like quotes, braces, newlines
			candPrefix := candTail
			for _, endChar := range []string{"\"", "\\", "\n", "\r", "}"} {
				if endIdx := strings.Index(candPrefix, endChar); endIdx != -1 {
					candPrefix = candPrefix[:endIdx]
				}
			}
			if len(candPrefix) >= 2 {
				if ph, sec, ok := s.redactor.MatchPrefix(candPrefix); ok {
					// Remove the partial placeholder from text so client doesn't see raw prefix
					text = text[:lastOpen] + text[lastOpen+len(candPrefix):]
					s.pendingTokenPrefix = candPrefix
					s.pendingTargetPlaceholder = ph
					s.pendingSecret = sec
				}
			}
		}
	}

	return []byte(text)
}

// Finish flushes any trailing pending bytes upon stream completion.
func (s *StreamRestorer) Finish() []byte {
	if s.pendingTokenPrefix != "" {
		flushed := s.pendingTokenPrefix
		s.pendingTokenPrefix = ""
		s.pendingTargetPlaceholder = ""
		s.pendingSecret = ""
		return []byte(flushed)
	}
	return nil
}
