package assets

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEmbeddedAssets(t *testing.T) {
	// 1. Verify critical files exist
	files := []string{
		"console.html",
		"console-base.css",
		"console-panels.css",
		"console-members.css",
		"console-tables.css",
		"console-modals.css",
		"console-feedback.css",
		"console-logs.css",
		"console-theme-beszel.css",
		"console-theme-v239.css",
		"console-theme-v2310.css",
		"console-validations.css",
		"console-upstream-stats.css",
		"console-i18n.js",
		"console-keys.js",
		"console-quotas-priority.js",
		"console-quotas-validation.js",
		"console-quotas-fetch.js",
		"console-quotas-render.js",
		"console-quotas-workbuddy.js",
		"console-quotas-stats.js",
		"console-stats.js",
		"console-logs.js",
		"console-traffic.css",
		"console-traffic.js",
		"console-cpa.js",
		"console-cpa.css",
		"console-overview.js",
		"console-overview.css",
		"console-state.js",
		"console-nav.js",
		"console-auth.js",
		"console-format.js",
		"console-polling.js",
		"console-member.js",
		"console-refresh.js",
		"console-init.js",
		"console-panels-quota.css",
		"console-panels-compact.css",
		"console-panels-status.css",
		"console-i18n-dict-common.js",
		"console-i18n-dict-stats.js",
		"console-i18n-dict-quotas.js",
		"console-i18n-dict-keys.js",
		"console-i18n-dict-logs.js",
		"console-quotas-meter-single.js",
		"console-quotas-meter-multi.js",
		"console-quotas-cards-compact.js",
		"console-quotas-cards-full.js",
		"console-keys-modals.js",
		"console-keys-picker.js",
		"console-keys-pricing.js",
		"console-logs-data.js",
		"console-logs-errors.js",
		"console-traffic-icons.js",
		"console-traffic-topology.js",
		"console-overview-charts.js",
		"images/routing-header.png",
		"images/gate-sakura-mark-128.png",
		"images/gate-sakura-mark-128.webp",
	}

	for _, f := range files {
		data, err := GetAsset(f)
		if err != nil || len(data) == 0 {
			t.Fatalf("asset %s failed to load or is empty: %v", f, err)
		}
	}
}

func TestStaticRoutesServing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterUI(r)

	// 1. Test /console serving console.html
	req := httptest.NewRequest(http.MethodGet, "/console", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /console, got %d", w.Code)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag header for /console")
	}

	// 2. Test 304 Not Modified with If-None-Match
	req = httptest.NewRequest(http.MethodGet, "/console", nil)
	req.Header.Set("If-None-Match", etag)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotModified {
		t.Fatalf("expected 304 for /console with matching ETag, got %d", w.Code)
	}

	// 3. Test /admin redirect to /console
	req = httptest.NewRequest(http.MethodGet, "/admin", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301 for /admin, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/console" {
		t.Fatalf("expected redirect to /console, got %s", loc)
	}

	// 4. Test /member redirect to /console
	req = httptest.NewRequest(http.MethodGet, "/member", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 for /member, got %d", w.Code)
	}

	// 5. Test /console/console-base.css
	req = httptest.NewRequest(http.MethodGet, "/console/console-base.css", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /console/console-base.css, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/css; charset=utf-8" {
		t.Fatalf("unexpected content type: %s", ct)
	}

	// 6. Test legacy alias /admin/style.css
	req = httptest.NewRequest(http.MethodGet, "/admin/style.css", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /admin/style.css, got %d", w.Code)
	}

	// 7. Test image asset /console/assets/gate-sakura-mark-128.webp
	req = httptest.NewRequest(http.MethodGet, "/console/assets/gate-sakura-mark-128.webp", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for image asset, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/webp" {
		t.Fatalf("unexpected image content type: %s", ct)
	}

	// 8. Test /console/console-traffic.css and /console/console-traffic.js
	req = httptest.NewRequest(http.MethodGet, "/console/console-traffic.css", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /console/console-traffic.css, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/css; charset=utf-8" {
		t.Fatalf("unexpected css content type: %s", ct)
	}

	req = httptest.NewRequest(http.MethodGet, "/console/console-traffic.js", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /console/console-traffic.js, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/javascript; charset=utf-8" {
		t.Fatalf("unexpected js content type: %s", ct)
	}

	// 8. Test legacy image alias /admin/assets/gate-sakura-mark-128.webp
	req = httptest.NewRequest(http.MethodGet, "/admin/assets/gate-sakura-mark-128.webp", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for legacy image asset, got %d", w.Code)
	}

	// 9. Test HTTP HEAD on /console (curl -I)
	reqHead := httptest.NewRequest(http.MethodHead, "/console", nil)
	wHead := httptest.NewRecorder()
	r.ServeHTTP(wHead, reqHead)
	if wHead.Code != http.StatusOK {
		t.Fatalf("expected 200 for HEAD /console, got %d", wHead.Code)
	}
	if wHead.Body.Len() != 0 {
		t.Fatalf("expected empty body for HEAD /console, got %d bytes", wHead.Body.Len())
	}
	if ct := wHead.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html for HEAD /console, got %s", ct)
	}

	// 10. Test HTTP HEAD on /admin (redirect)
	reqAdminHead := httptest.NewRequest(http.MethodHead, "/admin", nil)
	wAdminHead := httptest.NewRecorder()
	r.ServeHTTP(wAdminHead, reqAdminHead)
	if wAdminHead.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301 for HEAD /admin, got %d", wAdminHead.Code)
	}

	// 11. Test HTTP HEAD on CSS asset
	reqCssHead := httptest.NewRequest(http.MethodHead, "/console/console-base.css", nil)
	wCssHead := httptest.NewRecorder()
	r.ServeHTTP(wCssHead, reqCssHead)
	if wCssHead.Code != http.StatusOK {
		t.Fatalf("expected 200 for HEAD CSS, got %d", wCssHead.Code)
	}
	if wCssHead.Body.Len() != 0 {
		t.Fatalf("expected empty body for HEAD CSS, got %d bytes", wCssHead.Body.Len())
	}

	// 12. Test HTTP HEAD on image asset
	reqImgHead := httptest.NewRequest(http.MethodHead, "/console/assets/gate-sakura-mark-128.webp", nil)
	wImgHead := httptest.NewRecorder()
	r.ServeHTTP(wImgHead, reqImgHead)
	if wImgHead.Code != http.StatusOK {
		t.Fatalf("expected 200 for HEAD image, got %d", wImgHead.Code)
	}
	if wImgHead.Body.Len() != 0 {
		t.Fatalf("expected empty body for HEAD image, got %d bytes", wImgHead.Body.Len())
	}
}

// TestConsoleJSRegexLiterals guards a class of bug that a syntax check cannot
// catch: escape sequences written inside a plain string instead of a regex
// literal. `str.split('\s*·\s*')` parses fine, type-checks as a string, and
// silently never splits, so the call returns the input unchanged. The geo
// label formatter shipped with exactly that defect and kept rendering the
// three-part "中国 · 湖南 · 株洲" until the literal was corrected.
func TestConsoleJSRegexLiterals(t *testing.T) {
	// Every embedded JS file is covered: the hazard is invisible to a syntax
	// check, so a file that is never listed is a file that is never guarded.
	entries, errRead := fs.ReadDir(embeddedAssets, ".")
	if errRead != nil {
		t.Fatalf("read embedded assets: %v", errRead)
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".js") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no embedded JS files found")
	}
	// A quoted string whose body contains \s, \d, \w or \b is almost certainly
	// a regex that was meant to be a literal. Go string escaping doubles the
	// backslash, so the raw source has a single backslash at runtime.
	badEscapes := []string{`'\s`, `"\s`, `'\d`, `"\d`, `'\w`, `"\w`, `'\b`, `"\b`}

	for _, name := range files {
		data, errFile := embeddedAssets.ReadFile(name)
		if errFile != nil {
			t.Fatalf("read %s: %v", name, errFile)
		}
		src := string(data)
		for _, pattern := range badEscapes {
			if idx := strings.Index(src, pattern); idx >= 0 {
				line := 1 + strings.Count(src[:idx], "\n")
				t.Errorf("%s:%d contains a regex escape inside a string literal (%q); use a /.../ literal", name, line, pattern)
			}
		}
	}
}

// TestConsolePartialsAssembleIntoShell locks the server-side assembly of
// console.html. console.html was one 1467-line document until it was split
// into a small shell plus one partial per tab page and per modal
// (AGENTS.md §2.8). The browser still receives a single document, so the
// assembly has to be exact: every partial present, in document order, with no
// placeholder left behind and no anchor lost.
func TestConsolePartialsAssembleIntoShell(t *testing.T) {
	shell, err := GetAsset("console.html")
	if err != nil {
		t.Fatalf("read console.html: %v", err)
	}
	assembled, err := assembleConsoleHTML(shell)
	if err != nil {
		t.Fatalf("assemble console.html: %v", err)
	}
	body := string(assembled)

	for _, group := range consolePartials {
		if strings.Contains(body, group.marker) {
			t.Errorf("assembled console.html still contains the placeholder %s", group.marker)
		}
		offset := 0
		for _, name := range group.files {
			raw, errPart := GetAsset(name)
			if errPart != nil {
				t.Fatalf("read partial %s: %v", name, errPart)
			}
			needle := strings.TrimRight(string(raw), "\n")
			idx := strings.Index(body[offset:], needle)
			if idx < 0 {
				t.Errorf("partial %s is missing from the assembled page or out of document order", name)
				break
			}
			offset += idx
		}
	}

	// every tab page and modal anchor must survive the split
	for _, id := range []string{
		"tab-page-quotas", "tab-page-status", "tab-page-upstream-stats",
		"tab-page-overview", "tab-page-keys", "tab-page-traffic",
		"tab-page-logs", "tab-page-errors", "tab-page-tos-bans",
		"tab-page-validations", "manual-val-modal", "auth-modal",
		"prices-modal", "modal-openrouter-accounts",
	} {
		if !strings.Contains(body, `id="`+id+`"`) {
			t.Errorf("assembled console.html is missing the anchor #%s", id)
		}
	}
}
