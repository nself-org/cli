package manifestv2_test

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

func wantUnmapped(t *testing.T, name string, set map[string]any, want string) *manifestv2.Manifest {
	t.Helper()
	data := v1Base(t, set)
	if got := unmappedNames(t, data); got != want {
		t.Errorf("%s: unmapped = %q, want %q", name, got, want)
	}
	m, err := manifestv2.Normalize(data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return m
}

// notifications: a deprecated plugin whose top-level deprecated, replacedBy and
// replacement repeat the deprecation block. Those map; the version strings have
// no v2 home and stay refused.
func TestV1DeprecationFamily(t *testing.T) {
	dep := map[string]any{"announcedDate": "2026-05-07", "eolDate": "2026-11-07", "replacedBy": "notify", "migrationGuide": "https://example.com/guide"}
	base := func(extra map[string]any) map[string]any {
		set := map[string]any{"status": "deprecated", "deprecation": dep}
		for k, v := range extra {
			set[k] = v
		}
		return set
	}
	m := wantUnmapped(t, "real notifications", base(map[string]any{"deprecated": true, "replacedBy": "notify", "replacement": "notify",
		"deprecatedSince": "1.1.0", "deprecated_in": "v1.1.0", "removal_target": "v2.0.0"}), "deprecatedSince,deprecated_in,removal_target")
	if m.Deprecation.ReplacedBy != "notify" || m.Deprecation.State != "deprecated" {
		t.Errorf("deprecation = %+v", m.Deprecation)
	}
	wantUnmapped(t, "repeats", base(map[string]any{"deprecated": true, "replacedBy": "notify", "replacement": "notify"}), "")
	// Fills an empty replacedBy.
	noRepl := map[string]any{"announcedDate": "2026-05-07", "eolDate": "2026-11-07", "migrationGuide": "https://example.com/guide"}
	m = wantUnmapped(t, "fill", map[string]any{"status": "deprecated", "deprecation": noRepl, "replacement": "notify"}, "")
	if m.Deprecation.ReplacedBy != "notify" {
		t.Errorf("replacement did not fill replacedBy: %+v", m.Deprecation)
	}
	// Conflicts and misfits stay refused.
	wantUnmapped(t, "conflict with block", base(map[string]any{"replacedBy": "other"}), "replacedBy")
	wantUnmapped(t, "two values", base(map[string]any{"replacedBy": "notify", "replacement": "other"}), "replacedBy,replacement")
	wantUnmapped(t, "not deprecated", map[string]any{"replacedBy": "notify", "deprecated": true}, "deprecated,replacedBy")
	wantUnmapped(t, "deprecated false", base(map[string]any{"deprecated": false}), "deprecated")
}

// nself-audit and claw-web: service keys repeat or fill what is derived from port.
func TestV1ServiceKeys(t *testing.T) {
	m := wantUnmapped(t, "nself-audit", map[string]any{"port": 3843, "docker_image": "nself-audit", "health_check_path": "/health"}, "")
	if m.Service.Image == nil || *m.Service.Image != "nself-audit" || *m.Service.Healthcheck != "/health" || m.Service.Kind != "compose" {
		t.Errorf("service = %+v", m.Service)
	}
	m = wantUnmapped(t, "claw-web", map[string]any{"port": 3004, "health_endpoint": "/health", "internalPort": 3004,
		"depends_on": []any{"claw"}, "serviceType": "node", "minNselfVersion": "1.4.0"}, "serviceType")
	if m.Requires == nil || strings.Join(m.Requires.Plugins, ",") != "claw" || m.Requires.Nself != ">=1.4.0" {
		t.Errorf("requires = %+v", m.Requires)
	}
	if out, err := manifestv2.Marshal(m); err != nil {
		t.Fatal(err)
	} else if back, err := manifestv2.Parse(out); err != nil || back.Requires.Plugins[0] != "claw" {
		t.Errorf("v2 round trip: %v", err)
	}
	// Conflicts with the derived service, or a service that is not compose.
	wantUnmapped(t, "health differs", map[string]any{"port": 3843, "health_check_path": "/ready"}, "health_check_path")
	wantUnmapped(t, "health_endpoint differs", map[string]any{"port": 3843, "health_endpoint": "/live", "health_check_path": "/health"}, "health_check_path")
	wantUnmapped(t, "internalPort differs", map[string]any{"port": 3004, "internalPort": 3005}, "internalPort")
	wantUnmapped(t, "internalPort without port", map[string]any{"internalPort": 3004}, "internalPort")
	wantUnmapped(t, "image on a library", map[string]any{"docker_image": "x"}, "docker_image")
	wantUnmapped(t, "docker object (browser)", map[string]any{"port": 3719, "docker": map[string]any{"image": "nself/nself-browser", "shm_size": "256m"}}, "docker")
	wantUnmapped(t, "depends_on not slugs", map[string]any{"depends_on": []any{"Postgres 16"}}, "depends_on")
	wantUnmapped(t, "depends_on not a list", map[string]any{"depends_on": "claw"}, "depends_on")
}

// migrations maps only as {dir: "migrations", apply: "boot"}; the two real v1
// shapes (a file list, an object with schema and tables) stay refused.
func TestV1MigrationsAndCommands(t *testing.T) {
	m := wantUnmapped(t, "v2 shape", map[string]any{"migrations": map[string]any{"dir": "migrations", "apply": "boot"}}, "")
	if m.Migrations == nil || m.Migrations.Dir != "migrations" {
		t.Errorf("migrations = %+v", m.Migrations)
	}
	wantUnmapped(t, "auth-enterprise", map[string]any{"migrations": []any{"migrations/001_mfa_tables.sql", "migrations/002_sso_tables.sql"}}, "migrations")
	wantUnmapped(t, "status-page", map[string]any{"migrations": map[string]any{"schema": "np_status", "tables": []any{"np_status.np_status_components"}, "migration_dir": "migrations/"}}, "migrations")
	wantUnmapped(t, "wrong dir", map[string]any{"migrations": map[string]any{"dir": "sql", "apply": "boot"}}, "migrations")
	wantUnmapped(t, "extra key", map[string]any{"migrations": map[string]any{"dir": "migrations", "apply": "boot", "schema": "np_x"}}, "migrations")
	// mcp: a v1 commands list ("nself mcp serve") has no side_effect, output or json, and its names carry the prefix.
	wantUnmapped(t, "mcp commands", map[string]any{"commands": []any{map[string]any{"name": "nself mcp serve", "description": "Start MCP server"}}}, "commands")
	wantUnmapped(t, "schema_version and serviceType", map[string]any{"schema_version": "1.1.3", "serviceType": "node"}, "schema_version,serviceType")
}

// A manifest that fails to normalize reports every derived key it carries; a
// routes value that converts is still not reported.
func TestUnmappedWhenNormalizeFails(t *testing.T) {
	data := v1Base(t, map[string]any{"version": "bad", "depends_on": []any{"claw"}, "routes": []any{map[string]any{"method": "GET", "path": "/x"}}})
	if got := unmappedNames(t, data); got != "depends_on" {
		t.Errorf("unmapped = %q", got)
	}
}
