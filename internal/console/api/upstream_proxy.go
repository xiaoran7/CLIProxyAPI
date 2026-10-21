package api

import (
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/managementasset"
	"golang.org/x/crypto/bcrypt"
)

const (
	GateAdminTokenCookie = "gate_admin_token"
	upstreamScriptTag    = `<script>
  (function() {
    function rewriteUrl(url) {
      if (!url) return url;
      if (typeof url !== 'string') {
        try { url = url.toString(); } catch (e) { return url; }
      }
      if (url.includes('/admin/upstream/') || url.includes('/console/upstream/')) return url;

      var prefix = window.location.pathname.startsWith('/console/upstream') ? '/console/upstream' : '/admin/upstream';

      // 1. Relative paths
      if (url.startsWith('/v0/management') || url.startsWith('/v8/management') || url.startsWith('/v0/resource')) {
        return prefix + url;
      }
      if (url.startsWith('v0/management') || url.startsWith('v8/management') || url.startsWith('v0/resource')) {
        return prefix + '/' + url;
      }

      // 2. Absolute paths with Origin
      try {
        var parsed = new URL(url, window.location.href);
        if (parsed.pathname.startsWith('/v0/management') || parsed.pathname.startsWith('/v8/management') || parsed.pathname.startsWith('/v0/resource')) {
          return window.location.origin + prefix + parsed.pathname + parsed.search;
        }
      } catch (e) {}

      return url;
    }

    var originalFetch = window.fetch;
    window.fetch = function(input, init) {
      try {
        if (typeof input === 'string') {
          input = rewriteUrl(input);
        } else if (input instanceof URL) {
          input = new URL(rewriteUrl(input.toString()));
        } else if (input && typeof input.url === 'string') {
          var rewritten = rewriteUrl(input.url);
          if (rewritten !== input.url) {
            input = new Request(rewritten, input);
          }
        }
      } catch (e) {
        console.warn('[Gate-Proxy] fetch rewrite error:', e);
      }
      return originalFetch.call(this, input, init);
    };

    var originalOpen = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function(method, url) {
      var rest = Array.prototype.slice.call(arguments, 2);
      try {
        if (typeof url === 'string') {
          url = rewriteUrl(url);
        } else if (url && typeof url.toString === 'function') {
          url = rewriteUrl(url.toString());
        }
      } catch (e) {
        console.warn('[Gate-Proxy] XHR rewrite error:', e);
      }
      return originalOpen.apply(this, [method, url].concat(rest));
    };
  })();
</script>`
)

// RegisterUpstreamProxyRoutes mounts the authenticated upstream management proxy handlers on the router.
func RegisterUpstreamProxyRoutes(r *gin.Engine, cfg RouterConfig) {
	handler := HandleUpstreamProxy(cfg)
	r.Any("/admin/upstream", handler)
	r.Any("/admin/upstream/*action", handler)
	r.Any("/console/upstream", handler)
	r.Any("/console/upstream/*action", handler)
}

// HandleUpstreamProxy creates the Gin handler for proxying management requests into the native CPA subsystem.
func HandleUpstreamProxy(cfg RouterConfig) gin.HandlerFunc {
	host := cfg.CpaHost
	if strings.TrimSpace(host) == "" {
		host = "127.0.0.1"
	}
	port := cfg.CpaPort
	if port <= 0 {
		port = 8317
	}

	httpClient := &http.Client{
		Timeout: 60 * time.Second,
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// 1. Extract authentication candidate
		candidate := ""
		queryToken := strings.TrimSpace(c.Query("token"))
		if queryToken != "" {
			candidate = queryToken
		}

		if candidate == "" {
			if cookie, err := c.Cookie(GateAdminTokenCookie); err == nil && cookie != "" {
				candidate = strings.TrimSpace(cookie)
			}
		}

		if candidate == "" {
			authHeader := c.GetHeader("Authorization")
			if authHeader != "" {
				if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
					candidate = strings.TrimSpace(authHeader[7:])
				} else {
					candidate = strings.TrimSpace(authHeader)
				}
			}
		}

		if candidate == "" {
			candidate = strings.TrimSpace(c.GetHeader("x-admin-key"))
		}
		if candidate == "" {
			candidate = strings.TrimSpace(c.GetHeader("x-management-key"))
		}

		isAuthenticated := verifySecretCandidate(candidate, cfg.AdminSecret, cfg.CpaSecret)

		// 2. Query token exchange: if valid query token, set HttpOnly cookie and redirect to strip token
		if queryToken != "" && isAuthenticated {
			q := c.Request.URL.Query()
			q.Del("token")
			cleanURL := c.Request.URL.Path
			if len(q) > 0 {
				cleanURL += "?" + q.Encode()
			}

			http.SetCookie(c.Writer, &http.Cookie{
				Name:     GateAdminTokenCookie,
				Value:    queryToken,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   86400 * 30,
			})
			c.Header("Cache-Control", "no-store")
			c.Redirect(http.StatusFound, cleanURL)
			return
		}

		// 3. Unauthorized access interception
		if !isAuthenticated {
			accept := c.GetHeader("Accept")
			isHTML := strings.Contains(accept, "text/html") ||
				path == "/admin/upstream" || path == "/admin/upstream/" ||
				path == "/console/upstream" || path == "/console/upstream/" ||
				strings.HasSuffix(path, ".html")

			if isHTML {
				c.Header("Cache-Control", "no-store")
				c.Redirect(http.StatusFound, "/console?error=auth_required")
				return
			}

			c.Header("Cache-Control", "no-store")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Gate admin authentication required to access upstream management",
					"code":    401,
				},
			})
			return
		}

		// Ensure cookie is established for subsequent un-head requests if authenticated via header
		if candidate != "" && isAuthenticated {
			if _, err := c.Cookie(GateAdminTokenCookie); err != nil {
				http.SetCookie(c.Writer, &http.Cookie{
					Name:     GateAdminTokenCookie,
					Value:    candidate,
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
					MaxAge:   86400 * 30,
				})
			}
		}

		// 4. Resolve target path relative to upstream prefix
		targetPath := path
		if strings.HasPrefix(targetPath, "/admin/upstream") {
			targetPath = strings.TrimPrefix(targetPath, "/admin/upstream")
		} else if strings.HasPrefix(targetPath, "/console/upstream") {
			targetPath = strings.TrimPrefix(targetPath, "/console/upstream")
		}

		if targetPath == "" || targetPath == "/" {
			targetPath = "/management.html"
		}

		// 5. Direct static management.html delivery with script injection
		if targetPath == "/management.html" || targetPath == "management.html" {
			var htmlBytes []byte
			if data, err := os.ReadFile("static/management.html"); err == nil && len(data) > 0 {
				htmlBytes = data
			} else {
				htmlBytes = managementasset.EmbeddedManagementHTML()
			}

			if len(htmlBytes) == 0 {
				c.JSON(http.StatusNotFound, gin.H{"error": "management.html asset not found"})
				return
			}

			htmlStr := string(htmlBytes)
			if strings.Contains(htmlStr, "<head>") {
				htmlStr = strings.Replace(htmlStr, "<head>", "<head>"+upstreamScriptTag, 1)
			} else {
				htmlStr = upstreamScriptTag + htmlStr
			}

			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Header("Cache-Control", "no-store")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(htmlStr))
			return
		}

		// 6. Upstream API reverse proxy with loopback credential injection
		targetURL := fmt.Sprintf("http://%s:%d%s", host, port, targetPath)
		if rawQuery := c.Request.URL.RawQuery; rawQuery != "" {
			targetURL += "?" + rawQuery
		}

		proxyReq, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL, c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error": gin.H{
					"message": "failed to create upstream request: " + err.Error(),
					"code":    502,
				},
			})
			return
		}

		// Copy request headers, stripping hop-by-hop and cookie headers
		for k, vv := range c.Request.Header {
			lower := strings.ToLower(k)
			if lower == "cookie" || lower == "host" || lower == "connection" || lower == "upgrade" {
				continue
			}
			for _, v := range vv {
				proxyReq.Header.Add(k, v)
			}
		}

		// Provide loopback authorization token to CPA
		effectiveSecret := candidate
		if effectiveSecret == "" {
			if !strings.HasPrefix(cfg.CpaSecret, "$2a$") && !strings.HasPrefix(cfg.CpaSecret, "$2b$") && !strings.HasPrefix(cfg.CpaSecret, "$2y$") {
				effectiveSecret = cfg.CpaSecret
			}
		}
		if effectiveSecret == "" {
			if !strings.HasPrefix(cfg.AdminSecret, "$2a$") && !strings.HasPrefix(cfg.AdminSecret, "$2b$") && !strings.HasPrefix(cfg.AdminSecret, "$2y$") {
				effectiveSecret = cfg.AdminSecret
			}
		}
		if effectiveSecret != "" {
			proxyReq.Header.Set("Authorization", "Bearer "+effectiveSecret)
		}

		resp, err := httpClient.Do(proxyReq)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error": gin.H{
					"message": "upstream CPA management unreachable: " + err.Error(),
					"code":    502,
				},
			})
			return
		}
		defer func() { _ = resp.Body.Close() }()

		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, "text/html") {
			bodyBytes, errRead := io.ReadAll(resp.Body)
			if errRead != nil {
				c.Status(resp.StatusCode)
				return
			}
			htmlStr := string(bodyBytes)
			if strings.Contains(htmlStr, "<head>") {
				htmlStr = strings.Replace(htmlStr, "<head>", "<head>"+upstreamScriptTag, 1)
			} else {
				htmlStr = upstreamScriptTag + htmlStr
			}
			for k, vv := range resp.Header {
				lower := strings.ToLower(k)
				if lower == "content-length" || lower == "content-security-policy" {
					continue
				}
				for _, v := range vv {
					c.Writer.Header().Add(k, v)
				}
			}
			c.Header("Cache-Control", "no-store")
			c.Data(resp.StatusCode, "text/html; charset=utf-8", []byte(htmlStr))
			return
		}

		for k, vv := range resp.Header {
			for _, v := range vv {
				c.Writer.Header().Add(k, v)
			}
		}
		c.Status(resp.StatusCode)
		_, _ = io.Copy(c.Writer, resp.Body)
	}
}

func verifySecretCandidate(candidate, adminSecret, cpaSecret string) bool {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false
	}

	match := func(expected string) bool {
		expected = strings.TrimSpace(expected)
		if expected == "" {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(expected)) == 1 {
			return true
		}
		if strings.HasPrefix(expected, "$2a$") || strings.HasPrefix(expected, "$2b$") || strings.HasPrefix(expected, "$2y$") {
			if err := bcrypt.CompareHashAndPassword([]byte(expected), []byte(candidate)); err == nil {
				return true
			}
		}
		return false
	}

	return match(adminSecret) || match(cpaSecret)
}
