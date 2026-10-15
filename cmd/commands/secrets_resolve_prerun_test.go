package commands

// Purpose: pin resolver auditing across the real inherited source guard.
// Inputs: a fake source tree and both compatibility command paths.
// Outputs: one value-free audit event per refused invocation.
// Constraints: no secret store, Docker, or decryption required.
// SPORT: contract:cli.secret-resolve/v1, D-0581.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSecretsResolveSourceGuardAudit(t *testing.T) {
	bin := pilotBinary(t)
	for _, mode := range []string{"0", "1"} {
		for _, allow := range []string{"0", "1"} {
			t.Run(mode+"/allow="+allow, func(t *testing.T) {
				checkResolveSourceGuardAudit(t, bin, mode, allow)
			})
		}
	}
}

func checkResolveSourceGuardAudit(t *testing.T, bin, mode, allow string) {
	t.Helper()
	root, home := t.TempDir(), t.TempDir()
	for _, dir := range []string{"cmd/commands", "internal"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module nself\n\ngo 1.26.6\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"secrets", "resolve", "--project", root, "--for-attempt", "guard-a1", "--json", "project/FOO"}
	if mode == "1" {
		args = append([]string{"config"}, args...)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "NSELF_V15="+mode,
		"NSELF_ALLOW_SOURCE_DIR="+allow, "NSELF_NONINTERACTIVE=1", "NSELF_NO_UPDATE_CHECK=1", "OTEL_EXPORTER_OTLP_ENDPOINT=")
	stdoutPath := filepath.Join(t.TempDir(), "stdout")
	stdout, err := os.Create(stdoutPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdout.Close() })
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = stdout, &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("expected refusal")
	}
	out, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("CLI source repository")
	if allow == "1" {
		want = []byte("E542") // A regular output descriptor fails after pre-run succeeds.
	}
	if !bytes.Contains(append(out, stderr.Bytes()...), want) {
		t.Fatalf("wrong refusal: stdout=%s stderr=%s", out, stderr.Bytes())
	}
	b, err := os.ReadFile(filepath.Join(home, ".nself", "audit.log"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(b, []byte(`"event":"secret-resolve"`)) != 1 || !bytes.Contains(b, []byte(`"attempt_id":"guard-a1"`)) {
		t.Fatalf("expected exactly one resolver event: %s", b)
	}
}
