package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeOutput_ArgvDirAndStdout(t *testing.T) {
	dir := installRecordingDocker(t)
	work := t.TempDir()
	out, err := ComposeOutput(context.Background(), work, "ps", "--services")
	if err != nil {
		t.Fatal(err)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	if got := strings.TrimSpace(string(argv)); got != "compose ps --services" {
		t.Errorf("argv = %q", got)
	}
	if strings.TrimSpace(string(out)) != "out-text" {
		t.Errorf("stdout = %q (stderr must not be mixed in)", out)
	}
}

func TestComposeOutput_NonZeroExit(t *testing.T) {
	installRecordingDocker(t)
	t.Setenv("FAKE_RC", "1")
	out, err := ComposeOutput(context.Background(), t.TempDir(), "logs", "web")
	if err == nil || !strings.Contains(err.Error(), "err-text") || !strings.Contains(err.Error(), "compose logs") {
		t.Fatalf("want an error quoting the stderr line, got %v", err)
	}
	if strings.TrimSpace(string(out)) != "out-text" {
		t.Fatalf("failed command lost partial stdout: %q", out)
	}
}
