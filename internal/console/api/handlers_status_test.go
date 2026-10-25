package api

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
)

// TestBuildProviderGroupsSkipsEventsWithoutAuthIndex guards the phantom-group
// regression: events with an empty auth_index cannot be attributed to any
// credential, so model-name fallback invented "zhipu" and "zcode" groups beside
// the real "zcode-cn" group and one upstream rendered as three cards.
func TestBuildProviderGroupsSkipsEventsWithoutAuthIndex(t *testing.T) {
	t.Parallel()

	r, _, _, providers, cleanup := setupTestRouter(t)
	defer cleanup()
	_ = r

	// Register the BigModel credential the way the console does after syncing
	// auth files, so attributed events resolve to zcode-cn instead of falling
	// back to the model-name pattern "glm" -> zhipu.
	providers.RegisterAuthAccount("728a76ab8bdf3e34", "zcode-cn", "BigModel")

	rows := []db.RequestEventRow{
		// Attributed to the real BigModel credential.
		{TS: 1, Model: "GLM-5.3", Status: 200, AuthIndex: "728a76ab8bdf3e34"},
		{TS: 2, Model: "GLM-5.3-Flash", Status: 200, AuthIndex: "728a76ab8bdf3e34"},
		// Unattributed: these must not create provider groups.
		{TS: 3, Model: "glm-5.3", Status: 200, AuthIndex: ""},
		{TS: 4, Model: "glm-5.3-flash-trial", Status: 200, AuthIndex: ""},
		{TS: 5, Model: "glm-5", Status: 200, AuthIndex: ""},
	}

	groups := buildProviderGroups(rows, providers, 1000)

	if len(groups) != 1 {
		t.Fatalf("expected exactly 1 provider group, got %d: %+v", len(groups), groupKeys(groups))
	}
	if groups[0].Provider != "zcode-cn" {
		t.Errorf("group provider = %q, want zcode-cn", groups[0].Provider)
	}
	if groups[0].Accounts != 1 {
		t.Errorf("group accounts = %d, want 1", groups[0].Accounts)
	}
}

// TestBuildProviderGroupsKeepsAttributedGroups ensures the empty-auth filter does
// not drop events that do carry a credential.
func TestBuildProviderGroupsKeepsAttributedGroups(t *testing.T) {
	t.Parallel()

	_, _, _, providers, cleanup := setupTestRouter(t)
	defer cleanup()

	rows := []db.RequestEventRow{
		{TS: 1, Model: "GLM-5.3", Status: 200, AuthIndex: "auth-a"},
		{TS: 2, Model: "GLM-5.3", Status: 500, AuthIndex: "auth-a"},
		{TS: 3, Model: "some-model", Status: 200, AuthIndex: "auth-b"},
	}

	groups := buildProviderGroups(rows, providers, 1000)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups (one per auth), got %d: %+v", len(groups), groupKeys(groups))
	}
	for _, g := range groups {
		if g.Accounts != 1 {
			t.Errorf("group %q accounts = %d, want 1", g.Provider, g.Accounts)
		}
	}
}

func groupKeys(groups []providerGroupResult) []string {
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		out = append(out, g.Provider)
	}
	return out
}
