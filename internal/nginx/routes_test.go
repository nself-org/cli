package nginx

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// TestConflictErrorNamesBothPlugins is the S01.T02 regression test.
// When two plugins both claim the same route, HasDomainConflict must emit
// an error message that names both competing plugins, not just "conflict detected".
// Error format: "route conflict: /api claimed by <pluginA> and <pluginB>"
func TestConflictErrorNamesBothPlugins(t *testing.T) {
	routes := []NginxRoute{
		{ServerName: "api", Location: "/", PluginName: "plugin-a"},
		{ServerName: "api", Location: "/", PluginName: "plugin-b"},
	}

	conflict, details := HasDomainConflict(routes)
	if !conflict {
		t.Fatal("S01.T02 regression: HasDomainConflict() = false, expected true for duplicate route")
	}
	if len(details) == 0 {
		t.Fatal("S01.T02 regression: HasDomainConflict() returned no conflict details")
	}

	msg := details[0]

	// Both plugin names must appear in the conflict message.
	if !strings.Contains(msg, "plugin-a") {
		t.Errorf("S01.T02 regression: conflict message must name plugin-a; got: %q", msg)
	}
	if !strings.Contains(msg, "plugin-b") {
		t.Errorf("S01.T02 regression: conflict message must name plugin-b; got: %q", msg)
	}

	// Message must not be the old generic format without plugin names.
	if !strings.Contains(msg, "claimed by") {
		t.Errorf("S01.T02 regression: conflict message must contain 'claimed by'; got: %q", msg)
	}

	t.Logf("conflict message: %s", msg)
}

// TestConflictErrorNamesBothPlugins_NoPluginNames verifies that when PluginName
// is empty (legacy callers), HasDomainConflict still reports the conflict but
// falls back to the server-name-based format instead of panicking.
func TestConflictErrorNamesBothPlugins_NoPluginNames(t *testing.T) {
	routes := []NginxRoute{
		{ServerName: "api.example.com", Location: "/"},
		{ServerName: "api.example.com", Location: "/"},
	}

	conflict, details := HasDomainConflict(routes)
	if !conflict {
		t.Fatal("expected conflict for duplicate server_name without plugin names")
	}
	if len(details) == 0 {
		t.Fatal("expected at least one conflict detail string")
	}

	t.Logf("fallback conflict message: %s", details[0])
}

// ── Gap #7: Hasura nginx upstream must use the container-internal port ──────

// TestHasuraUpstream_UsesContainerPort verifies that the generated Hasura
// upstream always targets the fixed container-internal port (8080), never
// cfg.Hasura.Port — which is the HOST-mapped port and may be set to any value
// (e.g. 8181) to avoid a host port collision without affecting how nginx
// reaches Hasura over the Docker network.
func TestHasuraUpstream_UsesContainerPort(t *testing.T) {
	cfg := &config.Config{BaseDomain: "example.com"}
	cfg, err := config.ApplyDefaults(cfg)
	if err != nil {
		t.Fatalf("ApplyDefaults() error: %v", err)
	}
	// Simulate a host-mapped port override — must NOT change the upstream.
	cfg.Hasura.Port = 8181

	g := &Generator{cfg: cfg}
	entries := g.coreRoutes(cfg.BaseDomain, "example-com")

	var hasuraEntry *routeEntry
	for i := range entries {
		if entries[i].filename == "hasura.conf" {
			hasuraEntry = &entries[i]
		}
	}
	if hasuraEntry == nil {
		t.Fatal("expected a hasura.conf route entry")
	}
	if hasuraEntry.data.Upstream != "hasura:8080" {
		t.Errorf("expected Hasura upstream 'hasura:8080' regardless of HASURA_PORT override, got %q", hasuraEntry.data.Upstream)
	}
}

// ── Gap #5: app-prefixed subdomains ──────────────────────────────────────────

// TestCoreRoutes_NoAppName_BareScheme verifies the default (APP_NAME unset)
// scheme is unchanged: "api"/"auth", never prefixed. This is the backward
// compatibility guarantee for existing single-app deployments (ummat, unity).
func TestCoreRoutes_NoAppName_BareScheme(t *testing.T) {
	cfg := &config.Config{BaseDomain: "example.com"}
	cfg, err := config.ApplyDefaults(cfg)
	if err != nil {
		t.Fatalf("ApplyDefaults() error: %v", err)
	}
	if cfg.AppName != "" {
		t.Fatalf("expected AppName to default to empty, got %q", cfg.AppName)
	}

	g := &Generator{cfg: cfg}
	entries := g.coreRoutes(cfg.BaseDomain, "example-com")

	routesByFile := make(map[string]string)
	for _, e := range entries {
		routesByFile[e.filename] = e.data.Route
	}
	if routesByFile["hasura.conf"] != "api" {
		t.Errorf("expected bare route 'api' when AppName unset, got %q", routesByFile["hasura.conf"])
	}
	if routesByFile["auth.conf"] != "auth" {
		t.Errorf("expected bare route 'auth' when AppName unset, got %q", routesByFile["auth.conf"])
	}
}

// TestCoreRoutes_AppName_PrefixesSubdomains verifies that setting APP_NAME
// produces app-prefixed routes ("api.task", "auth.task") so the final
// server_name is "api.task.{BASE_DOMAIN}" instead of "api.{BASE_DOMAIN}".
func TestCoreRoutes_AppName_PrefixesSubdomains(t *testing.T) {
	cfg := &config.Config{BaseDomain: "stack-two.nself.org", AppName: "task"}
	cfg, err := config.ApplyDefaults(cfg)
	if err != nil {
		t.Fatalf("ApplyDefaults() error: %v", err)
	}

	g := &Generator{cfg: cfg}
	entries := g.coreRoutes(cfg.BaseDomain, "stack-two-nself-org")

	routesByFile := make(map[string]string)
	for _, e := range entries {
		routesByFile[e.filename] = e.data.Route
	}
	if routesByFile["hasura.conf"] != "api.task" {
		t.Errorf("expected app-prefixed route 'api.task', got %q", routesByFile["hasura.conf"])
	}
	if routesByFile["auth.conf"] != "auth.task" {
		t.Errorf("expected app-prefixed route 'auth.task', got %q", routesByFile["auth.conf"])
	}
}
