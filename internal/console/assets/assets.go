package assets

import (
	"crypto/md5"
	"embed"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed console.html console-*.css console-*.js console-*.html images/*
var embeddedAssets embed.FS

// FileETags caches MD5 hashes for static assets.
var fileETags = make(map[string]string)

// BuildVersion is injected at link time via
// -ldflags "-X '.../assets.BuildVersion=vX.Y.Z'".
// It drives the cache-busting query string on every Console asset URL. Without
// this the version was hardcoded in console.html across ~37 references and a
// release that forgot to bump it left browsers serving stale CSS/JS from cache.
var BuildVersion = "dev"

// assetVersionPattern matches the cache-busting query on asset references,
// e.g. "?v=v8.0.63". The version inside is rewritten at serve time.
var assetVersionPattern = regexp.MustCompile(`\?v=[A-Za-z0-9._-]+`)

// assetCacheVersion returns the version used for asset cache busting.
func assetCacheVersion() string {
	v := strings.TrimSpace(BuildVersion)
	if v == "" || v == "dev" {
		return "dev"
	}
	return v
}

// rewriteAssetVersions replaces every "?v=..." occurrence with the build
// version so that each release produces distinct asset URLs.
func rewriteAssetVersions(data []byte) []byte {
	v := assetCacheVersion()
	if v == "dev" {
		return data
	}
	return assetVersionPattern.ReplaceAll(data, []byte("?v="+v))
}

// consolePartials lists the HTML partials that are injected into the console
// shell, grouped by the placeholder marker they replace. The order inside a
// group is the document order of the markup, so the assembled page is
// byte-identical to the single-file console.html this replaced.
//
// The partials exist so that console.html itself stays a small, navigable
// shell: every tab page and every modal owns one file instead of piling up
// in one multi-thousand-line document (AGENTS.md §2.8).
var consolePartials = []struct {
	marker string
	files  []string
}{
	{
		marker: "<!--CONSOLE_PAGES-->",
		files: []string{
			"console-page-quotas.html",
			"console-page-status.html",
			"console-page-upstream-stats.html",
			"console-page-overview.html",
			"console-page-keys.html",
			"console-page-traffic.html",
			"console-page-logs.html",
			"console-page-errors.html",
			"console-page-tos-bans.html",
			"console-page-validations.html",
		},
	},
	{
		marker: "<!--CONSOLE_MODALS-->",
		files: []string{
			"console-modal-validation.html",
			"console-modal-account.html",
			"console-modal-pricing.html",
			"console-modal-openrouter.html",
		},
	},
}

// assembleConsoleHTML injects the embedded HTML partials into the console
// shell. Every partial is trimmed of its trailing newline and the parts are
// joined with a single "\n": the newline that terminates the placeholder line
// in the shell becomes the separator between the last partial and the shell
// content that follows it, which reproduces the original markup exactly.
func assembleConsoleHTML(data []byte) ([]byte, error) {
	out := string(data)
	for _, group := range consolePartials {
		parts := make([]string, 0, len(group.files))
		for _, name := range group.files {
			raw, err := embeddedAssets.ReadFile(name)
			if err != nil {
				return nil, fmt.Errorf("console partial %s: %w", name, err)
			}
			parts = append(parts, strings.TrimRight(string(raw), "\n"))
		}
		if strings.Count(out, group.marker) != 1 {
			return nil, fmt.Errorf("console placeholder %s must appear exactly once", group.marker)
		}
		out = strings.Replace(out, group.marker, strings.Join(parts, "\n"), 1)
	}
	return []byte(out), nil
}

func init() {
	// Precalculate ETag hashes for embedded files
	entries := []string{
		"console.html",
		"console-base.css",
		"console-panels.css",
		"console-members.css",
		"console-tables.css",
		"console-modals.css",
		"console-feedback.css",
		"console-logs.css",
		"console-traffic.css",
		"console-theme-beszel.css",
		"console-theme-v239.css",
		"console-theme-v2310.css",
		"console-validations.css",
		"console-upstream-stats.css",
		"console-i18n.js",
		"console-keys.js",
		"console-quotas-priority.js",
		"console-quotas-validation.js",
		"console-quotas-fetch.js",
		"console-quotas-render.js",
		"console-quotas-workbuddy.js",
		"console-quotas-stats.js",
		"console-stats.js",
		"console-logs.js",
		"console-traffic.js",
		"console-tos-bans.js",
		"console-cpa.js",
		"console-cpa.css",
		"console-overview.js",
		"console-overview.css",
		"console-state.js",
		"console-nav.js",
		"console-auth.js",
		"console-format.js",
		"console-polling.js",
		"console-member.js",
		"console-refresh.js",
		"console-init.js",
		"console-panels-quota.css",
		"console-panels-compact.css",
		"console-panels-status.css",
		"console-i18n-dict-common.js",
		"console-i18n-dict-stats.js",
		"console-i18n-dict-quotas.js",
		"console-i18n-dict-keys.js",
		"console-i18n-dict-logs.js",
		"console-quotas-meter-single.js",
		"console-quotas-meter-multi.js",
		"console-quotas-cards-compact.js",
		"console-quotas-cards-full.js",
		"console-keys-modals.js",
		"console-keys-picker.js",
		"console-keys-pricing.js",
		"console-logs-data.js",
		"console-logs-errors.js",
		"console-traffic-icons.js",
		"console-traffic-topology.js",
		"console-overview-charts.js",
	}

	for _, name := range entries {
		data, err := embeddedAssets.ReadFile(name)
		if err == nil {
			h := md5.Sum(data)
			fileETags[name] = `"` + hex.EncodeToString(h[:8]) + `"`
		}
	}
}

// GetAsset reads a named asset from the embedded filesystem.
func GetAsset(name string) ([]byte, error) {
	return embeddedAssets.ReadFile(name)
}

// RegisterUI mounts all Console UI routes, static assets, and backward-compatible admin aliases.
func RegisterUI(r *gin.Engine) {
	registerGetHead := func(path string, handler gin.HandlerFunc) {
		r.GET(path, handler)
		r.HEAD(path, handler)
	}

	// Root / console index handlers
	consoleIndexHandler := func(c *gin.Context) {
		// The index HTML is served from a version-less URL (/console/), so it must
		// never reach the browser disk cache: a stale copy would keep pinning the
		// old "?v=" asset URLs and make a deploy look like it never landed.
		// "no-store" is the only directive Chrome honours for that;
		// "no-cache, must-revalidate" still permits reusing a local copy.
		serveEmbeddedFile(c, "console.html", "text/html; charset=utf-8", "no-store, no-cache, must-revalidate, max-age=0")
	}

	registerGetHead("/console", consoleIndexHandler)
	registerGetHead("/console/", consoleIndexHandler)
	registerGetHead("/console/index.html", consoleIndexHandler)
	registerGetHead("/console/console.html", consoleIndexHandler)

	// Legacy admin and member redirects
	adminRedirectHandler := func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/console")
	}
	registerGetHead("/admin", adminRedirectHandler)
	registerGetHead("/admin/", adminRedirectHandler)

	memberRedirectHandler := func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/console")
	}
	registerGetHead("/member", memberRedirectHandler)
	registerGetHead("/member/", memberRedirectHandler)

	// Static JS and CSS assets under /console/*
	assetFiles := map[string]string{
		"/console/console-base.css":                "console-base.css",
		"/console/console-panels.css":              "console-panels.css",
		"/console/console-members.css":             "console-members.css",
		"/console/console-tables.css":              "console-tables.css",
		"/console/console-modals.css":              "console-modals.css",
		"/console/console-feedback.css":            "console-feedback.css",
		"/console/console-logs.css":                "console-logs.css",
		"/console/console-theme-beszel.css":        "console-theme-beszel.css",
		"/console/console-theme-v239.css":          "console-theme-v239.css",
		"/console/console-theme-v2310.css":         "console-theme-v2310.css",
		"/console/console-validations.css":         "console-validations.css",
		"/console/console-upstream-stats.css":      "console-upstream-stats.css",
		"/console/style.css":                       "console-base.css",
		"/console/console-i18n.js":                 "console-i18n.js",
		"/console/i18n.js":                         "console-i18n.js",
		"/console/console-keys.js":                 "console-keys.js",
		"/console/keys.js":                         "console-keys.js",
		"/console/console-quotas-priority.js":      "console-quotas-priority.js",
		"/console/console-quotas-validation.js":    "console-quotas-validation.js",
		"/console/console-quotas-fetch.js":         "console-quotas-fetch.js",
		"/console/console-quotas-render.js":        "console-quotas-render.js",
		"/console/console-quotas-workbuddy.js":     "console-quotas-workbuddy.js",
		"/console/console-quotas-stats.js":         "console-quotas-stats.js",
		"/console/console-stats.js":                "console-stats.js",
		"/console/stats.js":                        "console-stats.js",
		"/console/console-logs.js":                 "console-logs.js",
		"/console/logs.js":                         "console-logs.js",
		"/console/console-traffic.css":             "console-traffic.css",
		"/console/traffic.css":                     "console-traffic.css",
		"/console/console-traffic.js":              "console-traffic.js",
		"/console/traffic.js":                      "console-traffic.js",
		"/console/console-cpa.js":                  "console-cpa.js",
		"/console/cpa-management.js":               "console-cpa.js",
		"/console/console-cpa.css":                 "console-cpa.css",
		"/console/cpa-management.css":              "console-cpa.css",
		"/console/console-overview.js":             "console-overview.js",
		"/console/overview.js":                     "console-overview.js",
		"/console/console-overview.css":            "console-overview.css",
		"/console/overview.css":                    "console-overview.css",
		"/console/console-state.js":                "console-state.js",
		"/console/console-nav.js":                  "console-nav.js",
		"/console/console-auth.js":                 "console-auth.js",
		"/console/console-format.js":               "console-format.js",
		"/console/console-polling.js":              "console-polling.js",
		"/console/console-member.js":               "console-member.js",
		"/console/console-refresh.js":              "console-refresh.js",
		"/console/console-tos-bans.js":             "console-tos-bans.js",
		"/console/console-init.js":                 "console-init.js",
		"/console/console-panels-quota.css":        "console-panels-quota.css",
		"/console/console-panels-compact.css":      "console-panels-compact.css",
		"/console/console-panels-status.css":       "console-panels-status.css",
		"/console/console-i18n-dict-common.js":     "console-i18n-dict-common.js",
		"/console/console-i18n-dict-stats.js":      "console-i18n-dict-stats.js",
		"/console/console-i18n-dict-quotas.js":     "console-i18n-dict-quotas.js",
		"/console/console-i18n-dict-keys.js":       "console-i18n-dict-keys.js",
		"/console/console-i18n-dict-logs.js":       "console-i18n-dict-logs.js",
		"/console/console-quotas-meter-single.js":  "console-quotas-meter-single.js",
		"/console/console-quotas-meter-multi.js":   "console-quotas-meter-multi.js",
		"/console/console-quotas-cards-compact.js": "console-quotas-cards-compact.js",
		"/console/console-quotas-cards-full.js":    "console-quotas-cards-full.js",
		"/console/console-keys-modals.js":          "console-keys-modals.js",
		"/console/console-keys-picker.js":          "console-keys-picker.js",
		"/console/console-keys-pricing.js":         "console-keys-pricing.js",
		"/console/console-logs-data.js":            "console-logs-data.js",
		"/console/console-logs-errors.js":          "console-logs-errors.js",
		"/console/console-traffic-icons.js":        "console-traffic-icons.js",
		"/console/console-traffic-topology.js":     "console-traffic-topology.js",
		"/console/console-overview-charts.js":      "console-overview-charts.js",
	}

	for routePath, assetName := range assetFiles {
		name := assetName
		contentType := "text/javascript; charset=utf-8"
		if strings.HasSuffix(name, ".css") {
			contentType = "text/css; charset=utf-8"
		}
		registerGetHead(routePath, func(c *gin.Context) {
			serveEmbeddedFile(c, name, contentType, "no-cache, must-revalidate")
		})
	}

	// Legacy /admin/* static JS/CSS aliases for seamless backward compatibility
	legacyAliases := map[string]string{
		"/admin/i18n.js":                    "console-i18n.js",
		"/admin/admin-i18n.js":              "console-i18n.js",
		"/admin/cpa-management.js":          "console-cpa.js",
		"/admin/admin-cpa.js":               "console-cpa.js",
		"/admin/cpa-management.css":         "console-cpa.css",
		"/admin/admin-cpa.css":              "console-cpa.css",
		"/admin/overview.js":                "console-overview.js",
		"/admin/admin-overview.js":          "console-overview.js",
		"/admin/overview.css":               "console-overview.css",
		"/admin/admin-overview.css":         "console-overview.css",
		"/admin/console-base.css":           "console-base.css",
		"/admin/console-panels.css":         "console-panels.css",
		"/admin/console-members.css":        "console-members.css",
		"/admin/console-tables.css":         "console-tables.css",
		"/admin/console-modals.css":         "console-modals.css",
		"/admin/console-feedback.css":       "console-feedback.css",
		"/admin/console-logs.css":           "console-logs.css",
		"/admin/console-theme-beszel.css":   "console-theme-beszel.css",
		"/admin/console-theme-v239.css":     "console-theme-v239.css",
		"/admin/console-theme-v2310.css":    "console-theme-v2310.css",
		"/admin/console-validations.css":    "console-validations.css",
		"/admin/console-upstream-stats.css": "console-upstream-stats.css",
		"/admin/style.css":                  "console-base.css",
		"/admin/admin-style.css":            "console-base.css",
		"/admin/quotas-priority.js":         "console-quotas-priority.js",
		"/admin/quotas-validation.js":       "console-quotas-validation.js",
		"/admin/quotas-fetch.js":            "console-quotas-fetch.js",
		"/admin/quotas-render.js":           "console-quotas-render.js",
		"/admin/quotas-workbuddy.js":        "console-quotas-workbuddy.js",
		"/admin/quotas-stats.js":            "console-quotas-stats.js",
		"/admin/keys.js":                    "console-keys.js",
		"/admin/admin-keys.js":              "console-keys.js",
		"/admin/stats.js":                   "console-stats.js",
		"/admin/admin-stats.js":             "console-stats.js",
		"/admin/logs.js":                    "console-logs.js",
		"/admin/admin-logs.js":              "console-logs.js",
		"/admin/traffic.js":                 "console-traffic.js",
		"/admin/traffic.css":                "console-traffic.css",
		"/admin/state.js":                   "console-state.js",
		"/admin/nav.js":                     "console-nav.js",
		"/admin/auth.js":                    "console-auth.js",
		"/admin/format.js":                  "console-format.js",
		"/admin/polling.js":                 "console-polling.js",
		"/admin/member.js":                  "console-member.js",
		"/admin/refresh.js":                 "console-refresh.js",
		"/admin/init.js":                    "console-init.js",
	}

	for routePath, assetName := range legacyAliases {
		name := assetName
		contentType := "text/javascript; charset=utf-8"
		if strings.HasSuffix(name, ".css") {
			contentType = "text/css; charset=utf-8"
		}
		registerGetHead(routePath, func(c *gin.Context) {
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
		if c.Request.Method == http.MethodHead {
			c.Status(http.StatusOK)
			return
		}
		c.Data(http.StatusOK, ctype, data)
	}

	registerGetHead("/console/assets/*filepath", imageHandler)
	registerGetHead("/admin/assets/*filepath", imageHandler)
}

func serveEmbeddedFile(c *gin.Context, assetName, contentType, cacheControl string) {
	data, err := embeddedAssets.ReadFile(assetName)
	if err != nil {
		c.String(http.StatusNotFound, fmt.Sprintf("Asset %s not found: %v", assetName, err))
		return
	}

	// The index HTML references every asset with a "?v=" cache-busting query.
	// Rewrite it to the build version so a redeploy always yields fresh URLs.
	// This must happen before the ETag is derived, so the ETag describes the
	// bytes actually sent rather than the embedded original.
	if assetName == "console.html" {
		assembled, errAssemble := assembleConsoleHTML(data)
		if errAssemble != nil {
			c.String(http.StatusInternalServerError,
				fmt.Sprintf("Console assembly failed: %v", errAssemble))
			return
		}
		data = rewriteAssetVersions(assembled)
	}

	etag := fileETags[assetName]
	if etag == "" || assetName == "console.html" {
		h := md5.Sum(data)
		etag = `"` + hex.EncodeToString(h[:8]) + `"`
		if assetName != "console.html" {
			fileETags[assetName] = etag
		}
	}

	if match := c.GetHeader("If-None-Match"); match != "" && (match == etag || match == "W/"+etag || match == "*") {
		c.Status(http.StatusNotModified)
		return
	}

	c.Header("ETag", etag)
	c.Header("Cache-Control", cacheControl)
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}
	c.Data(http.StatusOK, contentType, data)
}
