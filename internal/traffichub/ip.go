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

// ExtractClientIP extracts the authentic client IP from HTTP headers, prioritizing:
// 1. CF-Connecting-IP (Cloudflare edge verified visitor IP)
// 2. True-Client-IP (Cloudflare Enterprise / Akamai / Fastly)
// 3. X-Forwarded-For (leftmost valid public IP, fallback to leftmost valid IP)
// 4. X-Real-IP
// 5. RemoteAddr (port stripped)
func ExtractClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}

	// 1. Check CF-Connecting-IP (Cloudflare authoritative visitor IP)
	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		if clean := CleanIP(cfIP); clean != "" && isValidIP(clean) {
			return clean
		}
	}

	// 2. Check True-Client-IP (Cloudflare Enterprise / Akamai)
	if trueIP := r.Header.Get("True-Client-IP"); trueIP != "" {
		if clean := CleanIP(trueIP); clean != "" && isValidIP(clean) {
			return clean
		}
	}

	// 3. Check X-Forwarded-For: parse left-to-right, prioritize first valid public IP
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		var firstValid string
		for _, part := range parts {
			clean := CleanIP(part)
			if clean != "" && isValidIP(clean) {
				if firstValid == "" {
					firstValid = clean
				}
				if isPublicIP(clean) {
					return clean
				}
			}
		}
		if firstValid != "" {
			return firstValid
		}
	}

	// 4. Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if clean := CleanIP(xri); clean != "" && isValidIP(clean) {
			return clean
		}
	}

	// 5. Fallback to RemoteAddr
	if r.RemoteAddr != "" {
		if clean := CleanIP(r.RemoteAddr); clean != "" && isValidIP(clean) {
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

	host, _, err := net.SplitHostPort(addr)
	if err == nil && host != "" {
		return strings.Trim(host, "[]")
	}

	clean := strings.Trim(addr, "[]")
	return strings.TrimSpace(clean)
}

func isValidIP(addr string) bool {
	ip := net.ParseIP(addr)
	return ip != nil
}

func isPublicIP(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		// RFC 1918 private IPv4 ranges
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return false
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return false
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return false
		}
		// CGNAT 100.64.0.0/10 (e.g. Tailscale / carrier grade NAT)
		if ip4[0] == 100 && (ip4[1]&0xc0) == 64 {
			return false
		}
		return true
	}
	// IPv6 ULA (fc00::/7)
	if len(ip) == 16 && (ip[0]&0xfe) == 0xfc {
		return false
	}
	return true
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
