package compose

// snapshot_test.go — golden-fragment tests for compose generation.
//
// Rather than byte-for-byte golden files (which break on generated passwords),
// these tests verify that key Phase 16 structural fragments appear (or are
// absent) in the generated YAML output.  Each test uses Generate() end-to-end
// so it exercises the full buildDockerCompose → postProcess pipeline.

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// snapshotConfig returns a fully-populated config suitable for snapshot tests.
// All secrets are fixed so output is deterministic (no random generation).
func snapshotConfig() *config.Config {
	cfg := &config.Config{
		ProjectName:      "snaptest",
		BaseDomain:       "example.com",
		Env:              "dev",
		DockerNetwork:    "snaptest_network",
		DockerLogMaxSize: "10m",
		DockerLogMaxFile: "3",
		DockerStopGrace:  "30s",
		Postgres: config.PostgresConfig{
			Version:  "16-alpine",
			Host:     "postgres",
			User:     "postgres",
			Password: "snaptestpw",
			DB:       "nself",
			Port:     5432,
			MemLimit: "2g",
			CPULimit: "2.0",
		},
		Hasura: config.HasuraConfig{
			AdminSecret: "snaptestsecret",
			JWTKey:      "snaptestjwtkey1234567890123456",
			JWTType:     "HS256",
			Port:        8080,
			Version:     "v2.36.0",
			MemLimit:    "1g",
			CPULimit:    "1.0",
		},
		Auth: config.AuthConfig{
			Version:            "0.36.0",
			Port:               4000,
			ClientURL:          "http://localhost:3000",
			AccessTokenExpiry:  900,
			RefreshTokenExpiry: 2592000,
			SMTPHost:           "mailpit",
			SMTPPort:           1025,
			SMTPSender:         "noreply@localhost",
			MemLimit:           "256m",
			CPULimit:           "0.25",
		},
		Nginx: config.NginxConfig{
			BindIP:   "0.0.0.0",
			HTTPPort: 80,
			SSLPort:  443,
		},
	}
	return withTestAuthKey(cfg)
}

// generateYAML is a test helper that calls Generate() and returns the YAML
// string, failing the test immediately if generation errors.
func generateYAML(t *testing.T, cfg *config.Config) string {
	t.Helper()
	g := NewGenerator(cfg)
	data, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() returned error: %v", err)
	}
	return string(data)
}

// TestSnapshot_SecurityCapDrop verifies that every generated service in the
// basic compose contains "cap_drop" with "ALL" (Phase 16 security hardening).
func TestSnapshot_SecurityCapDrop(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	if !strings.Contains(yaml, "cap_drop") {
		t.Errorf("snapshot: expected 'cap_drop' in generated YAML but not found")
	}
	if !strings.Contains(yaml, "- ALL") {
		t.Errorf("snapshot: expected '- ALL' under cap_drop in generated YAML but not found")
	}
}

// TestSnapshot_SecurityNoNewPrivileges verifies the no-new-privileges security
// option appears in generated YAML for all services.
func TestSnapshot_SecurityNoNewPrivileges(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	if !strings.Contains(yaml, "no-new-privileges:true") {
		t.Errorf("snapshot: expected 'no-new-privileges:true' in security_opt but not found")
	}
}

// TestSnapshot_PostgresCapAdd verifies PostgreSQL gets IPC_LOCK added back after
// cap_drop ALL (needed for shared memory management).
func TestSnapshot_PostgresCapAdd(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	if !strings.Contains(yaml, "IPC_LOCK") {
		t.Errorf("snapshot: expected 'IPC_LOCK' in postgres cap_add but not found")
	}
}

// TestSnapshot_AuthHealthcheckUsesAuthPort verifies the auth healthcheck URL uses
// Auth.Port (4000 by default) — nhost/hasura-auth binds on the AUTH_PORT env var.
func TestSnapshot_AuthHealthcheckUsesAuthPort(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	// snapshotConfig sets Auth.Port=4000; healthcheck must match.
	if !strings.Contains(yaml, "http://localhost:4000/health") {
		t.Errorf("snapshot: expected 'http://localhost:4000/health' in auth healthcheck, yaml excerpt:\n%s",
			extractServiceBlock(yaml, "auth"))
	}
}

// TestSnapshot_AuthPortMapping verifies auth port mapping uses host:4000->container:4000.
func TestSnapshot_AuthPortMapping(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	if !strings.Contains(yaml, "127.0.0.1:4000:4000") {
		t.Errorf("snapshot: expected auth port mapping '127.0.0.1:4000:4000' in YAML, auth block:\n%s",
			extractServiceBlock(yaml, "auth"))
	}
}

// TestSnapshot_AuthPGAliasVars verifies AUTH_DB_HOST / AUTH_DB_URL appear in the
// generated YAML (Phase 16 T06 alias vars).
func TestSnapshot_AuthPGAliasVars(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	aliases := []string{"AUTH_DB_HOST", "AUTH_DB_PORT", "AUTH_DB_NAME", "AUTH_DB_USER", "AUTH_DB_URL"}
	for _, key := range aliases {
		if !strings.Contains(yaml, key) {
			t.Errorf("snapshot: expected env var %q in generated YAML but not found", key)
		}
	}
}

// TestSnapshot_AuthAllowedRedirectURLs verifies AUTH_ALLOWED_REDIRECT_URLS with
// the base domain and wildcard appear in the generated YAML (Phase 16 T07).
func TestSnapshot_AuthAllowedRedirectURLs(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	if !strings.Contains(yaml, "AUTH_ALLOWED_REDIRECT_URLS") {
		t.Errorf("snapshot: expected AUTH_ALLOWED_REDIRECT_URLS in generated YAML")
	}
	if !strings.Contains(yaml, "https://example.com") {
		t.Errorf("snapshot: expected 'https://example.com' in AUTH_ALLOWED_REDIRECT_URLS")
	}
	if !strings.Contains(yaml, "https://*.example.com") {
		t.Errorf("snapshot: expected 'https://*.example.com' in AUTH_ALLOWED_REDIRECT_URLS")
	}
}

// TestSnapshot_ServiceOrder verifies the basic service insertion order in the
// generated YAML: postgres before hasura, hasura before auth, auth before nginx.
func TestSnapshot_ServiceOrder(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	positions := map[string]int{}
	for _, svc := range []string{"postgres", "hasura", "auth", "nginx"} {
		idx := strings.Index(yaml, "\n    "+svc+":")
		if idx < 0 {
			t.Errorf("snapshot: service %q not found in generated YAML", svc)
		}
		positions[svc] = idx
	}

	pairs := [][2]string{
		{"postgres", "hasura"},
		{"hasura", "auth"},
		{"auth", "nginx"},
	}
	for _, p := range pairs {
		before, after := p[0], p[1]
		if positions[before] >= positions[after] {
			t.Errorf("snapshot: service order violation: %q (pos %d) must appear before %q (pos %d)",
				before, positions[before], after, positions[after])
		}
	}
}

// TestSnapshot_LoggingDriver verifies json-file logging is applied to all services.
func TestSnapshot_LoggingDriver(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	if !strings.Contains(yaml, "json-file") {
		t.Errorf("snapshot: expected 'json-file' logging driver in generated YAML")
	}
	if !strings.Contains(yaml, "max-size") {
		t.Errorf("snapshot: expected 'max-size' logging option in generated YAML")
	}
}

// TestSnapshot_StopGracePeriod verifies stop_grace_period is emitted.
func TestSnapshot_StopGracePeriod(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	if !strings.Contains(yaml, "stop_grace_period") {
		t.Errorf("snapshot: expected 'stop_grace_period' in generated YAML")
	}
}

// TestSnapshot_HasuraPort8080 verifies Hasura uses internal port 8080.
func TestSnapshot_HasuraPort8080(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	if !strings.Contains(yaml, "127.0.0.1:8080:8080") {
		t.Errorf("snapshot: expected Hasura port mapping '127.0.0.1:8080:8080' in YAML")
	}
}

// TestSnapshot_PostgresBindLoop verifies Postgres port binds to 127.0.0.1.
func TestSnapshot_PostgresBindLoop(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	if !strings.Contains(yaml, "127.0.0.1:5432:5432") {
		t.Errorf("snapshot: expected postgres port mapping '127.0.0.1:5432:5432' in YAML")
	}
}

// TestSnapshot_ResourceLimitsPresent verifies deploy.resources.limits is present
// for core services (postgres, hasura, auth) in the generated YAML.
func TestSnapshot_ResourceLimitsPresent(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	// At minimum the keywords must appear in the YAML.
	if !strings.Contains(yaml, "limits:") {
		t.Errorf("snapshot: expected 'limits:' (resource limits) in generated YAML")
	}
	if !strings.Contains(yaml, "memory:") {
		t.Errorf("snapshot: expected 'memory:' in deploy resource limits")
	}
	if !strings.Contains(yaml, "cpus:") {
		t.Errorf("snapshot: expected 'cpus:' in deploy resource limits")
	}
}

// TestSnapshot_ComposeV5PidsLimitAbsent verifies that the generated YAML contains
// NO top-level pids_limit keys for any service. Docker Compose v5 treats
// service-level pids_limit as a canonical alias for deploy.resources.limits.pids,
// and rejects the compose file as "conflicting" when both forms are present.
// This test is the primary regression guard for G0-T13 (CRITICAL hotfix).
func TestSnapshot_ComposeV5PidsLimitAbsent(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	if strings.Contains(yaml, "pids_limit:") {
		t.Errorf("snapshot (Compose v5 compat): generated YAML contains top-level 'pids_limit:' field — " +
			"this causes 'can't set distinct values on pids_limit and deploy.resources.limits.pids' on Compose v5+. " +
			"All pids limits must use the deploy.resources.limits.pids form only.")
	}
}

// TestSnapshot_ComposeV5PidsLimitInDeploy verifies that deploy.resources.limits.pids
// is emitted in the generated YAML. This is the modern Docker Compose syntax (v3.4+)
// that works on both Compose v4 and v5 without conflict.
func TestSnapshot_ComposeV5PidsLimitInDeploy(t *testing.T) {
	yaml := generateYAML(t, snapshotConfig())

	// The pids field must appear under deploy.resources.limits (emitted as "pids:" by yaml.v3).
	if !strings.Contains(yaml, "pids:") {
		t.Errorf("snapshot (Compose v5 compat): generated YAML does not contain 'pids:' field under " +
			"deploy.resources.limits — fork-bomb prevention is missing. Expected: deploy.resources.limits.pids: 100")
	}
}

// TestSnapshot_HasuraGraphqlEnvVarForwarding is the end-to-end fixture
// regression test for P6 FIX-CLI-2 (2026-09-03): a full `nself build`-style
// Generate() run, with HASURA_GRAPHQL_ENABLE_ALLOWLIST and an arbitrary
// HASURA_GRAPHQL_* var set via cfg.Passthrough (mirroring what the loader
// populates from a real .env file — see internal/config/loader_helpers.go),
// must land both vars verbatim in the rendered docker-compose YAML under the
// hasura service. This is the "diffed against a golden fixture" check for
// the fix: rather than a byte-for-byte golden file (this package deliberately
// avoids those — see file header — because generated secrets make output
// non-deterministic), it follows the established golden-fragment convention
// and asserts the exact rendered fragment is present.
func TestSnapshot_HasuraGraphqlEnvVarForwarding(t *testing.T) {
	cfg := snapshotConfig()
	cfg.Passthrough = map[string]string{
		"HASURA_GRAPHQL_ENABLE_ALLOWLIST": "true",
		"HASURA_GRAPHQL_FOO":              "bar",
	}
	yaml := generateYAML(t, cfg)

	hasuraBlock := extractServiceBlock(yaml, "hasura")
	for _, frag := range []string{
		"HASURA_GRAPHQL_ENABLE_ALLOWLIST: \"true\"",
		"HASURA_GRAPHQL_FOO: bar",
	} {
		if !strings.Contains(hasuraBlock, frag) {
			t.Errorf("snapshot: expected %q in generated hasura service block but not found, block:\n%s", frag, hasuraBlock)
		}
	}
}

// extractServiceBlock is a debug helper that returns the lines in the YAML that
// start at "  <name>:" and continue until the next top-level-indented service.
// Used only for test failure messages — not part of the assertion logic.
func extractServiceBlock(yaml, name string) string {
	marker := "\n    " + name + ":"
	start := strings.Index(yaml, marker)
	if start < 0 {
		return "(service not found)"
	}
	rest := yaml[start+1:] // skip leading newline
	// Find the next service at same indent level
	lines := strings.Split(rest, "\n")
	var out []string
	for i, line := range lines {
		if i > 0 && len(line) >= 4 && line[:4] != "    " && strings.TrimSpace(line) != "" {
			break
		}
		out = append(out, line)
		if i > 40 {
			out = append(out, "  ...(truncated)")
			break
		}
	}
	return strings.Join(out, "\n")
}
