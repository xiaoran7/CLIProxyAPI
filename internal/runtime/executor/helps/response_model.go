package helps

import (
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	"github.com/tidwall/gjson"
)

const (
	// maxCodexResponseModelLength is a defensive bound on an upstream-controlled string
	// reaching logs and usage records; known codex model ids stay under ~30 bytes.
	maxCodexResponseModelLength = 128

	// codexModelSubstitutionWarnWindow bounds how often one credential and model pair
	// warns: on an affected credential every request is substituted.
	codexModelSubstitutionWarnWindow = 10 * time.Minute

	// codexModelSubstitutionWarnMaxEntries caps the throttle state, naturally bounded by
	// credentials times models; memory safety wins over perfect throttling.
	codexModelSubstitutionWarnMaxEntries = 1024
)

// extractCodexResponseModelEvent returns the model a codex upstream reports serving, read
// from a raw JSON frame or an SSE line, and whether the event terminates the response.
func extractCodexResponseModelEvent(payload []byte) (model string, terminal bool) {
	data := jsonPayload(payload)
	if len(data) == 0 {
		return "", false
	}
	// The event type is checked before the payload is validated, so the hot path
	// (output deltas) stays a single cheap lookup.
	carriesModel, terminal := codexResponseModelEventKind(gjson.GetBytes(data, "type").String())
	if !carriesModel {
		return "", false
	}
	if !gjson.ValidBytes(data) {
		return "", false
	}
	// The value is upstream-controlled: reject non-string and oversized names
	// rather than propagating them into logs and usage records.
	modelResult := gjson.GetBytes(data, "response.model")
	if modelResult.Type != gjson.String {
		return "", terminal
	}
	model = strings.TrimSpace(modelResult.String())
	if len(model) > maxCodexResponseModelLength {
		return "", terminal
	}
	return model, terminal
}

// codexResponseModelEventKind reports whether a codex event embeds the authoritative
// response object, and whether that event terminates the response.
func codexResponseModelEventKind(eventType string) (carriesModel bool, terminal bool) {
	switch strings.TrimSpace(eventType) {
	case "response.created", "response.in_progress":
		return true, false
	case "response.completed", "response.incomplete", "response.done":
		return true, true
	default:
		return false, false
	}
}

// normalizeCodexModelName lower-cases a model id and drops its thinking suffix,
// which never reaches the upstream request body.
func normalizeCodexModelName(model string) string {
	return strings.TrimSpace(thinking.ParseSuffix(strings.ToLower(strings.TrimSpace(model))).ModelName)
}

// IsCodexModelSubstituted reports whether the upstream served a model other than the
// requested one; a dated alias pins a snapshot of the same model and is accepted.
func IsCodexModelSubstituted(requested, served string) bool {
	servedModel := normalizeCodexModelName(served)
	if servedModel == "" {
		return false
	}
	requestedModel := normalizeCodexModelName(requested)
	if requestedModel == "" {
		return false
	}
	if requestedModel == servedModel {
		return false
	}
	return !isCodexDatedModelAlias(requestedModel, servedModel) &&
		!isCodexDatedModelAlias(servedModel, requestedModel)
}

// isCodexDatedModelAlias reports whether dated is base plus a release date suffix,
// which upstreams use to pin the exact snapshot of the same model.
func isCodexDatedModelAlias(base, dated string) bool {
	prefix := base + "-"
	if !strings.HasPrefix(dated, prefix) {
		return false
	}
	return isCodexModelDateSuffix(dated[len(prefix):])
}

// isCodexModelDateSuffix reports whether suffix is a YYYY-MM-DD or YYYYMMDD date.
func isCodexModelDateSuffix(suffix string) bool {
	switch len(suffix) {
	case len("YYYY-MM-DD"):
		if suffix[4] != '-' || suffix[7] != '-' {
			return false
		}
		return isCodexModelDigits(suffix[:4]) && isCodexModelDigits(suffix[5:7]) && isCodexModelDigits(suffix[8:])
	case len("YYYYMMDD"):
		return isCodexModelDigits(suffix)
	default:
		return false
	}
}

// isCodexModelDigits reports whether value is a non-empty run of ASCII digits.
func isCodexModelDigits(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

type codexModelSubstitutionKey struct {
	authID    string
	requested string
	served    string
}

// codexModelSubstitutionThrottle records the last warning per key; nowFunc is
// injectable so tests can advance the window without sleeping.
type codexModelSubstitutionThrottle struct {
	mu       sync.Mutex
	nowFunc  func() time.Time
	lastWarn map[codexModelSubstitutionKey]time.Time
}

func newCodexModelSubstitutionThrottle(nowFunc func() time.Time) *codexModelSubstitutionThrottle {
	if nowFunc == nil {
		nowFunc = time.Now
	}
	return &codexModelSubstitutionThrottle{
		nowFunc:  nowFunc,
		lastWarn: make(map[codexModelSubstitutionKey]time.Time),
	}
}

// allow reports whether the key may emit a warning now and records the decision.
// Suppressed repeats stay silent instead of moving to a lower level.
func (t *codexModelSubstitutionThrottle) allow(key codexModelSubstitutionKey) bool {
	if t == nil {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.nowFunc()
	if last, ok := t.lastWarn[key]; ok && now.Sub(last) < codexModelSubstitutionWarnWindow {
		return false
	}
	if len(t.lastWarn) >= codexModelSubstitutionWarnMaxEntries {
		for storedKey, storedAt := range t.lastWarn {
			if now.Sub(storedAt) >= codexModelSubstitutionWarnWindow {
				delete(t.lastWarn, storedKey)
			}
		}
		if len(t.lastWarn) >= codexModelSubstitutionWarnMaxEntries {
			clear(t.lastWarn)
		}
	}
	t.lastWarn[key] = now
	return true
}

// codexModelSubstitutionWarns throttles substitution warnings process-wide.
var codexModelSubstitutionWarns = newCodexModelSubstitutionThrottle(time.Now)
