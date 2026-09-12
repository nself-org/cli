package runner

// Note: LocalExecutor tests below run `echo`/`true` on whatever machine runs
// `go test` (dev laptop or CI runner already executing this test binary) —
// never against nself's protected staging/prod runner hosts. SSHExecutor
// tests only check argument construction, never open a real connection.

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestLocalExecutor_RunReturnsOutput(t *testing.T) {
	var ex LocalExecutor
	out, err := ex.Run(context.Background(), "echo hello-runner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello-runner" {
		t.Fatalf("out = %q, want %q", out, "hello-runner")
	}
	if ex.Label() != "local" {
		t.Fatalf("Label() = %q", ex.Label())
	}
}

func TestLocalExecutor_RunReturnsErrorOnNonZeroExit(t *testing.T) {
	var ex LocalExecutor
	_, err := ex.Run(context.Background(), "exit 7")
	if err == nil {
		t.Fatal("expected an error for a non-zero exit")
	}
}

func TestLocalExecutor_SupportsNullglob(t *testing.T) {
	// Regression guard: chromium.go relies on `shopt -s nullglob` working,
	// which requires bash, not POSIX sh.
	var ex LocalExecutor
	out, err := ex.Run(context.Background(), "shopt -s nullglob; for f in /no/such/path/*; do echo FOUND; done; echo done")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "done" {
		t.Fatalf("out = %q, nullglob should leave the loop empty", out)
	}
}

func TestNewSSHExecutor_DefaultsKeyPath(t *testing.T) {
	os.Unsetenv("NSELF_DEPLOY_KEY_PATH")
	os.Unsetenv("NSELF_DEPLOY_SSH_KEY")
	ex := NewSSHExecutor("ci-user@runner-host", "")
	if ex.Target.KeyPath == "" {
		t.Fatal("expected a default key path, got empty")
	}
	if !strings.HasSuffix(ex.Target.KeyPath, "id_ed25519") {
		t.Fatalf("KeyPath = %q, want default id_ed25519", ex.Target.KeyPath)
	}
	if ex.Label() != "ci-user@runner-host" {
		t.Fatalf("Label() = %q", ex.Label())
	}
}

func TestNewSSHExecutor_HonorsExplicitKeyPath(t *testing.T) {
	ex := NewSSHExecutor("ci-user@runner-host", "/custom/key")
	if ex.Target.KeyPath != "/custom/key" {
		t.Fatalf("KeyPath = %q, want /custom/key", ex.Target.KeyPath)
	}
}

func TestNewSSHExecutor_UsesEnvKeyPath(t *testing.T) {
	os.Setenv("NSELF_DEPLOY_KEY_PATH", "/env/key")
	defer os.Unsetenv("NSELF_DEPLOY_KEY_PATH")
	ex := NewSSHExecutor("ci-user@runner-host", "")
	if ex.Target.KeyPath != "/env/key" {
		t.Fatalf("KeyPath = %q, want /env/key", ex.Target.KeyPath)
	}
}
