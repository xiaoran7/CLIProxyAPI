package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
	"golang.org/x/crypto/bcrypt"
)

func TestUpstreamProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSecret := "test-admin-secret-999"
	cpaSecret := "test-cpa-secret-888"

	// Setup a mock CPA upstream server on loopback
	upstreamHit := false
	var upstreamAuthHeader string
	var upstreamPath string

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		upstreamAuthHeader = r.Header.Get("Authorization")
		upstreamPath = r.URL.Path
		if r.URL.RawQuery != "" {
			upstreamPath += "?" + r.URL.RawQuery
		}

		if r.URL.Path == "/v0/management/config" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"debug":false,"request-retry":3}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer upstreamServer.Close()

	// Parse upstream port
	var upstreamPort int
	parts := strings.Split(upstreamServer.URL, ":")
	if len(parts) >= 3 {
		_, _ = fmt.Sscanf(parts[2], "%d", &upstreamPort)
	}

	tmpDir, err := os.MkdirTemp("", "upstream_proxy_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer func() { _ = database.Close() }()

	eng, err := engine.NewEngine(database, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}
	defer eng.Stop()

	pricing, err := NewPricingEngine(database)
	if err != nil {
		t.Fatalf("failed to init pricing engine: %v", err)
	}

	providers, err := NewProviderRegistryEngine(database)
	if err != nil {
		t.Fatalf("failed to init providers engine: %v", err)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	cfg := RouterConfig{
		AdminSecret: adminSecret,
		CpaHost:     "127.0.0.1",
		CpaPort:     upstreamPort,
		CpaSecret:   cpaSecret,
	}

	RegisterRoutes(r, eng, cfg, pricing, providers)

	// 1. Unauthenticated HTML request should 302 redirect to /console?error=auth_required
	t.Run("Unauthenticated HTML redirect", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/upstream/management.html", nil)
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusFound {
			t.Fatalf("expected 302 Found, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if loc != "/console?error=auth_required" {
			t.Fatalf("expected Location /console?error=auth_required, got %q", loc)
		}
	})

	// 2. Unauthenticated API request should return 401 JSON
	t.Run("Unauthenticated API returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/upstream/v0/management/config", nil)
		req.Header.Set("Accept", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "Gate admin authentication required") {
			t.Fatalf("expected error message in response body, got %s", rr.Body.String())
		}
	})

	// 3. Query token exchange: sets cookie and 302 redirects to clean URL
	t.Run("Query token exchange", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/upstream/management.html?token="+adminSecret, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusFound {
			t.Fatalf("expected 302 Found, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if loc != "/admin/upstream/management.html" {
			t.Fatalf("expected Location /admin/upstream/management.html, got %q", loc)
		}
		cookie := rr.Header().Get("Set-Cookie")
		if !strings.Contains(cookie, GateAdminTokenCookie+"="+adminSecret) {
			t.Fatalf("expected Set-Cookie to contain token, got %q", cookie)
		}
		if !strings.Contains(cookie, "HttpOnly") {
			t.Fatalf("expected Set-Cookie to be HttpOnly, got %q", cookie)
		}
	})

	// 4. Authenticated HTML delivery with script injection
	t.Run("Authenticated HTML delivery with injection", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/upstream/management.html", nil)
		req.AddCookie(&http.Cookie{
			Name:  GateAdminTokenCookie,
			Value: adminSecret,
		})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rr.Code)
		}
		contentType := rr.Header().Get("Content-Type")
		if !strings.Contains(contentType, "text/html") {
			t.Fatalf("expected text/html, got %q", contentType)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "rewriteUrl") {
			t.Fatalf("expected body to contain rewriteUrl script injection")
		}
		if !strings.Contains(body, "/admin/upstream") {
			t.Fatalf("expected body to contain /admin/upstream rewrite target")
		}
	})

	// 5. Console prefix alias /console/upstream
	t.Run("Console prefix alias /console/upstream", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/console/upstream/management.html", nil)
		req.AddCookie(&http.Cookie{
			Name:  GateAdminTokenCookie,
			Value: adminSecret,
		})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "rewriteUrl") {
			t.Fatalf("expected body to contain rewriteUrl")
		}
	})

	// 6. Upstream API proxying with loopback authorization injection
	t.Run("Upstream API proxying", func(t *testing.T) {
		upstreamHit = false
		req := httptest.NewRequest(http.MethodGet, "/admin/upstream/v0/management/config?foo=bar", nil)
		req.Header.Set("Authorization", "Bearer "+adminSecret)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rr.Code)
		}
		if !upstreamHit {
			t.Fatalf("expected upstream to receive request")
		}
		if upstreamPath != "/v0/management/config?foo=bar" {
			t.Fatalf("expected upstream path /v0/management/config?foo=bar, got %q", upstreamPath)
		}
		if upstreamAuthHeader != "Bearer "+adminSecret {
			t.Fatalf("expected upstream Authorization Bearer %s, got %q", adminSecret, upstreamAuthHeader)
		}
		if !strings.Contains(rr.Body.String(), `"request-retry":3`) {
			t.Fatalf("expected response body to contain upstream json, got %s", rr.Body.String())
		}
	})

	// 7. Bcrypt secret authentication
	t.Run("Bcrypt secret authentication", func(t *testing.T) {
		plainSecret := "bcrypt-pass-123456"
		hashed, errHash := bcrypt.GenerateFromPassword([]byte(plainSecret), bcrypt.DefaultCost)
		if errHash != nil {
			t.Fatalf("failed to hash secret: %v", errHash)
		}

		rBcrypt := gin.New()
		cfgBcrypt := RouterConfig{
			AdminSecret: string(hashed),
			CpaHost:     "127.0.0.1",
			CpaPort:     upstreamPort,
		}
		RegisterRoutes(rBcrypt, eng, cfgBcrypt, pricing, providers)

		req := httptest.NewRequest(http.MethodGet, "/admin/upstream/management.html?token="+plainSecret, nil)
		rr := httptest.NewRecorder()
		rBcrypt.ServeHTTP(rr, req)

		if rr.Code != http.StatusFound {
			t.Fatalf("expected 302 Found with bcrypt secret, got %d", rr.Code)
		}
		cookie := rr.Header().Get("Set-Cookie")
		if !strings.Contains(cookie, GateAdminTokenCookie+"="+plainSecret) {
			t.Fatalf("expected Set-Cookie to contain token, got %q", cookie)
		}
	})
}
