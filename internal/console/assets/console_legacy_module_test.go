package assets

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// This file locks the contract between the console split and the legacy asset
// URLs.
//
// v8.0.76 carved seven single-file modules into several files each:
//
//	console-i18n.js          -> 5 dictionary shards + the runtime
//	console-quotas-render.js -> meter-single, meter-multi, cards-compact, cards-full
//	console-keys.js          -> keys-modals, keys-picker, keys-pricing
//	console-logs.js          -> logs-data, logs-errors
//	console-traffic.js       -> traffic-icons, traffic-topology
//	console-overview.js      -> overview-charts
//	console-panels.css       -> panels-quota, panels-compact, panels-status
//
// The alias URLs (/console/keys.js, /admin/keys.js, ...) used to serve the
// monolith, so they used to hand out the whole module. Serving only the
// fragment that kept the old file name silently drops code, and in the
// overview case it is worse than silent: the fragment destructures
// window.GATE_OVW at load time, so loading it on its own throws
// "TypeError: Cannot destructure property 'PAGE_ID' of 'window.GATE_OVW' as it
// is undefined" before the first line of the module runs.
//
// The alias routes therefore serve the whole module again. The canonical
// /console/console-*.js routes keep serving one file each, because that is
// what console.html loads.
//
// These tests make that contract executable: they catch a module table that
// fell behind the files, an alias route that was never moved over to the
// module table, and a bundle whose members are not the ones the page loads.

// assetReferencePattern matches an asset URL in the console shell, with or
// without the cache-busting query.
var assetReferencePattern = regexp.MustCompile(`(?:src|href)="(/console/[^"?]+)(?:\?[^"]*)?"`)

// consoleShellReferences returns the asset URLs the console shell loads, in
// document order.
func consoleShellReferences(t *testing.T) []string {
	t.Helper()
	shell, err := embeddedAssets.ReadFile("console.html")
	if err != nil {
		t.Fatalf("read console shell: %v", err)
	}
	matches := assetReferencePattern.FindAllStringSubmatch(string(shell), -1)
	if len(matches) == 0 {
		t.Fatal("no asset reference found in the console shell")
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

// consolePageAssets returns the asset names the console shell loads, in
// document order.
func consolePageAssets(t *testing.T) []string {
	t.Helper()
	refs := consoleShellReferences(t)
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		name, ok := consoleAssetRoutes[ref]
		if !ok {
			// A reference the route table does not know about is a different
			// bug; the routing contract test covers it.
			continue
		}
		out = append(out, name)
	}
	return out
}

// TestSplitModuleMembersAreEmbedded locks that every module member really is
// an embedded asset, and that no file is claimed by two modules.
func TestSplitModuleMembersAreEmbedded(t *testing.T) {
	owner := map[string]string{}
	for module, members := range splitModules {
		if len(members) == 0 {
			t.Errorf("module %s has no members", module)
			continue
		}
		for _, member := range members {
			if _, err := embeddedAssets.ReadFile(member); err != nil {
				t.Errorf("module %s lists %s, which is not embedded: %v", module, member, err)
				continue
			}
			if other, dup := owner[member]; dup {
				t.Errorf("%s belongs to both module %s and module %s", member, other, module)
				continue
			}
			owner[member] = module
		}
	}
}

// TestSplitModuleOrderMatchesTheConsoleLoadOrder locks that a module is
// assembled in the order the console page loads its parts.
//
// The parts are classic scripts that share one global lexical environment:
// the order is what decides which file runs first, so a module assembled in
// a different order than the page uses could execute a statement before the
// name it reads exists.
func TestSplitModuleOrderMatchesTheConsoleLoadOrder(t *testing.T) {
	page := consolePageAssets(t)
	position := map[string]int{}
	for i, name := range page {
		if _, seen := position[name]; !seen {
			position[name] = i
		}
	}

	for module, members := range splitModules {
		previous := -1
		for _, member := range members {
			at, ok := position[member]
			if !ok {
				t.Errorf("module %s lists %s, which the console page does not load", module, member)
				continue
			}
			if at <= previous {
				t.Errorf("module %s is not in the console page's load order: %s comes after %s",
					module, member, members[max(0, indexOf(members, member)-1)])
			}
			previous = at
		}
	}
}

// indexOf returns the position of name in names, or -1.
func indexOf(names []string, name string) int {
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return -1
}

// TestAliasRoutesServeTheWholeModule locks that every legacy alias route
// hands out the complete module, not a fragment of it.
func TestAliasRoutesServeTheWholeModule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterUI(r)

	for route, members := range splitModuleRoutes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("route %s answered %d; the legacy URL would break its page", route, rec.Code)
			continue
		}
		body := rec.Body.String()
		for _, member := range members {
			raw, err := embeddedAssets.ReadFile(member)
			if err != nil {
				t.Errorf("read %s: %v", member, err)
				continue
			}
			if !strings.Contains(body, string(raw)) {
				t.Errorf("route %s does not serve %s; the legacy URL would lose that part of the module",
					route, member)
			}
		}
	}
}

// TestAliasRoutesCoverEverySplitAsset locks the other direction: an asset
// that the split carved up must not be reachable through a URL that the
// console page does not load, because such a URL used to serve the whole
// module and would now hand out a fragment.
//
// The console page loads the canonical /console/console-*.js URLs, and those
// keep serving one file each. Every other URL that resolves to a split
// module has to serve the module in full.
func TestAliasRoutesCoverEverySplitAsset(t *testing.T) {
	loaded := map[string]bool{}
	for _, ref := range consoleShellReferences(t) {
		loaded[ref] = true
	}

	missing := map[string]bool{}
	for route, asset := range consoleAssetRoutes {
		if _, split := splitModules[asset]; !split || loaded[route] {
			continue
		}
		if _, ok := splitModuleRoutes[route]; !ok {
			missing[route] = true
		}
	}
	for route, asset := range legacyAdminAssetRoutes {
		if _, split := splitModules[asset]; !split || loaded[route] {
			continue
		}
		if _, ok := splitModuleRoutes[route]; !ok {
			missing[route] = true
		}
	}

	routes := make([]string, 0, len(missing))
	for route := range missing {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	for _, route := range routes {
		t.Errorf("route %s serves a split module but is not registered in splitModuleRoutes; "+
			"it would hand out only the fragment that kept the old file name", route)
	}
}

// TestPageLoadedRoutesServeExactlyOneFile locks the other side of the
// contract: a URL the console page loads keeps serving the one file the page
// asks for, so that the page and the module bundles never disagree about
// what a file contains.
func TestPageLoadedRoutesServeExactlyOneFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterUI(r)

	for _, ref := range consoleShellReferences(t) {
		name, ok := consoleAssetRoutes[ref]
		if !ok {
			if strings.HasPrefix(ref, "/console/assets/") {
				// Images are served by the image handler, not the route table.
				continue
			}
			t.Errorf("the console shell loads %s, which is not in consoleAssetRoutes", ref)
			continue
		}
		want, err := embeddedAssets.ReadFile(name)
		if err != nil {
			t.Errorf("read %s: %v", name, err)
			continue
		}
		req := httptest.NewRequest(http.MethodGet, ref, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("route %s answered %d", ref, rec.Code)
			continue
		}
		if rec.Body.String() != string(want) {
			t.Errorf("route %s does not serve exactly %s", ref, name)
		}
	}
}

// TestSplitModuleRoutesDoNotCollideWithAssetRoutes locks that one route
// lives in exactly one table.
//
// Registering the same path twice makes the router panic at startup, and a
// route that is in both tables would silently be served by whichever
// registration ran last.
func TestSplitModuleRoutesDoNotCollideWithAssetRoutes(t *testing.T) {
	for route := range splitModuleRoutes {
		if _, ok := consoleAssetRoutes[route]; ok {
			t.Errorf("route %s is registered in both consoleAssetRoutes and splitModuleRoutes", route)
		}
		if _, ok := legacyAdminAssetRoutes[route]; ok {
			t.Errorf("route %s is registered in both legacyAdminAssetRoutes and splitModuleRoutes", route)
		}
	}
}
