package assets

import (
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestConsoleDocumentIsStableAcrossRequests 锁定 /console 返回的文档与 ETag 稳定。
//
// v8.0.76 把 1467 行的 console.html 拆成壳 + 14 个 partial，由服务端在请求
// 时拼装。拼装结果被记忆化之后，记忆化的键、缓存的字节或 ETag 的计算只要
// 有��处错，浏览器就会拿到半份文档，或者拿到与 body 不匹配的 ETag（304 会
// 返回错内容，正是 2026-11-08 那次缓存事故的形态）。
//
// 这里锁四件事：两次请求 body 逐字节相同、ETag 相同、ETag 等于 body 的
// MD5 前 8 字节、拼装占位标记没有残留在响应里。
func TestConsoleDocumentIsStableAcrossRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterUI(r)

	fetch := func() (*httptest.ResponseRecorder, string) {
		req := httptest.NewRequest(http.MethodGet, "/console", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec, rec.Body.String()
	}

	rec1, body1 := fetch()
	if rec1.Code != http.StatusOK {
		t.Fatalf("first GET /console status = %d, want 200", rec1.Code)
	}
	rec2, body2 := fetch()
	if rec2.Code != http.StatusOK {
		t.Fatalf("second GET /console status = %d, want 200", rec2.Code)
	}

	if body1 != body2 {
		t.Errorf("two GETs of /console returned different documents (%d vs %d bytes)", len(body1), len(body2))
	}

	etag1 := rec1.Header().Get("ETag")
	etag2 := rec2.Header().Get("ETag")
	if etag1 == "" {
		t.Fatal("no ETag header on /console")
	}
	if etag1 != etag2 {
		t.Errorf("ETag is not stable across requests: %q vs %q", etag1, etag2)
	}

	sum := md5.Sum([]byte(body1))
	want := `"` + hex.EncodeToString(sum[:8]) + `"`
	if etag1 != want {
		t.Errorf("ETag %q does not describe the served body (want %q); a 304 would return the wrong content", etag1, want)
	}

	for _, marker := range []string{"<!--CONSOLE_PAGES-->", "<!--CONSOLE_MODALS-->"} {
		if strings.Contains(body1, marker) {
			t.Errorf("the served document still carries the assembly placeholder %s", marker)
		}
	}
}
