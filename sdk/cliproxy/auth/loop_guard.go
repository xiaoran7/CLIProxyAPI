package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	loopWindow   = 256
	loopDistinct = 24
	unitMost     = 4096
	pendingMost  = 1 << 20
)

// LoopTrip records which channel looped and the repeating patterns found.
type LoopTrip struct {
	Reasoning bool
	Units     []string
	Window    int
}

func (t LoopTrip) Message() string {
	what := "text"
	if t.Reasoning {
		what = "reasoning"
	}
	quoted := make([]string, 0, 6)
	for i, u := range t.Units {
		if i == 5 {
			quoted = append(quoted, "…")
			break
		}
		quoted = append(quoted, fmt.Sprintf("%q", u))
	}
	return fmt.Sprintf("stream loop guard aborted reply: model %s was stuck in a repetitive loop (distinct units: %d in window %d, e.g. %s)",
		what, len(t.Units), t.Window, strings.Join(quoted, ", "))
}

type unitCount struct {
	n    int
	text string
	at   int
}

type unitRun struct {
	cur      strings.Builder
	stop     bool
	lettered bool
	ring     [loopWindow]uint64
	n        int
	counts   map[uint64]*unitCount
}

func unitKey(raw string) (string, bool) {
	hasLetter := false
	for _, r := range raw {
		if unicode.IsLetter(r) {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return "", false
	}
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.TrimRight(s, ".!?。！？ \t\r\n")
	return s, s != ""
}

func (r *unitRun) end() (LoopTrip, bool) {
	raw := r.cur.String()
	r.cur.Reset()
	r.stop, r.lettered = false, false

	key, ok := unitKey(raw)
	if !ok {
		return LoopTrip{}, false
	}
	h := fnv.New64a()
	h.Write([]byte(key))
	sum := h.Sum64()
	if r.counts == nil {
		r.counts = make(map[uint64]*unitCount)
	}
	slot := r.n % loopWindow
	if r.n >= loopWindow {
		old := r.ring[slot]
		if c := r.counts[old]; c != nil {
			c.n--
			if c.n <= 0 {
				delete(r.counts, old)
			}
		}
	}
	r.ring[slot] = sum
	c := r.counts[sum]
	if c == nil {
		text := strings.TrimSpace(raw)
		if utf8.RuneCountInString(text) > 40 {
			text = string([]rune(text)[:40]) + "…"
		}
		c = &unitCount{text: text, at: r.n}
		r.counts[sum] = c
	}
	c.n++
	r.n++
	if r.n < loopWindow || len(r.counts) > loopDistinct {
		return LoopTrip{}, false
	}

	all := make([]*unitCount, 0, len(r.counts))
	for _, cnt := range r.counts {
		all = append(all, cnt)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].n != all[j].n {
			return all[i].n > all[j].n
		}
		return all[i].at < all[j].at
	})
	units := make([]string, len(all))
	for i, u := range all {
		units[i] = u.text
	}
	return LoopTrip{Units: units, Window: loopWindow}, true
}

func (r *unitRun) add(s string) (LoopTrip, bool) {
	for _, c := range s {
		switch {
		case c == '\n':
			if t, ok := r.end(); ok {
				return t, true
			}
			continue
		case r.stop && unicode.IsSpace(c):
			if t, ok := r.end(); ok {
				return t, true
			}
			continue
		}
		r.cur.WriteRune(c)
		switch c {
		case '.', '!', '?':
			r.stop = r.lettered
		case '。', '！', '？':
			if t, ok := r.end(); ok {
				return t, true
			}
			continue
		default:
			r.stop = false
			r.lettered = r.lettered || unicode.IsLetter(c)
		}
		if r.cur.Len() >= unitMost {
			if t, ok := r.end(); ok {
				return t, true
			}
		}
	}
	return LoopTrip{}, false
}

// LoopGuard inspects streaming chunks across OpenAI, Anthropic, Responses, and Gemini
// protocols and trips if a model's reasoning or output content falls into a repetitive loop.
type LoopGuard struct {
	pending []byte
	think   unitRun
	text    unitRun
}

// NewLoopGuard creates a new stream loop guard.
func NewLoopGuard() *LoopGuard {
	return &LoopGuard{}
}

// Feed consumes a stream chunk payload and reports whether a loop was detected.
func (g *LoopGuard) Feed(b []byte) (LoopTrip, bool) {
	if len(b) == 0 {
		return LoopTrip{}, false
	}
	g.pending = append(g.pending, b...)
	for {
		idx := bytes.IndexByte(g.pending, '\n')
		if idx < 0 {
			break
		}
		line := g.pending[:idx]
		g.pending = g.pending[idx+1:]

		line = bytes.TrimRight(line, "\r")
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(line[5:])
		if len(data) == 0 || data[0] != '{' {
			continue
		}

		think, text := extractSSEWords(data)
		if think != "" {
			if t, ok := g.think.add(think); ok {
				t.Reasoning = true
				return t, true
			}
		}
		if text != "" {
			if t, ok := g.text.add(text); ok {
				t.Reasoning = false
				return t, true
			}
		}
	}
	if len(g.pending) > pendingMost {
		g.pending = nil
	}
	return LoopTrip{}, false
}

func extractSSEWords(data []byte) (think, text string) {
	var v struct {
		Type    string          `json:"type"`
		Delta   json.RawMessage `json:"delta"`
		Choices []struct {
			Delta struct {
				Content          json.RawMessage `json:"content"`
				ReasoningContent json.RawMessage `json:"reasoning_content"`
				Reasoning        json.RawMessage `json:"reasoning"`
			} `json:"delta"`
		} `json:"choices"`
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.Unmarshal(data, &v) != nil {
		return "", ""
	}

	rawString := func(raw json.RawMessage) string {
		var s string
		if len(raw) > 0 && raw[0] == '"' && json.Unmarshal(raw, &s) == nil {
			return s
		}
		return ""
	}

	switch v.Type {
	case "content_block_delta": // Anthropic
		var d struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		}
		if json.Unmarshal(v.Delta, &d) == nil {
			switch d.Type {
			case "text_delta":
				return "", d.Text
			case "thinking_delta":
				return d.Thinking, ""
			}
		}
		return "", ""
	case "response.output_text.delta": // OpenAI Responses output
		return "", rawString(v.Delta)
	case "response.reasoning_text.delta", "response.reasoning_summary_text.delta": // OpenAI Responses reasoning
		return rawString(v.Delta), ""
	}

	var th, tx strings.Builder
	for _, c := range v.Choices { // Chat Completions
		r := rawString(c.Delta.ReasoningContent)
		if r == "" {
			r = rawString(c.Delta.Reasoning)
		}
		th.WriteString(r)
		tx.WriteString(rawString(c.Delta.Content))
	}
	for _, c := range v.Candidates { // Gemini
		for _, p := range c.Content.Parts {
			if p.Thought {
				th.WriteString(p.Text)
			} else {
				tx.WriteString(p.Text)
			}
		}
	}
	return th.String(), tx.String()
}
