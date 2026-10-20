package commands

// config_set_test.go — Regression coverage for the T16 fix: `nself config
// set` inherits FindNSelfRoot's monorepo auto-chdir (a `.backend/` marker
// found in a parent directory silently becomes the project root) and used to
// print a bare "Added KEY to .env" with no indication the write actually
// landed in `.backend/.env`, two directories above where the user was
// standing.
//
// This reproduces the exact scratch-fixture scenario from the ticket
// description: a monorepo root with a `.backend/.env`, and a nested
// `apps/foo/` subdirectory with nothing of its own. Running `config set`
// from `apps/foo/` must still write to `.backend/.env` (that resolution
// behavior is intentional and unchanged) but must now say so.
//
// Inputs: a temp monorepo fixture; cwd inside its nested app dir.
// Outputs: assertions on the file actually written and on stdout.
// Constraints: no certbot/docker/network; same package as config_test.go so
// it reuses newConfigCmd() and captureStdout() rather than redefining them.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/sysexec"
)

// TestConfigSet_MonorepoRedirectIsVisible proves the T16 fix: running
// `config set` from a nested monorepo subdirectory still writes to
// `.backend/.env` (unchanged write-target resolution) but now prints the
// full resolved path plus an explicit redirection notice, instead of a bare
// ".env" that gives no indication the write happened somewhere else.
//
// Before the fix, the success message was built from filepath.Base(envFile),
// which always prints exactly ".env" regardless of which directory it came
// from — this test fails against that code because the output contains no
// path separator at all, let alone the backend directory's full path.
func TestConfigSet_MonorepoRedirectIsVisible(t *testing.T) {
	t.Cleanup(sysexec.Set(&sysexec.Fake{})) // D-0275: v1.5 config set builds; keep it off the host mkcert
	monorepoRoot := t.TempDir()
	backendDir := filepath.Join(monorepoRoot, ".backend")
	nestedDir := filepath.Join(monorepoRoot, "apps", "foo")

	if err := os.MkdirAll(backendDir, 0o750); err != nil {
		t.Fatalf("mkdir .backend: %v", err)
	}
	if err := os.MkdirAll(nestedDir, 0o750); err != nil {
		t.Fatalf("mkdir apps/foo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backendDir, ".env"), []byte("EXISTING=root\n"), 0o600); err != nil {
		t.Fatalf("seed .backend/.env: %v", err)
	}

	t.Chdir(nestedDir)

	root := newConfigCmd()
	root.SetArgs([]string{"config", "set", "TEST_KEY", "hello"})

	output, err := captureStdout(t, func() error { return root.Execute() })
	if err != nil {
		t.Fatalf("config set failed: %v", err)
	}

	// Write-target resolution is unchanged: the value lands in
	// .backend/.env, and nothing is created in the nested dir itself.
	backendEnv, readErr := os.ReadFile(filepath.Join(backendDir, ".env"))
	if readErr != nil {
		t.Fatalf("read .backend/.env: %v", readErr)
	}
	if !strings.Contains(string(backendEnv), "TEST_KEY=hello") {
		t.Errorf("expected TEST_KEY=hello in .backend/.env, got:\n%s", backendEnv)
	}
	if _, statErr := os.Stat(filepath.Join(nestedDir, ".env")); !os.IsNotExist(statErr) {
		t.Errorf("expected no .env created in the nested dir, stat err: %v", statErr)
	}

	// The printed output must contain the full resolved path (not a bare
	// ".env" with no directory context) and must name the redirection.
	if !strings.Contains(output, filepath.Join(backendDir, ".env")) {
		t.Errorf("expected output to contain the full path %q, got:\n%s",
			filepath.Join(backendDir, ".env"), output)
	}
	if !strings.Contains(output, "monorepo") {
		t.Errorf("expected output to name the monorepo redirection, got:\n%s", output)
	}
}
