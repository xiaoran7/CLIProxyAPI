package middleware

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func decompressGzip(t *testing.T, data []byte) string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer zr.Close()
	decompressed, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("failed to decompress gzip: %v", err)
	}
	return string(decompressed)
}

func TestSelectiveGzip_LargeJSON_Compressed(t *testing.T) {
	router := gin.New()
	router.Use(SelectiveGzipMiddleware())

	largePayload := fmt.Sprintf(`{"data":"%s"}`, strings.Repeat("hello world ", 200)) // > 2KB
	router.GET("/test-large", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, largePayload)
	})

	req := httptest.NewRequest(http.MethodGet, "/test-large", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected Content-Encoding: gzip, got %q", w.Header().Get("Content-Encoding"))
	}
	if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("expected Vary: Accept-Encoding, got %q", w.Header().Get("Vary"))
	}

	decompressed := decompressGzip(t, w.Body.Bytes())
	if decompressed != largePayload {
		t.Fatalf("decompressed payload mismatch")
	}
}

func TestSelectiveGzip_SmallJSON_NotCompressed(t *testing.T) {
	router := gin.New()
	router.Use(SelectiveGzipMiddleware())

	smallPayload := `{"status":"ok"}`
	router.GET("/test-small", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, smallPayload)
	})

	req := httptest.NewRequest(http.MethodGet, "/test-small", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("expected no Content-Encoding for small payload, got %q", w.Header().Get("Content-Encoding"))
	}
	if w.Body.String() != smallPayload {
		t.Fatalf("expected body %q, got %q", smallPayload, w.Body.String())
	}
}

func TestSelectiveGzip_NoAcceptGzip_NotCompressed(t *testing.T) {
	router := gin.New()
	router.Use(SelectiveGzipMiddleware())

	largePayload := fmt.Sprintf(`{"data":"%s"}`, strings.Repeat("hello world ", 200))
	router.GET("/test-no-gzip", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, largePayload)
	})

	req := httptest.NewRequest(http.MethodGet, "/test-no-gzip", nil)
	// No Accept-Encoding header
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("expected no Content-Encoding, got %q", w.Header().Get("Content-Encoding"))
	}
	if w.Body.String() != largePayload {
		t.Fatalf("body mismatch")
	}
}

func TestSelectiveGzip_SSEStreaming_Bypassed(t *testing.T) {
	router := gin.New()
	router.Use(SelectiveGzipMiddleware())

	router.GET("/test-sse", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		// Write multiple chunks with flusher
		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		c.Writer.Write([]byte("data: chunk1\n\n"))
		flusher.Flush()
		c.Writer.Write([]byte("data: chunk2\n\n"))
		flusher.Flush()
	})

	req := httptest.NewRequest(http.MethodGet, "/test-sse", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	// SSE MUST NOT be compressed with gzip
	if w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("expected no Content-Encoding for SSE stream, got %q", w.Header().Get("Content-Encoding"))
	}

	expected := "data: chunk1\n\ndata: chunk2\n\n"
	if w.Body.String() != expected {
		t.Fatalf("expected SSE chunks %q, got %q", expected, w.Body.String())
	}
}

func TestSelectiveGzip_EarlyFlush_Bypassed(t *testing.T) {
	router := gin.New()
	router.Use(SelectiveGzipMiddleware())

	router.GET("/test-flush", func(c *gin.Context) {
		// Even without text/event-stream header, an early Flush() signals streaming
		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		c.Writer.Write([]byte("chunk A"))
		flusher.Flush()
		c.Writer.Write([]byte("chunk B"))
	})

	req := httptest.NewRequest(http.MethodGet, "/test-flush", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("expected no Content-Encoding when Flush() is called, got %q", w.Header().Get("Content-Encoding"))
	}
	if w.Body.String() != "chunk Achunk B" {
		t.Fatalf("expected body %q, got %q", "chunk Achunk B", w.Body.String())
	}
}

func TestSelectiveGzip_AlreadyCompressed_NoDoubleCompression(t *testing.T) {
	router := gin.New()
	router.Use(SelectiveGzipMiddleware())

	router.GET("/test-already-gzip", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.Header("Content-Encoding", "gzip")
		// Simulate pre-compressed upstream response
		c.String(http.StatusOK, "pre-compressed-raw-bytes")
	})

	req := httptest.NewRequest(http.MethodGet, "/test-already-gzip", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Body.String() != "pre-compressed-raw-bytes" {
		t.Fatalf("expected body untouched, got %q", w.Body.String())
	}
}

func TestSelectiveGzip_NoContent_Uncompressed(t *testing.T) {
	router := gin.New()
	router.Use(SelectiveGzipMiddleware())

	router.DELETE("/test-204", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodDelete, "/test-204", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", w.Code)
	}
	if w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("expected no Content-Encoding for 204, got %q", w.Header().Get("Content-Encoding"))
	}
	if w.Body.Len() != 0 {
		t.Fatalf("expected empty body for 204, got %d bytes", w.Body.Len())
	}
}
