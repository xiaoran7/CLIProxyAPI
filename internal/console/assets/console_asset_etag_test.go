package assets

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestAssetRoutesServeETag guards the ETag of every asset route the console
// exposes.
//
// serveEmbeddedFile used to shadow its outer `etag` variable with
// `etag, ok := fileETag(...)`.  The outer variable therefore stayed empty, and
// gin DELETES a header whose value is empty -- so every single-file asset
// route (all the console JS and CSS) went out without an ETag, and the
// If-None-Match comparison ran against "" so 304 revalidation never fired.
//
// The /console document route was unaffected (it assigns with "="), which is
// why the pre-existing ETag tests all passed while the assets silently lost
// their caching headers.
func TestAssetRoutesServeETag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterUI(r)

	routes := make(map[string]string, len(consoleAssetRoutes)+len(legacyAdminAssetRoutes))
	for route, asset := range consoleAssetRoutes {
		routes[route] = asset
	}
	for route, asset := range legacyAdminAssetRoutes {
		routes[route] = asset
	}
	for route := range splitModuleRoutes {
		routes[route] = "module"
	}
	if len(routes) == 0 {
		t.Fatal("no asset routes registered")
	}

	for route := range routes {
		t.Run(route, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, route, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("GET %s = %d, want 200", route, w.Code)
			}
			etag := w.Header().Get("ETag")
			if etag == "" {
				t.Fatalf("GET %s served without an ETag header", route)
			}

			req := httptest.NewRequest(http.MethodGet, route, nil)
			req.Header.Set("If-None-Match", etag)
			w2 := httptest.NewRecorder()
			r.ServeHTTP(w2, req)
			if w2.Code != http.StatusNotModified {
				t.Errorf("GET %s with If-None-Match = %d, want 304", route, w2.Code)
			}
		})
	}
}

// TestAssetRoutesServeETagUnderConcurrency hammers the asset routes from many
// goroutines.  The ETag cache is written from the request handlers, so a
// data race there would be a fatal "concurrent map writes" panic rather than a
// recoverable error.
func TestAssetRoutesServeETagUnderConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterUI(r)

	routes := make([]string, 0, len(consoleAssetRoutes))
	for route := range consoleAssetRoutes {
		routes = append(routes, route)
	}
	if len(routes) == 0 {
		t.Fatal("no asset routes registered")
	}

	done := make(chan string, 64)
	for i := 0; i < 16; i++ {
		go func() {
			for j := 0; j < 20; j++ {
				for _, route := range routes {
					w := httptest.NewRecorder()
					r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, route, nil))
					if w.Header().Get("ETag") == "" {
						done <- "missing ETag for " + route
						return
					}
				}
			}
			done <- ""
		}()
	}
	for i := 0; i < 16; i++ {
		if msg := <-done; msg != "" {
			t.Fatal(msg)
		}
	}
}
