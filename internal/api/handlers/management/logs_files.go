package management

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/logging"
)

func (h *Handler) logDirectory() string {
	if h == nil {
		return ""
	}
	if h.logDir != "" {
		return h.logDir
	}
	return logging.ResolveLogDirectory(h.cfg)
}

func (h *Handler) collectLogFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		path  string
		order int64
	}
	cands := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == defaultLogFileName {
			cands = append(cands, candidate{path: filepath.Join(dir, name), order: 0})
			continue
		}
		if order, ok := rotationOrder(name); ok {
			cands = append(cands, candidate{path: filepath.Join(dir, name), order: order})
		}
	}
	if len(cands) == 0 {
		return []string{}, nil
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].order < cands[j].order })
	paths := make([]string, 0, len(cands))
	for i := len(cands) - 1; i >= 0; i-- {
		paths = append(paths, cands[i].path)
	}
	return paths, nil
}

type logFileMeta struct {
	prefix  string
	time    time.Time
	seq     int
	hasTime bool
}

func parseLogMetadata(filename string) logFileMeta {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	lastHyphen := strings.LastIndex(base, "-")
	if lastHyphen <= 0 {
		return logFileMeta{prefix: base}
	}
	beforeID := base[:lastHyphen]

	seq := 0
	beforeSeq := beforeID
	if lastUnderscore := strings.LastIndex(beforeID, "_"); lastUnderscore >= 0 {
		if parsedSeq, errConv := strconv.Atoi(beforeID[lastUnderscore+1:]); errConv == nil {
			seq = parsedSeq
			beforeSeq = beforeID[:lastUnderscore]
		}
	}

	const tsLen = 17 // len("2006-01-02T150405")
	if len(beforeSeq) >= tsLen {
		tsStr := beforeSeq[len(beforeSeq)-tsLen:]
		if t, errTime := time.Parse("2006-01-02T150405", tsStr); errTime == nil {
			prefix := beforeSeq
			if len(beforeSeq) > tsLen && beforeSeq[len(beforeSeq)-tsLen-1] == '-' {
				prefix = beforeSeq[:len(beforeSeq)-tsLen-1]
			}
			return logFileMeta{
				prefix:  prefix,
				time:    t,
				seq:     seq,
				hasTime: true,
			}
		}
	}

	return logFileMeta{
		prefix: beforeSeq,
		seq:    seq,
	}
}

// logFileIsNewer determines whether candidate should take precedence over current.
// It compares file modification times, and on ties, compares the embedded timestamp
// and collision sequence for matching prefixes.
func logFileIsNewer(candidateName string, candidateMod time.Time, currentName string, currentMod time.Time) bool {
	if candidateMod.After(currentMod) {
		return true
	}
	if candidateMod.Before(currentMod) {
		return false
	}
	candMeta := parseLogMetadata(candidateName)
	currMeta := parseLogMetadata(currentName)

	if candMeta.hasTime && currMeta.hasTime {
		if !candMeta.time.Equal(currMeta.time) {
			return candMeta.time.After(currMeta.time)
		}
		if candMeta.prefix == currMeta.prefix && candMeta.seq != currMeta.seq {
			return candMeta.seq > currMeta.seq
		}
	}
	return candidateName > currentName
}

func isRotatedLogFile(name string) bool {
	if _, ok := rotationOrder(name); ok {
		return true
	}
	return false
}

func rotationOrder(name string) (int64, bool) {
	if order, ok := numericRotationOrder(name); ok {
		return order, true
	}
	if order, ok := timestampRotationOrder(name); ok {
		return order, true
	}
	return 0, false
}

func numericRotationOrder(name string) (int64, bool) {
	if !strings.HasPrefix(name, defaultLogFileName+".") {
		return 0, false
	}
	suffix := strings.TrimPrefix(name, defaultLogFileName+".")
	if suffix == "" {
		return 0, false
	}
	n, err := strconv.Atoi(suffix)
	if err != nil {
		return 0, false
	}
	return int64(n), true
}

func timestampRotationOrder(name string) (int64, bool) {
	ext := filepath.Ext(defaultLogFileName)
	base := strings.TrimSuffix(defaultLogFileName, ext)
	if base == "" {
		return 0, false
	}
	prefix := base + "-"
	if !strings.HasPrefix(name, prefix) {
		return 0, false
	}
	clean := strings.TrimPrefix(name, prefix)
	if strings.HasSuffix(clean, ".gz") {
		clean = strings.TrimSuffix(clean, ".gz")
	}
	if ext != "" {
		if !strings.HasSuffix(clean, ext) {
			return 0, false
		}
		clean = strings.TrimSuffix(clean, ext)
	}
	if clean == "" {
		return 0, false
	}
	if idx := strings.IndexByte(clean, '.'); idx != -1 {
		clean = clean[:idx]
	}
	parsed, err := time.ParseInLocation("2006-01-02T15-04-05", clean, time.Local)
	if err != nil {
		return 0, false
	}
	return math.MaxInt64 - parsed.Unix(), true
}

func parseCutoff(raw string) int64 {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0
	}
	ts, err := strconv.ParseInt(value, 10, 64)
	if err != nil || ts <= 0 {
		return 0
	}
	return ts
}

func parseLimit(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("must be a positive integer")
	}
	if limit <= 0 {
		return 0, fmt.Errorf("must be greater than zero")
	}
	return limit, nil
}

func parseTimestamp(line string) int64 {
	if strings.HasPrefix(line, "[") {
		line = line[1:]
	}
	if len(line) < 19 {
		return 0
	}
	candidate := line[:19]
	t, err := time.ParseInLocation("2006-01-02 15:04:05", candidate, time.Local)
	if err != nil {
		return 0
	}
	return t.Unix()
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
