package assets

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestConsoleHTMLIsNeverDiskCached 锁定 index HTML 的缓存指令。
//
// 历史事故（2026-11-08）：console.html 与所有 JS/CSS 资源共用无版本号的 URL
// (/console/)，浏览器把旧 HTML 存进磁盘缓存后，即便服务端已发新版也继续复用，
// 用户侧表现为"部署了但界面没变"，连续三次发版都没生效。
// 根因是 Cache-Control 用了 "no-cache, must-revalidate" —— Chrome 对同 URL 的
// 磁盘副本仍允许直接复用，只有 no-store 才是硬禁止。
func TestConsoleHTMLIsNeverDiskCached(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterUI(r)

	for _, path := range []string{"/console", "/console/", "/console/index.html", "/console/console.html"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, rec.Code)
		}
		cc := rec.Header().Get("Cache-Control")
		if !strings.Contains(cc, "no-store") {
			t.Errorf("GET %s Cache-Control = %q, want it to contain no-store", path, cc)
		}
	}
}

// TestConsoleAssetsStayRevalidatable 确保 JS/CSS 资源仍走 "no-cache" 而非
// "no-store" —— 资源带 "?v=<版本>" URL，允许本地缓存只在版本变化时才回源，
// 关掉缓存会拖慢每次打开控制台的速度。
func TestConsoleAssetsStayRevalidatable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterUI(r)

	req := httptest.NewRequest(http.MethodGet, "/console/console-traffic.js", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET console-traffic.js status = %d, want 200", rec.Code)
	}
	cc := rec.Header().Get("Cache-Control")
	if strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q; assets must stay cacheable for speed", cc)
	}
	if !strings.Contains(cc, "no-cache") {
		t.Errorf("Cache-Control = %q, want it to contain no-cache", cc)
	}
}
