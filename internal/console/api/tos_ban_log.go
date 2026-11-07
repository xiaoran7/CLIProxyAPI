package api

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TOS enforcement is logged by the executor stack under different shapes and
// never lands in gate.db: the accounting middleware only keeps a 300-byte
// prefix of the upstream body, while the marker (reason=TOS_VIOLATION) sits
// far beyond that cut. The application log is therefore the only authoritative
// source for these records.

const (
	// tosLogMaxScanBytes bounds how much of a single log file is inspected so a
	// multi-hundred-megabyte file cannot stall the console.
	tosLogMaxScanBytes = 64 << 20
	// unknownTOSAccount labels a record whose log line carried no identity.
	unknownTOSAccount = "(未知账号)"
	// tosLogMaxLineBytes is well above the largest observed upstream diagnostic
	// (553 bytes) but still guards against pathological single-line files.
	tosLogMaxLineBytes = 1 << 20
)

var (
	// tosLogFilePattern matches the rotating application log names produced by
	// the logger: main.log plus main-<timestamp>.log archives.
	tosLogFilePattern = regexp.MustCompile(`^main(-[0-9T:.\-]+)?\.log$`)
	// tosLogTimestampPattern captures the bracketed local timestamp that starts
	// every logrus line.
	tosLogTimestampPattern = regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\]`)
	// tosStrongPattern identifies a record that positively asserts a terms-of-
	// service enforcement, independent of which executor emitted it.
	tosStrongPattern = regexp.MustCompile(`(?i)violation of Terms of Service|TOS_VIOLATION|This service has been disabled in this account`)
	// tosStatusPattern captures the HTTP status that precedes the failure detail.
	tosStatusPattern        = regexp.MustCompile(`(\d{3}) \|`)
	tosModelPattern         = regexp.MustCompile(`\bmodel=([^\s]+)`)
	tosAuthKindPattern      = regexp.MustCompile(`\b(auth_file|auth_id)=([^\s]+)`)
	tosProviderPattern      = regexp.MustCompile(`\bprovider=([^\s"'}]+)`)
	tosJSONCodePattern      = regexp.MustCompile(`"code"\s*:\s*(\d{3})`)
	tosJSONMessagePattern   = regexp.MustCompile(`"message"\s*:\s*"((?:[^"\\]|\\.)*)"`)
	tosJSONStatusPattern    = regexp.MustCompile(`"status"\s*:\s*"([^"]*)"`)
	tosJSONReasonPattern    = regexp.MustCompile(`"reason"\s*:\s*"([^"]*)"`)
	tosValidationURLPattern = regexp.MustCompile(`https?://[^\s"'\\]+`)
)

// TOSBanRecord is a single violation observed in the application log.
type TOSBanRecord struct {
	TS          int64  `json:"ts"`
	TimeText    string `json:"time_text"`
	Account     string `json:"account"`
	AuthFile    string `json:"auth_file,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model,omitempty"`
	Status      int    `json:"status"`
	ErrorCode   string `json:"error_code,omitempty"`
	Message     string `json:"message"`
	UpstreamMsg string `json:"upstream_message,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Validation  string `json:"validation_url,omitempty"`
	Raw         string `json:"raw"`
	Truncated   bool   `json:"truncated,omitempty"`
}

// TOSLogScanResult is the aggregate produced by scanning the log directory.
type TOSLogScanResult struct {
	Records   []TOSBanRecord
	FilesRead int
	FilesSeen int
	LogDir    string
	Err       error
}

// resolveTOSLogDirectory returns the directory that holds the application logs.
// It mirrors logging.ResolveLogDirectory semantics without importing the logging
// package, which would create an initialization cycle with the console boot path.
func resolveTOSLogDirectory() string {
	if env := strings.TrimSpace(os.Getenv("CPA_LOG_DIR")); env != "" {
		return env
	}
	candidates := []string{"logs", filepath.Join("data", "logs")}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "logs"))
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return candidates[0]
}

// listTOSLogFiles returns the main log and its archives, newest first.
func listTOSLogFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		path string
		mod  time.Time
	}
	var files []candidate
	for _, entry := range entries {
		if entry.IsDir() || !tosLogFilePattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, candidate{path: filepath.Join(dir, entry.Name()), mod: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.path)
	}
	return paths, nil
}

// ScanTOSBanLogs reads every rotated application log under dir and returns the
// terms-of-service enforcement records it can recover. Records are returned
// newest first. A missing log directory is reported through the error field of
// the result rather than as a hard failure, so the console can still render the
// database-derived statistics.
func ScanTOSBanLogs(dir string) TOSLogScanResult {
	result := TOSLogScanResult{LogDir: dir}
	if strings.TrimSpace(dir) == "" {
		dir = resolveTOSLogDirectory()
		result.LogDir = dir
	}
	paths, err := listTOSLogFiles(dir)
	if err != nil {
		result.Err = err
		return result
	}
	result.FilesSeen = len(paths)
	seen := make(map[string]struct{})
	for _, path := range paths {
		records, readErr := scanSingleTOSLog(path)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				continue
			}
			result.Err = readErr
			continue
		}
		result.FilesRead++
		for _, rec := range records {
			// The same line can appear in both main.log and an archive that was
			// rotated from it, so de-duplicate on the stable identity fields.
			key := strconv.FormatInt(rec.TS, 10) + "|" + rec.Account + "|" + rec.Model + "|" + rec.Raw
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result.Records = append(result.Records, rec)
		}
	}
	result.Records = coalesceTOSEchoes(result.Records)
	sort.SliceStable(result.Records, func(i, j int) bool {
		if result.Records[i].TS == result.Records[j].TS {
			return result.Records[i].Account < result.Records[j].Account
		}
		return result.Records[i].TS > result.Records[j].TS
	})
	return result
}

// scanSingleTOSLog parses one log file, stopping after tosLogMaxScanBytes.
func scanSingleTOSLog(path string) ([]TOSBanRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	reader := bufio.NewReaderSize(io.LimitReader(file, tosLogMaxScanBytes), 64*1024)
	var records []TOSBanRecord
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > tosLogMaxLineBytes {
			line = line[:tosLogMaxLineBytes]
		}
		if rec, ok := ParseTOSBanLogLine(line); ok {
			records = append(records, rec)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return records, nil
			}
			return records, readErr
		}
	}
}

// ParseTOSBanLogLine extracts a violation record from one log line. It returns
// false for lines that are not terms-of-service enforcement.
func ParseTOSBanLogLine(line string) (TOSBanRecord, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !tosStrongPattern.MatchString(trimmed) {
		return TOSBanRecord{}, false
	}

	rec := TOSBanRecord{Raw: trimmed, Status: 403}

	if match := tosLogTimestampPattern.FindStringSubmatch(trimmed); match != nil {
		rec.TimeText = match[1]
		if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", match[1], time.Local); err == nil {
			rec.TS = parsed.Unix()
		}
	}

	if match := tosStatusPattern.FindStringSubmatch(trimmed); match != nil {
		if code, err := strconv.Atoi(match[1]); err == nil {
			rec.Status = code
		}
	} else if match := tosJSONCodePattern.FindStringSubmatch(trimmed); match != nil {
		if code, err := strconv.Atoi(match[1]); err == nil {
			rec.Status = code
		}
	}

	if match := tosModelPattern.FindStringSubmatch(trimmed); match != nil {
		rec.Model = match[1]
	}

	if match := tosAuthKindPattern.FindStringSubmatch(trimmed); match != nil {
		switch strings.ToLower(match[1]) {
		case "auth_file":
			rec.AuthFile = match[2]
			rec.Account = AccountFromAuthFile(match[2])
		case "auth_id":
			rec.Account = match[2]
		}
	}
	if rec.Provider == "" {
		if match := tosProviderPattern.FindStringSubmatch(trimmed); match != nil {
			rec.Provider = match[1]
		}
	}

	if match := tosJSONMessagePattern.FindStringSubmatch(trimmed); match != nil {
		rec.UpstreamMsg = unquoteJSONFragment(match[1])
	}
	if match := tosJSONStatusPattern.FindStringSubmatch(trimmed); match != nil {
		rec.ErrorCode = match[1]
	}
	if match := tosJSONReasonPattern.FindStringSubmatch(trimmed); match != nil {
		rec.Reason = match[1]
	}

	if url := tosValidationURLPattern.FindString(trimmed); url != "" {
		rec.Validation = strings.TrimRight(url, "\\,.;")
	}

	switch {
	case rec.UpstreamMsg != "":
		rec.Message = rec.UpstreamMsg
	case rec.Reason != "":
		rec.Message = rec.Reason
	default:
		rec.Message = trimmed
	}
	// The diagnostic is capped at 300 runes upstream, so a trailing ellipsis
	// means the upstream body was cut before the console ever saw it.
	rec.Truncated = strings.HasSuffix(rec.Message, "...") || strings.HasSuffix(trimmed, "...")

	if strings.TrimSpace(rec.Account) == "" {
		rec.Account = unknownTOSAccount
	}
	if rec.Provider == "" && strings.Contains(strings.ToLower(rec.AuthFile), "antigravity") {
		rec.Provider = "antigravity"
	}
	return rec, true
}

// AccountFromAuthFile turns an auth file name into the account identifier that
// operators recognise, e.g. "antigravity-a@b.com.json" -> "a@b.com".
func AccountFromAuthFile(name string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		return ""
	}
	base = filepath.Base(base)
	base = strings.TrimSuffix(base, ".json")
	// Auth files are conventionally "<provider>-<account>.json"; strip the known
	// providers explicitly rather than splitting on the first dash, because some
	// account identifiers legitimately contain dashes.
	for _, prefix := range []string{"antigravity-", "gemini-", "claude-", "codex-", "qwen-", "iflow-", "vertex-"} {
		if strings.HasPrefix(base, prefix) {
			return strings.TrimPrefix(base, prefix)
		}
	}
	return base
}

// unquoteJSONFragment resolves the escape sequences that survive inside a log
// line so the console shows the upstream text rather than its JSON encoding.
func unquoteJSONFragment(value string) string {
	if !strings.Contains(value, `\`) {
		return value
	}
	decoded, err := strconv.Unquote(`"` + value + `"`)
	if err != nil {
		return value
	}
	return decoded
}

// coalesceTOSEchoes merges the anonymous executor debug lines into the named
// conductor record for the same enforcement event. The executor logs the same
// upstream 403 a moment earlier without any account identity; left separate it
// would inflate the account count with a phantom entry.
func coalesceTOSEchoes(records []TOSBanRecord) []TOSBanRecord {
	// Index the records that do name an account, by second.
	namedByTS := make(map[int64][]int, len(records))
	for i, rec := range records {
		if isNamedTOSRecord(rec) {
			namedByTS[rec.TS] = append(namedByTS[rec.TS], i)
		}
	}

	merged := make([]TOSBanRecord, 0, len(records))
	for _, rec := range records {
		if !isNamedTOSRecord(rec) {
			if idxs, ok := namedByTS[rec.TS]; ok && len(idxs) > 0 {
				// Attribute the echo to the named record rather than dropping it:
				// the detail is real, only the identity was missing.
				src := records[idxs[0]]
				rec.Account = src.Account
				rec.AuthFile = src.AuthFile
				rec.Provider = src.Provider
				if rec.Model == "" {
					rec.Model = src.Model
				}
			}
		}
		// Collapse a duplicate that now resolves to the same event as an
		// already-kept record.
		duplicate := false
		for i := range merged {
			if merged[i].TS == rec.TS && merged[i].Account == rec.Account && merged[i].Message == rec.Message {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		merged = append(merged, rec)
	}
	return merged
}

// isNamedTOSRecord reports whether a record carries a real account identity.
func isNamedTOSRecord(rec TOSBanRecord) bool {
	account := strings.TrimSpace(rec.Account)
	return account != "" && account != unknownTOSAccount
}
