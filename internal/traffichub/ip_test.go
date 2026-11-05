package traffichub

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expectedIP string
	}{
		{
			name: "Cloudflare CF-Connecting-IP takes highest priority",
			headers: map[string]string{
				"CF-Connecting-IP": "119.39.133.76",
				"True-Client-IP":   "10.0.0.1",
				"X-Forwarded-For":  "172.18.0.1, 104.21.45.67",
				"X-Real-IP":        "172.18.0.1",
			},
			remoteAddr: "172.18.0.1:54321",
			expectedIP: "119.39.133.76",
		},
		{
			name: "True-Client-IP priority when CF-Connecting-IP missing",
			headers: map[string]string{
				"True-Client-IP":  "198.51.100.99",
				"X-Forwarded-For": "10.0.0.1, 198.51.100.99",
				"X-Real-IP":       "10.0.0.1",
			},
			remoteAddr: "10.0.0.1:12345",
			expectedIP: "198.51.100.99",
		},
		{
			name: "X-Forwarded-For single IP",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.195",
			},
			remoteAddr: "192.168.1.1:12345",
			expectedIP: "203.0.113.195",
		},
		{
			name: "X-Forwarded-For multi-hop takes first public hop",
			headers: map[string]string{
				"X-Forwarded-For": "10.0.0.1, 203.0.113.195, 70.41.3.18",
			},
			remoteAddr: "192.168.1.1:12345",
			expectedIP: "203.0.113.195",
		},
		{
			name: "X-Forwarded-For with port in hop",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.195:8080, 70.41.3.18",
			},
			remoteAddr: "192.168.1.1:12345",
			expectedIP: "203.0.113.195",
		},
		{
			name: "X-Real-IP fallback when X-Forwarded-For is empty",
			headers: map[string]string{
				"X-Real-IP": "198.51.100.42",
			},
			remoteAddr: "192.168.1.1:12345",
			expectedIP: "198.51.100.42",
		},
		{
			name:       "RemoteAddr fallback with IPv4 port stripped",
			headers:    map[string]string{},
			remoteAddr: "192.0.2.1:54321",
			expectedIP: "192.0.2.1",
		},
		{
			name:       "RemoteAddr IPv6 with port stripped",
			headers:    map[string]string{},
			remoteAddr: "[2001:db8::1]:8443",
			expectedIP: "2001:db8::1",
		},
		{
			name:       "RemoteAddr bare IPv6 without port",
			headers:    map[string]string{},
			remoteAddr: "2001:db8::1",
			expectedIP: "2001:db8::1",
		},
		{
			name:       "Nil request",
			headers:    nil,
			remoteAddr: "",
			expectedIP: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.name != "Nil request" {
				req = httptest.NewRequest(http.MethodGet, "http://example.com/v1/models", nil)
				req.RemoteAddr = tc.remoteAddr
				for k, v := range tc.headers {
					req.Header.Set(k, v)
				}
			}

			actual := ExtractClientIP(req)
			if actual != tc.expectedIP {
				t.Fatalf("expected IP %q, got %q", tc.expectedIP, actual)
			}
		})
	}
}

func TestMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware())

	var capturedIP string
	r.GET("/test", func(c *gin.Context) {
		capturedIP = GetClientIP(c)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("CF-Connecting-IP", "119.39.133.76")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if capturedIP != "119.39.133.76" {
		t.Fatalf("expected captured IP '119.39.133.76', got %q", capturedIP)
	}
}
