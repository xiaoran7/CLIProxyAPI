package assets

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEmbeddedAssets(t *testing.T) {
	// 1. Verify critical files exist
	files := []string{
		"console.html",
		"console-style.css",
		"console-i18n.js",
		"console-keys.js",
		"console-quotas.js",
		"console-stats.js",
		"console-logs.js",
		"console-cpa.js",
		"console-cpa.css",
		"console-overview.js",
		"console-overview.css",
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

	// 5. Test /console/console-style.css
	req = httptest.NewRequest(http.MethodGet, "/console/console-style.css", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /console/console-style.css, got %d", w.Code)
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

	// 8. Test legacy image alias /admin/assets/gate-sakura-mark-128.webp
	req = httptest.NewRequest(http.MethodGet, "/admin/assets/gate-sakura-mark-128.webp", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for legacy image asset, got %d", w.Code)
	}
}
