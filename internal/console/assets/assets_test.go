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
