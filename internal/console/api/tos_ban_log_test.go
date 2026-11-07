package api

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Real line captured from the Tokyo production log on 2026-11-06. The upstream
// body is cut at the diagnostic limit, which is exactly the truncation the UI
// has to surface.
const tosSampleLine = `[2026-11-06 15:57:26] [7e666964] [warn ] [conductor_execution.go:2157] 403 |         746ms | upstream execution failed: provider=antigravity model=gemini-3.8-flash-high auth=provider=antigravity auth_file=antigravity-adryanlin381@gmail.com.json err={ "error": { "code": 403, "message": "This service has been disabled in this account for violation of Terms of Service. Please submit an appeal to continue using this product.", "status": "PERMISSION_DENIED", "details": [ { "@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": "TOS_VIOLATIO...`

// A shorter variant emitted by the executor's own debug logger. It carries no
// account identity, so the parser must fall back to a placeholder rather than
// inventing one.
const tosDebugLine = `[2026-11-06 19:35:44] [--------] [debug] [antigravity_executor_execute.go:185] antigravity executor: upstream error status: 403, body: This service has been disabled in this account for violation of Terms of Service. Please submit an appeal to continue using this product.`

func TestParseTOSBanLogLine_ConductorWarn(t *testing.T) {
	rec, ok := ParseTOSBanLogLine(tosSampleLine)
	if !ok {
		t.Fatalf("expected the conductor warning to be recognised as a TOS record")
	}
	if rec.Account != "adryanlin381@gmail.com" {
		t.Errorf("account = %q, want adryanlin381@gmail.com", rec.Account)
	}
	if rec.AuthFile != "antigravity-adryanlin381@gmail.com.json" {
		t.Errorf("auth_file = %q", rec.AuthFile)
	}
	if rec.Provider != "antigravity" {
		t.Errorf("provider = %q, want antigravity", rec.Provider)
	}
	if rec.Model != "gemini-3.8-flash-high" {
		t.Errorf("model = %q, want gemini-3.8-flash-high", rec.Model)
	}
	if rec.Status != 403 {
		t.Errorf("status = %d, want 403", rec.Status)
	}
	if rec.ErrorCode != "PERMISSION_DENIED" {
		t.Errorf("error_code = %q, want PERMISSION_DENIED", rec.ErrorCode)
	}
	if rec.TimeText != "2026-11-06 15:57:26" {
		t.Errorf("time = %q", rec.TimeText)
	}
	if rec.TS == 0 {
		t.Errorf("expected a non-zero unix timestamp")
	}
	if rec.Message == "" || rec.Message[:4] != "This" {
		t.Errorf("message = %q, want the upstream text", rec.Message)
	}
	if !rec.Truncated {
		t.Errorf("expected the record to be flagged truncated because the marker is cut at TOS_VIOLATIO...")
	}
}

func TestParseTOSBanLogLine_DebugFallback(t *testing.T) {
	rec, ok := ParseTOSBanLogLine(tosDebugLine)
	if !ok {
		t.Fatalf("expected the executor debug line to be recognised")
	}
	if rec.Status != 403 {
		t.Errorf("status = %d, want 403", rec.Status)
	}
	if rec.Account != "(未知账号)" {
		t.Errorf("account = %q, want the unknown-account placeholder", rec.Account)
	}
	if rec.Model != "" {
		t.Errorf("model = %q, want empty because the debug line carries none", rec.Model)
	}
}

func TestParseTOSBanLogLine_RejectsUnrelatedLines(t *testing.T) {
	cases := []string{
		"",
		"   ",
		`[2026-11-07 16:40:44] [2484d403] [debug] [antigravity_connection_trace.go:73] antigravity upstream connection acquired provider=antigravity auth_index="00b1bf94e0683957"`,
		`[2026-11-07 16:41:12] [2484d403] [info ] [gin_logger.go:113] 200 |       31.325s |     100.101.2.4 | POST    "/v1/messages?beta=true"`,
		`{"error":{"code":"model_not_allowed","message":"Model 'x' is not permitted for this API key."}}`,
	}
	for _, line := range cases {
		if _, ok := ParseTOSBanLogLine(line); ok {
			t.Errorf("expected line to be rejected: %s", line)
		}
	}
}

func TestParseTOSBanLogLine_DoesNotConfuseRequestIDsForStatuses(t *testing.T) {
	// "2484d403" contains 403 but is a request id, not an HTTP status.
	line := `[2026-11-07 16:41:12] [2484d403] [warn ] [conductor_execution.go:2157] upstream execution failed: provider=antigravity model=m auth=provider=antigravity auth_file=antigravity-x@y.com.json err={ "error": { "code": 403, "message": "violation of Terms of Service", "status": "PERMISSION_DENIED" } }`
	rec, ok := ParseTOSBanLogLine(line)
	if !ok {
		t.Fatalf("expected record")
	}
	if rec.Status != 403 {
		t.Errorf("status = %d, want 403 from the JSON code", rec.Status)
	}
}

func TestAccountFromAuthFile(t *testing.T) {
	cases := map[string]string{
		"antigravity-adryanlin381@gmail.com.json": "adryanlin381@gmail.com",
		"gemini-someone@example.com.json":         "someone@example.com",
		"claude-team-user@example.com.json":       "team-user@example.com",
		"/abs/path/antigravity-a@b.com.json":      "a@b.com",
		"":                                        "",
		"plain.json":                              "plain",
	}
	for input, want := range cases {
		if got := AccountFromAuthFile(input); got != want {
			t.Errorf("AccountFromAuthFile(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestGroupTOSRecordsByAccount(t *testing.T) {
	records := []TOSBanRecord{
		{TS: 300, Account: "b@x.com", Model: "m1", AuthFile: "antigravity-b@x.com.json"},
		{TS: 100, Account: "b@x.com", Model: "m2", AuthFile: "antigravity-b@x.com.json"},
		{TS: 200, Account: "a@x.com", Model: "m1", AuthFile: "antigravity-a@x.com.json"},
		{TS: 400, Account: "b@x.com", Model: "m1", AuthFile: "antigravity-b@x.com.json"},
	}
	authByFile := map[string]authFileInfo{
		"antigravity-b@x.com.json": {AuthIndex: "idx-b", Status: "disabled", Disabled: true},
	}

	buckets, order := groupTOSRecordsByAccount(records, authByFile)
	if len(order) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(order))
	}

	b := buckets["b@x.com"].stats
	if b.BanCount != 3 {
		t.Errorf("ban_count = %d, want 3", b.BanCount)
	}
	if b.FirstBannedAt != 100 {
		t.Errorf("first_banned_at = %d, want 100", b.FirstBannedAt)
	}
	if b.LastBannedAt != 400 {
		t.Errorf("last_banned_at = %d, want 400", b.LastBannedAt)
	}
	if b.AuthIndex != "idx-b" {
		t.Errorf("auth_index = %q, want idx-b", b.AuthIndex)
	}
	if !b.AuthDisabled || b.AuthStatus != "disabled" {
		t.Errorf("expected the disabled auth status to be carried through")
	}
	// Duplicate model names must collapse; distinct ones must survive in order.
	if !reflect.DeepEqual(b.Models, []string{"m1", "m2"}) {
		t.Errorf("models = %v, want [m1 m2]", b.Models)
	}

	a := buckets["a@x.com"].stats
	if a.BanCount != 1 {
		t.Errorf("a.ban_count = %d, want 1", a.BanCount)
	}
	if a.AuthIndex != "" {
		t.Errorf("a.auth_index = %q, want empty", a.AuthIndex)
	}
}

func TestScanTOSBanLogs_ReadsRotatedFilesAndDedupes(t *testing.T) {
	dir := t.TempDir()
	// The same line appears in the live log and in an archive rotated from it.
	writeLog := func(name, body string) {
		path := filepath.Join(dir, name)
		if err := writeFile(path, body); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeLog("main.log", tosSampleLine+"\n"+tosDebugLine+"\n")
	writeLog("main-2026-11-06T15-57-26.000.log", tosSampleLine+"\n")
	// Files that must be ignored.
	writeLog("error-v1-messages-2026-11-06.log", tosSampleLine+"\n")
	writeLog("console-2026-11-06.log", tosSampleLine+"\n")

	result := ScanTOSBanLogs(dir)
	if result.Err != nil {
		t.Fatalf("scan error: %v", result.Err)
	}
	if result.FilesSeen != 2 {
		t.Errorf("files_seen = %d, want 2 (only main*.log)", result.FilesSeen)
	}
	if len(result.Records) != 2 {
		t.Fatalf("records = %d, want 2 after de-duplication", len(result.Records))
	}
	// Newest-first ordering: the debug line is 2026-11-06 19:35:44.
	if result.Records[0].TimeText != "2026-11-06 19:35:44" {
		t.Errorf("first record = %q, want the newer debug line", result.Records[0].TimeText)
	}
}

func TestScanTOSBanLogs_MissingDirectory(t *testing.T) {
	result := ScanTOSBanLogs(filepath.Join(t.TempDir(), "does-not-exist"))
	if result.Err == nil {
		t.Fatalf("expected an error for a missing directory")
	}
	if len(result.Records) != 0 {
		t.Errorf("records = %d, want 0", len(result.Records))
	}
}

func TestBuildTOSSummary_AggregatesWindowsAndTruncation(t *testing.T) {
	accounts := []TOSAccountStats{
		{Forbidden24h: 2, Forbidden7d: 5, TotalForbidden: 9, Records: []TOSBanRecord{{Truncated: true}, {}}},
		{Forbidden24h: 1, Forbidden7d: 3, TotalForbidden: 4, Records: []TOSBanRecord{{Truncated: true}}},
	}
	scan := TOSLogScanResult{Records: make([]TOSBanRecord, 3), FilesRead: 2, LogDir: "/logs"}

	summary := buildTOSSummary(accounts, scan)
	if summary.BannedAccounts != 2 {
		t.Errorf("banned_accounts = %d, want 2", summary.BannedAccounts)
	}
	if summary.Forbidden24h != 3 || summary.Forbidden7d != 8 || summary.ForbiddenTotal != 13 {
		t.Errorf("forbidden roll-up = %d/%d/%d, want 3/8/13",
			summary.Forbidden24h, summary.Forbidden7d, summary.ForbiddenTotal)
	}
	if summary.TruncatedCount != 2 {
		t.Errorf("truncated_records = %d, want 2", summary.TruncatedCount)
	}
	if summary.RecordCount != 3 {
		t.Errorf("record_count = %d, want 3", summary.RecordCount)
	}
	if summary.FilesScanned != 2 || summary.LogDir != "/logs" {
		t.Errorf("scan metadata not carried through")
	}
}

func TestUnquoteJSONFragment(t *testing.T) {
	if got := unquoteJSONFragment(`a \"quoted\" b`); got != `a "quoted" b` {
		t.Errorf("unquote = %q", got)
	}
	if got := unquoteJSONFragment("plain"); got != "plain" {
		t.Errorf("plain text must be returned unchanged, got %q", got)
	}
}

// writeFile is a tiny helper so the test file does not need to import os just
// for setup.
func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}

func TestCoalesceTOSEchoesMergesAnonymousEcho(t *testing.T) {
	// The executor logs the anonymous debug line a moment before the named
	// conductor record for the same enforcement event.
	records := []TOSBanRecord{
		{TS: 1794013244, Account: unknownTOSAccount, Message: "This service has been disabled in this account for violation of Terms of Service.", Status: 403},
		{TS: 1794013244, Account: "adryanlin381@gmail.com", AuthFile: "antigravity-adryanlin381@gmail.com.json", Provider: "antigravity", Model: "gemini-3.8-flash-high", Message: "This service has been disabled in this account for violation of Terms of Service.", Status: 403},
		{TS: 1794023260, Account: "adryanlin381@gmail.com", Model: "gemini-3.8-flash-high", Message: "other", Status: 403},
	}

	merged := coalesceTOSEchoes(records)
	if len(merged) != 2 {
		t.Fatalf("merged = %d records, want 2 (echo folded into its named sibling)", len(merged))
	}
	for _, rec := range merged {
		if rec.Account == unknownTOSAccount {
			t.Fatalf("phantom unknown account survived coalescing: %+v", rec)
		}
	}
	// The surviving record for that second must carry the real identity.
	found := false
	for _, rec := range merged {
		if rec.TS == 1794013244 {
			found = true
			if rec.AuthFile == "" || rec.Provider != "antigravity" {
				t.Errorf("echo identity not backfilled: %+v", rec)
			}
		}
	}
	if !found {
		t.Errorf("expected a record at ts 1794013244")
	}
}

func TestCoalesceTOSEchoesKeepsOrphanAnonymous(t *testing.T) {
	// No named sibling exists, so the anonymous record must be preserved rather
	// than silently dropped.
	records := []TOSBanRecord{
		{TS: 100, Account: unknownTOSAccount, Message: "violation of Terms of Service", Status: 403},
	}
	merged := coalesceTOSEchoes(records)
	if len(merged) != 1 {
		t.Fatalf("merged = %d, want 1 orphan preserved", len(merged))
	}
	if merged[0].Account != unknownTOSAccount {
		t.Errorf("account = %q, want the placeholder preserved", merged[0].Account)
	}
}
