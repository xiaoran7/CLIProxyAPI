package assets

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestAssetVersionIsRewrittenFromBuildVersion 锁定缓存击穿机制：
// console.html 里所有 "?v=..." 必须被替换成 BuildVersion。
// 历史事故：版本号硬编码在 console.html 的 37 处引用里，某次发版漏改，
// 导致浏览器继续用旧 URL 命中本地缓存，用户侧"部署了但界面没变"。
func TestAssetVersionIsRewrittenFromBuildVersion(t *testing.T) {
	original := BuildVersion
	defer func() { BuildVersion = original }()

	BuildVersion = "v9.9.9-test"

	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterUI(r)

	req := httptest.NewRequest(http.MethodGet, "/console/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /console/ status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	// 1) 不应再残留任何旧版本号
	stale := regexp.MustCompile(`\?v=v8\.0\.`)
	if stale.MatchString(body) {
		n := len(stale.FindAllString(body, -1))
		t.Errorf("found %d asset references still pinned to v8.0.x; they must be rewritten to the build version", n)
	}

	// 2) 必须出现新版本号
	if !strings.Contains(body, "?v=v9.9.9-test") {
		t.Error("no asset reference carries the injected build version; cache busting will not work")
	}

	// 3) 所有 "?v=" 出现次数应与硬编码版本号数量一致（不能有漏网）
	total := strings.Count(body, "?v=")
	if total == 0 {
		t.Fatal("no cache-busting query found at all")
	}
	withBuild := strings.Count(body, "?v=v9.9.9-test")
	if total != withBuild {
		t.Errorf("%d of %d asset refs were rewritten, %d left stale", withBuild, total, total-withBuild)
	}

	// 4) ETag 必须与替换后的 body 匹配，否则 304 会返回错内容
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag header")
	}
	req2 := httptest.NewRequest(http.MethodGet, "/console/", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotModified {
		t.Errorf("re-request with matching ETag = %d, want 304", rec2.Code)
	}
}

// TestAssetVersionFallsBackToDev 确保未注入版本时不会把 URL 写坏。
func TestAssetVersionFallsBackToDev(t *testing.T) {
	original := BuildVersion
	defer func() { BuildVersion = original }()

	BuildVersion = ""
	got := assetCacheVersion()
	if got != "dev" {
		t.Errorf("assetCacheVersion() with empty BuildVersion = %q, want \"dev\"", got)
	}

	BuildVersion = "dev"
	if got := assetCacheVersion(); got != "dev" {
		t.Errorf("assetCacheVersion() = %q, want \"dev\"", got)
	}

	// dev 模式下不得改写（保持 HTML 原样，便于本地开发）
	data := []byte(`<link href="/console/x.css?v=v8.0.63"/>`)
	out := rewriteAssetVersions(data)
	if string(out) != string(data) {
		t.Errorf("dev mode rewrote the asset URL: %s", out)
	}
}
