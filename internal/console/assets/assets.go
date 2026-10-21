package assets

import (
	"crypto/md5"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed console.html console-*.css console-*.js images/*
var embeddedAssets embed.FS

// FileETags caches MD5 hashes for static assets.
var fileETags = make(map[string]string)

func init() {
	// Precalculate ETag hashes for embedded files
	entries := []string{
		"console.html",
		"console-style.css",
		"console-i18n.js",
		"console-keys.js",
		"console-quotas.js",
		"console-stats.js",
		"console-logs.js",
		"console-cpa.js",
		"console-cpa.css",
		"console-overview.js",
		"console-overview.css",
	}

	for _, name := range entries {
		data, err := embeddedAssets.ReadFile(name)
		if err == nil {
			h := md5.Sum(data)
			fileETags[name] = `"` + hex.EncodeToString(h[:8]) + `"`
		}
	}
}

// FS returns the underlying embedded filesystem.
func FS() embed.FS {
	return embeddedAssets
}

// GetAsset reads a named asset from the embedded filesystem.
func GetAsset(name string) ([]byte, error) {
	return embeddedAssets.ReadFile(name)
}

// RegisterUI mounts all Console UI routes, static assets, and backward-compatible admin aliases.
func RegisterUI(r *gin.Engine) {
	// Root / console index handlers
	consoleIndexHandler := func(c *gin.Context) {
		serveEmbeddedFile(c, "console.html", "text/html; charset=utf-8", "no-cache, must-revalidate")
	}

	r.GET("/console", consoleIndexHandler)
	r.GET("/console/", consoleIndexHandler)
	r.GET("/console/index.html", consoleIndexHandler)
	r.GET("/console/console.html", consoleIndexHandler)

	// Legacy admin and member redirects
	r.GET("/admin", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/console")
	})
	r.GET("/admin/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/console")
	})
	r.GET("/member", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/console")
	})
	r.GET("/member/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/console")
	})

	// Static JS and CSS assets under /console/*
	assetFiles := map[string]string{
		"/console/console-style.css":    "console-style.css",
		"/console/style.css":            "console-style.css",
		"/console/console-i18n.js":      "console-i18n.js",
		"/console/i18n.js":              "console-i18n.js",
		"/console/console-keys.js":      "console-keys.js",
		"/console/keys.js":              "console-keys.js",
		"/console/console-quotas.js":    "console-quotas.js",
		"/console/quotas.js":            "console-quotas.js",
		"/console/console-stats.js":     "console-stats.js",
		"/console/stats.js":             "console-stats.js",
		"/console/console-logs.js":      "console-logs.js",
		"/console/logs.js":              "console-logs.js",
		"/console/console-cpa.js":       "console-cpa.js",
		"/console/cpa-management.js":    "console-cpa.js",
		"/console/console-cpa.css":      "console-cpa.css",
		"/console/cpa-management.css":   "console-cpa.css",
		"/console/console-overview.js":  "console-overview.js",
		"/console/overview.js":          "console-overview.js",
		"/console/console-overview.css": "console-overview.css",
		"/console/overview.css":         "console-overview.css",
	}

	for routePath, assetName := range assetFiles {
		name := assetName
		contentType := "text/javascript; charset=utf-8"
		if strings.HasSuffix(name, ".css") {
			contentType = "text/css; charset=utf-8"
		}
		r.GET(routePath, func(c *gin.Context) {
			serveEmbeddedFile(c, name, contentType, "no-cache, must-revalidate")
		})
	}

	// Legacy /admin/* static JS/CSS aliases for seamless backward compatibility
	legacyAliases := map[string]string{
		"/admin/i18n.js":            "console-i18n.js",
		"/admin/admin-i18n.js":      "console-i18n.js",
		"/admin/cpa-management.js":  "console-cpa.js",
		"/admin/admin-cpa.js":       "console-cpa.js",
		"/admin/cpa-management.css": "console-cpa.css",
		"/admin/admin-cpa.css":      "console-cpa.css",
		"/admin/overview.js":        "console-overview.js",
		"/admin/admin-overview.js":  "console-overview.js",
		"/admin/overview.css":       "console-overview.css",
		"/admin/admin-overview.css": "console-overview.css",
		"/admin/style.css":          "console-style.css",
		"/admin/admin-style.css":    "console-style.css",
		"/admin/quotas.js":          "console-quotas.js",
		"/admin/admin-quotas.js":    "console-quotas.js",
		"/admin/keys.js":            "console-keys.js",
		"/admin/admin-keys.js":      "console-keys.js",
		"/admin/stats.js":           "console-stats.js",
		"/admin/admin-stats.js":     "console-stats.js",
		"/admin/logs.js":            "console-logs.js",
		"/admin/admin-logs.js":      "console-logs.js",
	}

	for routePath, assetName := range legacyAliases {
		name := assetName
		contentType := "text/javascript; charset=utf-8"
		if strings.HasSuffix(name, ".css") {
			contentType = "text/css; charset=utf-8"
		}
		r.GET(routePath, func(c *gin.Context) {
			serveEmbeddedFile(c, name, contentType, "no-cache, must-revalidate")
		})
	}

	// Image assets handler for /console/assets/* and /admin/assets/*
	imageHandler := func(c *gin.Context) {
		filename := c.Param("filepath")
		filename = strings.TrimPrefix(filename, "/")
		fullPath := "images/" + filename

		data, err := embeddedAssets.ReadFile(fullPath)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Asset not found"})
			return
		}

		etag := fileETags[fullPath]
		if etag == "" {
			h := md5.Sum(data)
			etag = `"` + hex.EncodeToString(h[:8]) + `"`
			fileETags[fullPath] = etag
		}

		if match := c.GetHeader("If-None-Match"); match != "" && (match == etag || match == "W/"+etag || match == "*") {
			c.Status(http.StatusNotModified)
			return
		}

		ctype := mime.TypeByExtension(filepath.Ext(filename))
		if ctype == "" {
			switch {
			case strings.HasSuffix(filename, ".webp"):
				ctype = "image/webp"
			case strings.HasSuffix(filename, ".svg"):
				ctype = "image/svg+xml"
			case strings.HasSuffix(filename, ".png"):
				ctype = "image/png"
			default:
				ctype = "application/octet-stream"
			}
		}

		c.Header("ETag", etag)
		c.Header("Cache-Control", "public, max-age=604800, immutable")
		c.Header("Content-Type", ctype)
		c.Data(http.StatusOK, ctype, data)
	}

	r.GET("/console/assets/*filepath", imageHandler)
	r.GET("/admin/assets/*filepath", imageHandler)
}

func serveEmbeddedFile(c *gin.Context, assetName, contentType, cacheControl string) {
	etag := fileETags[assetName]
	if etag != "" {
		if match := c.GetHeader("If-None-Match"); match != "" && (match == etag || match == "W/"+etag || match == "*") {
			c.Status(http.StatusNotModified)
			return
		}
	}

	data, err := embeddedAssets.ReadFile(assetName)
	if err != nil {
		c.String(http.StatusNotFound, fmt.Sprintf("Asset %s not found: %v", assetName, err))
		return
	}

	if etag == "" {
		h := md5.Sum(data)
		etag = `"` + hex.EncodeToString(h[:8]) + `"`
		fileETags[assetName] = etag
	}

	c.Header("ETag", etag)
	c.Header("Cache-Control", cacheControl)
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, contentType, data)
}

// ServeAssetWriter provides direct io.Writer output.
func ServeAssetWriter(w io.Writer, assetName string) error {
	data, err := embeddedAssets.ReadFile(assetName)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
