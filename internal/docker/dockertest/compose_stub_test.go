package dockertest

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// The stub answers and records a call that carries both project env files in order, and
// refuses one that does not with exit 51.
func TestComposeProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("compose stub requires a POSIX shell")
	}
	t.Run("complete", func(t *testing.T) {
		root, calls := ComposeProject(t, "svc-a\n")
		out, err := exec.Command("docker", "compose", "--env-file", filepath.Join(root, ".env"),
			"--env-file", filepath.Join(root, ".nself/compose.env"), "ps").Output()
		if err != nil {
			t.Fatalf("stub refused a call with both env files: %v", err)
		}
		if string(out) != "svc-a\n" {
			t.Errorf("stub stdout = %q, want the configured output", out)
		}
		if got := calls(); len(got) != 1 {
			t.Errorf("recorded calls = %q, want one", got)
		}
	})
	t.Run("missing env file", func(t *testing.T) {
		root, _ := ComposeProject(t, "")
		var exit *exec.ExitError
		err := exec.Command("docker", "compose", "--env-file", filepath.Join(root, ".nself/compose.env"), "ps").Run()
		if !errors.As(err, &exit) || exit.ExitCode() != 51 {
			t.Errorf("stub with one env file: err %v, want exit 51", err)
		}
	})
}
