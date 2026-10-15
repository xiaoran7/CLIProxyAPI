package middleware

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

const (
	// DefaultMinCompressSize is the minimum payload size (in bytes) eligible for compression.
	// Small payloads (< 1KB) often expand under compression and waste CPU cycles.
	DefaultMinCompressSize = 1024
)

var gzipWriterPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		return w
	},
}

func compressGzip(data []byte) ([]byte, error) {
	gw, ok := gzipWriterPool.Get().(*gzip.Writer)
	if !ok || gw == nil {
		var err error
		gw, err = gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		if err != nil {
			return nil, err
		}
	}
	defer gzipWriterPool.Put(gw)

	var buf bytes.Buffer
	buf.Grow(len(data) / 3)
	gw.Reset(&buf)

	if _, err := gw.Write(data); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func clientAcceptsGzip(req *http.Request) bool {
	if req == nil || req.Method == http.MethodHead {
		return false
	}
	if req.Header.Get("Upgrade") != "" || strings.Contains(req.Header.Get("Accept"), "text/event-stream") {
		return false
	}
	ae := strings.ToLower(req.Header.Get("Accept-Encoding"))
	for _, part := range strings.Split(ae, ",") {
		if strings.TrimSpace(part) == "gzip" {
			return true
		}
	}
	return false
}

func isCompressibleContentType(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct == "" {
		return true
	}
	if strings.Contains(ct, "text/event-stream") ||
		strings.Contains(ct, "application/octet-stream") ||
		strings.Contains(ct, "image/") ||
		strings.Contains(ct, "audio/") ||
		strings.Contains(ct, "video/") ||
		strings.Contains(ct, "application/zip") ||
		strings.Contains(ct, "application/gzip") ||
		strings.Contains(ct, "application/zstd") {
		return false
	}
	return strings.Contains(ct, "application/json") ||
		strings.Contains(ct, "application/xml") ||
		strings.Contains(ct, "application/javascript") ||
		strings.Contains(ct, "text/")
}

// selectiveGzipWriter intercepts non-streaming responses to compress them with Gzip
// while strictly bypassing streaming SSE, WebSockets, and small payloads.
type selectiveGzipWriter struct {
	gin.ResponseWriter
	buf             bytes.Buffer
	headerWritten   bool
	statusCode      int
	streamingBypass bool
	minSize         int
}

func newSelectiveGzipWriter(w gin.ResponseWriter, minSize int) *selectiveGzipWriter {
	return &selectiveGzipWriter{
		ResponseWriter: w,
		minSize:        minSize,
	}
}

func (w *selectiveGzipWriter) Status() int {
	if w.statusCode > 0 {
		return w.statusCode
	}
	return w.ResponseWriter.Status()
}

func (w *selectiveGzipWriter) Written() bool {
	return w.headerWritten || w.ResponseWriter.Written()
}

func (w *selectiveGzipWriter) Size() int {
	if !w.streamingBypass && !w.headerWritten {
		return w.buf.Len()
	}
	return w.ResponseWriter.Size()
}

func (w *selectiveGzipWriter) WriteHeaderNow() {
	if !w.headerWritten && w.streamingBypass {
		w.headerWritten = true
		status := w.statusCode
		if status == 0 {
			status = http.StatusOK
		}
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *selectiveGzipWriter) WriteHeader(code int) {
	w.statusCode = code
	if w.streamingBypass {
		w.ResponseWriter.WriteHeader(code)
		w.headerWritten = true
	}
}

func (w *selectiveGzipWriter) enableStreamingBypass() {
	if w.streamingBypass {
		return
	}
	w.streamingBypass = true
	if !w.headerWritten {
		status := w.statusCode
		if status == 0 {
			status = http.StatusOK
		}
		w.ResponseWriter.WriteHeader(status)
		w.headerWritten = true
	}
	if w.buf.Len() > 0 {
		_, _ = w.ResponseWriter.Write(w.buf.Bytes())
		w.buf.Reset()
	}
}

func (w *selectiveGzipWriter) Write(data []byte) (int, error) {
	if w.streamingBypass {
		return w.ResponseWriter.Write(data)
	}

	ct := w.Header().Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") || strings.Contains(ct, "application/octet-stream") {
		w.enableStreamingBypass()
		return w.ResponseWriter.Write(data)
	}

	return w.buf.Write(data)
}

func (w *selectiveGzipWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *selectiveGzipWriter) Flush() {
	if !w.streamingBypass {
		w.enableStreamingBypass()
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *selectiveGzipWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("underlying ResponseWriter does not implement http.Hijacker")
}

func (w *selectiveGzipWriter) finalize() {
	if w.streamingBypass {
		return
	}

	status := w.statusCode
	if status == 0 {
		status = http.StatusOK
	}

	if w.buf.Len() == 0 {
		if !w.headerWritten {
			w.ResponseWriter.WriteHeader(status)
			w.headerWritten = true
		}
		return
	}

	// Check if eligible for compression
	eligibleStatus := status != http.StatusNoContent &&
		status != http.StatusNotModified &&
		status != http.StatusSwitchingProtocols

	eligibleEncoding := w.Header().Get("Content-Encoding") == ""
	eligibleType := isCompressibleContentType(w.Header().Get("Content-Type"))
	eligibleSize := w.buf.Len() >= w.minSize

	if eligibleStatus && eligibleEncoding && eligibleType && eligibleSize {
		compressed, err := compressGzip(w.buf.Bytes())
		if err == nil && len(compressed) < w.buf.Len() {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Add("Vary", "Accept-Encoding")
			w.Header().Set("Content-Length", strconv.Itoa(len(compressed)))
			if !w.headerWritten {
				w.ResponseWriter.WriteHeader(status)
				w.headerWritten = true
			}
			_, _ = w.ResponseWriter.Write(compressed)
			return
		}
	}

	// Fallback to uncompressed write
	if !w.headerWritten {
		w.ResponseWriter.WriteHeader(status)
		w.headerWritten = true
	}
	_, _ = w.ResponseWriter.Write(w.buf.Bytes())
}

// SelectiveGzipMiddleware returns a Gin middleware that selectively applies Gzip
// compression to eligible non-streaming responses while leaving streaming (SSE)
// and WebSockets unbuffered and uncompressed.
func SelectiveGzipMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !clientAcceptsGzip(c.Request) {
			c.Next()
			return
		}

		writer := newSelectiveGzipWriter(c.Writer, DefaultMinCompressSize)
		c.Writer = writer

		c.Next()

		writer.finalize()
	}
}
