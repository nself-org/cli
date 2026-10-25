package build

// plugin_nginx_server_name_test.go — regression coverage for the 2026-09-03
// staging incident: nginx logged `conflicting server name
// "api.stack-two.nself.org" on 0.0.0.0:443, ignored` because two generated
// site files under nginx/sites/ declared the same server_name. nginx does
// not fail its config test over this — it silently serves whichever server
// block loaded last, so the duplicate went unnoticed until a route behaved
// unpredictably in production.
//
// Purpose: prove InjectPluginNginxRoutes refuses to write a plugin's nginx
// route file when its server_name is already claimed by a different file in
// nginx/sites/, and that this refusal is a hard error (fails the build),
// not the warn-and-continue behavior it shipped with.
// Inputs: a temp workdir with a pre-existing nginx/sites/ file and a temp
// plugin directory with a conflicting/non-conflicting nginx/ route file.
// Outputs: pass/fail on whether the conflict is caught and reported by name.
// Constraints: exercises InjectPluginNginxRoutes exactly as orchestrator_build_ssl.go
// calls it (Step 7.1), against a real filesystem via t.TempDir().

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func minimalTestConfig(baseDomain string) *config.Config {
	return &config.Config{
		ProjectName: "testproj",
		BaseDomain:  baseDomain,
	}
}

// writeExistingSite writes a pre-existing nginx/sites/<name> file declaring
// server_name serverName, simulating a core/optional/custom route the
// nginx.Generator already wrote earlier in the same `nself build`.
func writeExistingSite(t *testing.T, workdir, name, serverName string) {
	t.Helper()
	sitesDir := filepath.Join(workdir, "nginx", "sites")
	if err := os.MkdirAll(sitesDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "server {\n    listen 443 ssl;\n    server_name " + serverName + ";\n}\n"
	if err := os.WriteFile(filepath.Join(sitesDir, name), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// writePluginNginxRoute creates pluginDir/<pluginName>/nginx/<fileName>
// declaring server_name serverName, simulating an installed plugin that
// ships its own nginx route config.
func writePluginNginxRoute(t *testing.T, pluginDir, pluginName, fileName, serverName string) {
	t.Helper()
	nginxDir := filepath.Join(pluginDir, pluginName, "nginx")
	if err := os.MkdirAll(nginxDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "server {\n    listen 443 ssl;\n    server_name " + serverName + ";\n}\n"
	if err := os.WriteFile(filepath.Join(nginxDir, fileName), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestInjectPluginNginxRoutes_RejectsDuplicateServerName reproduces the
// staging incident directly: a core route (hasura.conf) already claims
// "api.stack-two.nself.org" and a plugin ships a route that claims the exact
// same server_name. InjectPluginNginxRoutes must fail the build and name
// both files, not silently write the duplicate and warn.
func TestInjectPluginNginxRoutes_RejectsDuplicateServerName(t *testing.T) {
	workdir := t.TempDir()
	pluginDir := t.TempDir()

	writeExistingSite(t, workdir, "hasura.conf", "api.stack-two.nself.org")
	writePluginNginxRoute(t, pluginDir, "gateway", "route.conf", "api.stack-two.nself.org")

	cfg := minimalTestConfig("stack-two.nself.org")

	count, err := InjectPluginNginxRoutes(workdir, pluginDir, cfg)
	if err == nil {
		t.Fatalf("expected an error for duplicate server_name \"api.stack-two.nself.org\", got nil (count=%d)", count)
	}

	for _, want := range []string{"api.stack-two.nself.org", "hasura.conf", "gateway-route.conf"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message %q does not name %q — a conflict must name both sources so it can be fixed", err.Error(), want)
		}
	}

	// The conflicting file must NOT have been written — a rejected route
	// must not land in nginx/sites/ half-applied.
	if _, statErr := os.Stat(filepath.Join(workdir, "nginx", "sites", "gateway-route.conf")); statErr == nil {
		t.Error("gateway-route.conf was written despite the server_name conflict — the build should have failed before the write")
	}
}

// TestInjectPluginNginxRoutes_AllowsDistinctServerNames is the negative
// control: two different server_names must inject cleanly with no error.
func TestInjectPluginNginxRoutes_AllowsDistinctServerNames(t *testing.T) {
	workdir := t.TempDir()
	pluginDir := t.TempDir()

	writeExistingSite(t, workdir, "hasura.conf", "api.stack-two.nself.org")
	writePluginNginxRoute(t, pluginDir, "gateway", "route.conf", "gateway.stack-two.nself.org")

	cfg := minimalTestConfig("stack-two.nself.org")

	count, err := InjectPluginNginxRoutes(workdir, pluginDir, cfg)
	if err != nil {
		t.Fatalf("unexpected error for distinct server_names: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if _, statErr := os.Stat(filepath.Join(workdir, "nginx", "sites", "gateway-route.conf")); statErr != nil {
		t.Errorf("gateway-route.conf was not written: %v", statErr)
	}
}

// TestInjectPluginNginxRoutes_SelfUpdateNotAConflict verifies that a plugin
// re-running `nself build` and rewriting its own previously-injected file is
// NOT treated as a conflict with itself.
func TestInjectPluginNginxRoutes_SelfUpdateNotAConflict(t *testing.T) {
	workdir := t.TempDir()
	pluginDir := t.TempDir()

	writePluginNginxRoute(t, pluginDir, "gateway", "route.conf", "gateway.stack-two.nself.org")
	cfg := minimalTestConfig("stack-two.nself.org")

	if _, err := InjectPluginNginxRoutes(workdir, pluginDir, cfg); err != nil {
		t.Fatalf("first inject: unexpected error: %v", err)
	}
	// Second run: same plugin, same file, same server_name — must not
	// conflict with the file it is about to overwrite.
	if _, err := InjectPluginNginxRoutes(workdir, pluginDir, cfg); err != nil {
		t.Fatalf("second inject (self-update): unexpected error: %v", err)
	}
}

// ── Whole-directory sweep ────────────────────────────────────────────────
//
// The plugin-injection check above catches only one of three writers into
// nginx/sites/. The nginx generator (Step 7) and the api-docs site conf
// (Step 11) also write there, and neither can see the others' output.
// checkServerNameUniqueness runs after all of them, so it is the check that
// actually covers the reported staging failure.

// writeSite is a small helper for the post-validate sweep tests.
func writeSite(t *testing.T, sitesDir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(sitesDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sitesDir, name), []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestCheckServerNameUniqueness_ReportsDuplicate reproduces the staging
// finding: two generated site files both claim api.stack-two.nself.org.
// `nginx -t` calls that "syntax is ok" and merely logs that it ignored one,
// so post-build validation has to be the thing that fails the build — and
// it has to name BOTH files, or the operator cannot tell which route died.
func TestCheckServerNameUniqueness_ReportsDuplicate(t *testing.T) {
	sitesDir := filepath.Join(t.TempDir(), "sites")
	writeSite(t, sitesDir, "hasura.conf", "server {\n    server_name api.stack-two.nself.org;\n}\n")
	writeSite(t, sitesDir, "ir-gateway.conf", "server {\n    server_name api.stack-two.nself.org;\n}\n")

	result := PostValidateResult{NginxValid: true}
	checkServerNameUniqueness(sitesDir, &result)

	if len(result.Errors) != 1 {
		t.Fatalf("Errors = %d (%v), want exactly 1 for one duplicated server_name", len(result.Errors), result.Errors)
	}
	if result.NginxValid {
		t.Error("NginxValid stayed true despite a duplicate server_name — the build would not fail")
	}
	for _, want := range []string{"api.stack-two.nself.org", "hasura.conf", "ir-gateway.conf"} {
		if !strings.Contains(result.Errors[0], want) {
			t.Errorf("error %q does not name %q", result.Errors[0], want)
		}
	}
}

// TestCheckServerNameUniqueness_Deterministic verifies the reported "first"
// file does not depend on directory iteration order, which os.ReadDir does
// not guarantee across platforms. A conflict that names a different file on
// each run is not actionable.
func TestCheckServerNameUniqueness_Deterministic(t *testing.T) {
	var first string
	for i := 0; i < 5; i++ {
		sitesDir := filepath.Join(t.TempDir(), "sites")
		writeSite(t, sitesDir, "zzz-last.conf", "server { server_name api.example.com; }\n")
		writeSite(t, sitesDir, "aaa-first.conf", "server { server_name api.example.com; }\n")

		result := PostValidateResult{NginxValid: true}
		checkServerNameUniqueness(sitesDir, &result)
		if len(result.Errors) != 1 {
			t.Fatalf("run %d: Errors = %v, want 1", i, result.Errors)
		}
		if i == 0 {
			first = result.Errors[0]
			continue
		}
		if result.Errors[0] != first {
			t.Fatalf("conflict message is not deterministic:\n  run 0: %s\n  run %d: %s", first, i, result.Errors[0])
		}
	}
	if !strings.Contains(first, "both aaa-first.conf and zzz-last.conf") {
		t.Errorf("conflict message does not report the files in a stable sorted order: %s", first)
	}
}

// TestCheckServerNameUniqueness_CleanDirectory is the negative control:
// distinct server_names, plus the catch-all default server which every
// deployment legitimately has, must produce no findings.
func TestCheckServerNameUniqueness_CleanDirectory(t *testing.T) {
	sitesDir := filepath.Join(t.TempDir(), "sites")
	writeSite(t, sitesDir, "hasura.conf", "server { server_name api.example.com; }\n")
	writeSite(t, sitesDir, "auth.conf", "server { server_name auth.example.com; }\n")
	writeSite(t, sitesDir, "a-default.conf", "server { server_name _; }\n")
	writeSite(t, sitesDir, "b-default.conf", "server { server_name _; }\n")

	result := PostValidateResult{NginxValid: true}
	checkServerNameUniqueness(sitesDir, &result)

	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v, want none for distinct server_names plus the catch-all default", result.Errors)
	}
	if !result.NginxValid {
		t.Error("NginxValid was cleared with no actual conflict")
	}
}

// TestCheckServerNameUniqueness_NoSitesDir verifies a missing sites/
// directory is not a finding — a stack fronted by another stack's nginx
// legitimately generates none.
func TestCheckServerNameUniqueness_NoSitesDir(t *testing.T) {
	result := PostValidateResult{NginxValid: true}
	checkServerNameUniqueness(filepath.Join(t.TempDir(), "nginx"), &result)
	if len(result.Errors) != 0 || !result.NginxValid {
		t.Errorf("a missing sites/ directory produced findings: %v", result.Errors)
	}
}

// TestCheckServerNameUniqueness_DifferentPortsNotAConflict is the check
// that keeps this gate from being worse than the bug. nginx's own rule is
// per listen port — it reports `conflicting server name "x" on 0.0.0.0:443`
// — so the same name served on :80 by one file and :443 by another is a
// legitimate split. Failing a build over that would be a false positive on
// correct configuration.
func TestCheckServerNameUniqueness_DifferentPortsNotAConflict(t *testing.T) {
	sitesDir := filepath.Join(t.TempDir(), "sites")
	writeSite(t, sitesDir, "redirect.conf", "server {\n    listen 80;\n    server_name api.example.com;\n}\n")
	writeSite(t, sitesDir, "hasura.conf", "server {\n    listen 443 ssl;\n    server_name api.example.com;\n}\n")

	result := PostValidateResult{NginxValid: true}
	checkServerNameUniqueness(sitesDir, &result)

	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v, want none: the same name on :80 and :443 is a legitimate http/https split, not a conflict", result.Errors)
	}
}

// TestCheckServerNameUniqueness_SamePortIsAConflict is its counterpart:
// once the ports match, it must fail.
func TestCheckServerNameUniqueness_SamePortIsAConflict(t *testing.T) {
	sitesDir := filepath.Join(t.TempDir(), "sites")
	writeSite(t, sitesDir, "a.conf", "server {\n    listen 443 ssl;\n    server_name api.example.com;\n}\n")
	writeSite(t, sitesDir, "b.conf", "server {\n    listen 443 ssl;\n    server_name api.example.com;\n}\n")

	result := PostValidateResult{NginxValid: true}
	checkServerNameUniqueness(sitesDir, &result)

	if len(result.Errors) != 1 {
		t.Fatalf("Errors = %v, want exactly 1", result.Errors)
	}
	if !strings.Contains(result.Errors[0], "port 443") {
		t.Errorf("error does not name the port it conflicts on: %s", result.Errors[0])
	}
}

// TestPostValidate_FailsOnDuplicateServerName is the end-to-end proof that
// the sweep is actually WIRED INTO the build, not merely present in the
// package. orchestrator_build_finish turns any PostValidateResult.Errors
// into a failed build, so a check that exists but is never called from
// PostValidate would leave the defect shipping exactly as before.
func TestPostValidate_FailsOnDuplicateServerName(t *testing.T) {
	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  postgres:\n    image: postgres:16\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sitesDir := filepath.Join(dir, "nginx", "sites")
	// Two generated site files claiming one FQDN on one port: the core
	// Hasura route and an api-docs route that landed on the same name.
	writeSite(t, sitesDir, "hasura.conf", "server {\n    listen 443 ssl;\n    server_name api.stack-two.nself.org;\n}\n")
	writeSite(t, sitesDir, "api-docs.conf", "server {\n    listen 443 ssl;\n    server_name api.stack-two.nself.org;\n}\n")

	result := PostValidate(composePath, sitesDir)

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "api.stack-two.nself.org") {
			found = true
		}
	}
	if !found {
		t.Errorf("PostValidate did not report the duplicate server_name — the check is not wired into the build. Errors: %v", result.Errors)
	}
	if result.NginxValid {
		t.Error("NginxValid = true despite a duplicate server_name; the build would not fail")
	}
}
