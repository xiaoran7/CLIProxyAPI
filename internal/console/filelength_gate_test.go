package console

// This file implements the single-file size gate for the native Console tree.
//
// AGENTS.md §2.8 sets the engineering baseline for file size:
//   - 300~500 lines is the healthy band for ordinary source files;
//   - above 800 lines triggers a refactoring warning;
//   - 1000 lines or more is architectural degradation and the file MUST be
//     split into orthogonal sub-modules.
//
// A rule that nobody can run is a note, not a rule, so this test makes the
// baseline executable with three enforcement layers:
//
//  1. Hard limit. A file at or above fileLengthFailAt (1000) fails the build,
//     unless it carries a documented exemption in its header.
//  2. Registration. A file above fileLengthWarnAt (800) that is not registered
//     in pendingSplit fails the build outright. Registering a file is a
//     promise to split it, not a way to silence the gate.
//  3. Ratchet. A registered file is pinned to the line count recorded in
//     pendingSplit: it may shrink, never grow. Splitting a file removes its
//     entry; a registered path that no longer exists is a stale entry and
//     fails until the entry is cleaned up.
//
// Exemptions follow AGENTS.md §2.8.3: the file header must carry the marker
// "file-length-exempt" followed by a colon, together with the reason and the
// architecture note, and the reason must name one of the categories the rule
// allows. Only
// genuinely indivisible code qualifies - core algorithms, dense state
// machines, continuous math or physics, low level bit manipulation, third
// party DSLs, generated output.
//
// Scope is internal/console only. Every other directory in this repository is
// upstream code (router-for-me/CLIProxyAPI) and is not ours to reshape.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	// fileLengthWarnAt is the AGENTS.md §2.8.2 refactoring-warning threshold.
	fileLengthWarnAt = 800
	// fileLengthFailAt is the AGENTS.md §2.8.2 architectural-degradation threshold.
	fileLengthFailAt = 1000
	// exemptionMarker opts a file out of the gate when it appears in the header.
	exemptionMarker = "file-length-exempt:"
	// exemptionHeaderLines is how far into a file the marker may sit.
	exemptionHeaderLines = 40
	// exemptionMinReasonLen rejects lazy one-word exemptions.
	exemptionMinReasonLen = 12
)

// fileLengthExemptCategories lists the AGENTS.md §2.8.3 scenarios in which a
// long file is architecturally justified rather than a symptom of neglect.
// An exemption reason must name one of them.
var fileLengthExemptCategories = []string{
	"核心算法", "状态机", "连续", "汇编", "位操作", "DSL", "代码生成", "不可拆分", "数据字典",
	"algorithm", "state machine", "assembl", "bit-level", "generated", "indivisible",
}

// pendingSplit registers the files that are still over the warning threshold
// and are queued for a split. Each entry pins the file to a ceiling: it may
// shrink, never grow. Splitting a file removes its entry here.
//
// The table is empty as of v8.0.76: every file the gate ever flagged has been
// split, so the gate is now a pure ceiling check over internal/console. The
// map stays so that the next oversized file has an obvious place to be
// registered, and so a file that is split without removing its entry fails
// the build as a stale entry.
//
// Paths are relative to internal/console.
var pendingSplit = map[string]int{}

// consoleTreeRoot returns the root of the native Console tree.
func consoleTreeRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "internal", "console")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("cannot locate the repository root from %s", dir)
		}
		dir = parent
	}
}

// consoleSourceExtensions are the file types the gate governs.
var consoleSourceExtensions = map[string]bool{
	".go": true, ".js": true, ".css": true, ".html": true,
}

// collectSourceFiles lists every governed file under root, relative to it.
func collectSourceFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		name := info.Name()
		if info.IsDir() {
			if name == "testdata" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		if !consoleSourceExtensions[strings.ToLower(filepath.Ext(name))] {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(files)
	return files
}

// countSourceLines counts the lines of a file the way wc -l does, plus one
// for a trailing line that has no newline terminator.
func countSourceLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte("\n"))
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// exemptionReason inspects the file header for an exemption marker.
//
// It returns found=true when the marker is present. reason is empty when the
// marker carries no usable justification, which the caller treats as a hard
// failure.
func exemptionReason(data []byte) (reason string, found bool) {
	lines := strings.SplitN(string(data), "\n", exemptionHeaderLines+1)
	if len(lines) > exemptionHeaderLines {
		lines = lines[:exemptionHeaderLines]
	}
	for _, line := range lines {
		idx := strings.Index(line, exemptionMarker)
		if idx < 0 {
			continue
		}
		rest := line[idx+len(exemptionMarker):]
		rest = strings.TrimRight(rest, "*/- \t")
		rest = strings.TrimSpace(rest)
		return rest, true
	}
	return "", false
}

// exemptionReasonIsValid reports whether the reason justifies an exemption
// under AGENTS.md §2.8.3.
func exemptionReasonIsValid(reason string) bool {
	if len(reason) < exemptionMinReasonLen {
		return false
	}
	lower := strings.ToLower(reason)
	for _, category := range fileLengthExemptCategories {
		if strings.Contains(reason, category) || strings.Contains(lower, strings.ToLower(category)) {
			return true
		}
	}
	return false
}

// TestConsoleFileLengthGate enforces the AGENTS.md §2.8 single-file size rule
// over the native Console tree.
func TestConsoleFileLengthGate(t *testing.T) {
	root := consoleTreeRoot(t)
	files := collectSourceFiles(t, root)
	seen := make(map[string]bool, len(files))

	var failures []string
	var registered []string
	var exempted []string

	for _, rel := range files {
		seen[rel] = true
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		lines := countSourceLines(data)

		if reason, found := exemptionReason(data); found {
			if !exemptionReasonIsValid(reason) {
				failures = append(failures, fmt.Sprintf(
					"%s: %s marker without a valid reason (need %d+ chars naming one of %v)",
					rel, exemptionMarker, exemptionMinReasonLen, fileLengthExemptCategories))
				continue
			}
			exempted = append(exempted, fmt.Sprintf("%-40s %5d lines  %s", rel, lines, reason))
			continue
		}

		if lines < fileLengthWarnAt {
			continue
		}

		ceiling, ok := pendingSplit[rel]
		if !ok {
			severity := "warning threshold"
			if lines >= fileLengthFailAt {
				severity = "architectural degradation threshold"
			}
			failures = append(failures, fmt.Sprintf(
				"%s: %d lines exceeds the %d-line %s and is not registered in pendingSplit",
				rel, lines, fileLengthWarnAt, severity))
			continue
		}
		if lines > ceiling {
			failures = append(failures, fmt.Sprintf(
				"%s: %d lines grew past its recorded ceiling of %d - split it instead of growing it",
				rel, lines, ceiling))
			continue
		}
		registered = append(registered, fmt.Sprintf("%-40s %5d / %5d lines", rel, lines, ceiling))
	}

	// A registered path that no longer exists means the file was split and the
	// entry was left behind: the table has to stay accurate.
	stale := make([]string, 0, len(pendingSplit))
	for rel := range pendingSplit {
		if !seen[rel] {
			stale = append(stale, rel)
		}
	}
	sort.Strings(stale)
	for _, rel := range stale {
		failures = append(failures, fmt.Sprintf(
			"%s: registered in pendingSplit but the file no longer exists - remove the entry", rel))
	}

	if len(registered) > 0 {
		t.Logf("files still queued for a split (ratchet locked, may shrink but never grow):")
		for _, line := range registered {
			t.Logf("  %s", line)
		}
	}
	if len(exempted) > 0 {
		t.Logf("files exempted under AGENTS.md §2.8.3:")
		for _, line := range exempted {
			t.Logf("  %s", line)
		}
	}
	t.Logf("gate scope: %d files under %s, thresholds warn=%d fail=%d",
		len(files), filepath.Base(root), fileLengthWarnAt, fileLengthFailAt)

	for _, failure := range failures {
		t.Errorf("file length gate: %s", failure)
	}
}
