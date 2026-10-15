package handlers

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func gzipCompress(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	return buf.Bytes()
}

func brotliCompress(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := brotli.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatalf("brotli write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("brotli close failed: %v", err)
	}
	return buf.Bytes()
}

func deflateCompress(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatalf("zlib write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zlib close failed: %v", err)
	}
	return buf.Bytes()
}

func rawFlateCompress(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("flate writer init failed: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("flate write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("flate close failed: %v", err)
	}
	return buf.Bytes()
}

func zstdCompress(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatalf("zstd writer init failed: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("zstd write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zstd close failed: %v", err)
	}
	return buf.Bytes()
}

func TestReadRequestBody_Plain(t *testing.T) {
	payload := `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	body, err := ReadRequestBody(c)
	if err != nil {
		t.Fatalf("ReadRequestBody failed: %v", err)
	}
	if string(body) != payload {
		t.Fatalf("expected payload %q, got %q", payload, string(body))
	}
}

func TestReadRequestBody_Gzip(t *testing.T) {
	payload := `{"model":"gpt-4o","prompt":"test gzip decompression"}`
	compressed := gzipCompress(t, []byte(payload))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(compressed))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	body, err := ReadRequestBody(c)
	if err != nil {
		t.Fatalf("ReadRequestBody gzip failed: %v", err)
	}
	if string(body) != payload {
		t.Fatalf("expected payload %q, got %q", payload, string(body))
	}
}

func TestReadRequestBody_XGzip(t *testing.T) {
	payload := `{"model":"gpt-4o","prompt":"test x-gzip"}`
	compressed := gzipCompress(t, []byte(payload))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(compressed))
	req.Header.Set("Content-Encoding", "x-gzip")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	body, err := ReadRequestBody(c)
	if err != nil {
		t.Fatalf("ReadRequestBody x-gzip failed: %v", err)
	}
	if string(body) != payload {
		t.Fatalf("expected payload %q, got %q", payload, string(body))
	}
}

func TestReadRequestBody_Brotli(t *testing.T) {
	payload := `{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"test brotli"}]}`
	compressed := brotliCompress(t, []byte(payload))

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(compressed))
	req.Header.Set("Content-Encoding", "br")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	body, err := ReadRequestBody(c)
	if err != nil {
		t.Fatalf("ReadRequestBody brotli failed: %v", err)
	}
	if string(body) != payload {
		t.Fatalf("expected payload %q, got %q", payload, string(body))
	}
}

func TestReadRequestBody_Deflate(t *testing.T) {
	payload := `{"model":"gemini-2.5-pro","content":"test deflate"}`

	// Test zlib-wrapped deflate
	compressedZlib := deflateCompress(t, []byte(payload))
	reqZlib := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(compressedZlib))
	reqZlib.Header.Set("Content-Encoding", "deflate")
	wZlib := httptest.NewRecorder()
	cZlib, _ := gin.CreateTestContext(wZlib)
	cZlib.Request = reqZlib

	body, err := ReadRequestBody(cZlib)
	if err != nil {
		t.Fatalf("ReadRequestBody deflate (zlib) failed: %v", err)
	}
	if string(body) != payload {
		t.Fatalf("expected payload %q, got %q", payload, string(body))
	}

	// Test raw deflate
	compressedRaw := rawFlateCompress(t, []byte(payload))
	reqRaw := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(compressedRaw))
	reqRaw.Header.Set("Content-Encoding", "deflate")
	wRaw := httptest.NewRecorder()
	cRaw, _ := gin.CreateTestContext(wRaw)
	cRaw.Request = reqRaw

	bodyRaw, errRaw := ReadRequestBody(cRaw)
	if errRaw != nil {
		t.Fatalf("ReadRequestBody deflate (raw) failed: %v", errRaw)
	}
	if string(bodyRaw) != payload {
		t.Fatalf("expected payload %q, got %q", payload, string(bodyRaw))
	}
}

func TestReadRequestBody_Zstd(t *testing.T) {
	payload := `{"model":"gpt-4o","prompt":"test zstd"}`
	compressed := zstdCompress(t, []byte(payload))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(compressed))
	req.Header.Set("Content-Encoding", "zstd")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	body, err := ReadRequestBody(c)
	if err != nil {
		t.Fatalf("ReadRequestBody zstd failed: %v", err)
	}
	if string(body) != payload {
		t.Fatalf("expected payload %q, got %q", payload, string(body))
	}
}

func TestReadRequestBody_ChainedEncodings(t *testing.T) {
	payload := `{"test":"chained compression"}`
	// First gzip, then zstd
	gz := gzipCompress(t, []byte(payload))
	zstdGz := zstdCompress(t, gz)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(zstdGz))
	req.Header.Set("Content-Encoding", "gzip, zstd")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	body, err := ReadRequestBody(c)
	if err != nil {
		t.Fatalf("ReadRequestBody chained failed: %v", err)
	}
	if string(body) != payload {
		t.Fatalf("expected payload %q, got %q", payload, string(body))
	}
}

func TestReadRequestBody_MaxBytesExceeded(t *testing.T) {
	payload := `{"data":"` + string(bytes.Repeat([]byte("A"), 10000)) + `"}`
	compressed := gzipCompress(t, []byte(payload))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(compressed))
	req.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set(MaxRequestBodyBytesContextKey, int64(100))

	_, err := ReadRequestBody(c)
	if err == nil {
		t.Fatal("expected MaxBytesError when payload exceeds limit, got nil")
	}
	maxBytesErr, ok := err.(*http.MaxBytesError)
	if !ok || maxBytesErr.Limit != 100 {
		t.Fatalf("expected MaxBytesError with limit 100, got: %v", err)
	}
}

func TestReadRequestBody_UnsupportedEncoding(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte("dummy")))
	req.Header.Set("Content-Encoding", "snappy")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	_, err := ReadRequestBody(c)
	if err == nil {
		t.Fatal("expected error for unsupported encoding, got nil")
	}
}

func TestReadRequestBody_CorruptData(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte("not valid gzip data")))
	req.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	_, err := ReadRequestBody(c)
	if err == nil {
		t.Fatal("expected error for corrupted gzip data, got nil")
	}
}
