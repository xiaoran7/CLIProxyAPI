package traffichub

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	// ContextClientIP is the Gin context key where the resolved client IP is stored.
	ContextClientIP = "cpa_client_ip"
)

// ExtractClientIP extracts the real client IP from incoming HTTP request headers,
// prioritizing the first hop in X-Forwarded-For, then X-Real-IP, and falling back
// to RemoteAddr with port stripped.
func ExtractClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}

	// 1. Check X-Forwarded-For first hop
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				if clean := CleanIP(trimmed); clean != "" {
					return clean
				}
			}
		}
	}

	// 2. Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		trimmed := strings.TrimSpace(xri)
		if trimmed != "" {
			if clean := CleanIP(trimmed); clean != "" {
				return clean
			}
		}
	}

	// 3. Fallback to RemoteAddr
	if r.RemoteAddr != "" {
		if clean := CleanIP(r.RemoteAddr); clean != "" {
			return clean
		}
	}

	return ""
}

// CleanIP strips port numbers (for IPv4:port or [IPv6]:port) and brackets.
func CleanIP(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}

	// Try splitting host and port
	host, _, err := net.SplitHostPort(addr)
	if err == nil && host != "" {
		return strings.Trim(host, "[]")
	}

	// If no port, strip surrounding brackets for IPv6 literals
	clean := strings.Trim(addr, "[]")
	return strings.TrimSpace(clean)
}

// Middleware returns a Gin middleware that extracts and caches client IP in Gin context.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := ExtractClientIP(c.Request)
		if ip != "" {
			c.Set(ContextClientIP, ip)
		}
		c.Next()
	}
}

// GetClientIP retrieves the cached client IP from Gin context or extracts it on the fly.
func GetClientIP(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if val, exists := c.Get(ContextClientIP); exists {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	if c.Request != nil {
		ip := ExtractClientIP(c.Request)
		if ip != "" {
			c.Set(ContextClientIP, ip)
			return ip
		}
	}
	return ""
}
