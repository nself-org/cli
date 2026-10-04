package docker

// exec_capture_test.go — ExecCapture argv and output handling (fake docker).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecCapture_ArgvAndOutput(t *testing.T) {
	dir := installRecordingDocker(t)
	stdout, stderr, err := ExecCapture(context.Background(), "web-nginx-1", []string{"nginx", "-t"})
	if err != nil {
		t.Fatalf("ExecCapture: %v", err)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	if got := strings.TrimSpace(string(argv)); got != "exec web-nginx-1 nginx -t" {
		t.Errorf("argv = %q", got)
	}
	if !strings.Contains(stdout, "out-text") || !strings.Contains(stderr, "err-text") {
		t.Errorf("output not captured: %q / %q", stdout, stderr)
	}
}

func TestExecCapture_NonZeroExit(t *testing.T) {
	installRecordingDocker(t)
	t.Setenv("FAKE_RC", "1")
	_, stderr, err := ExecCapture(context.Background(), "c", []string{"nginx", "-t"})
	if err == nil || !strings.Contains(err.Error(), "err-text") || !strings.Contains(stderr, "err-text") {
		t.Fatalf("want error quoting stderr, got %v", err)
	}
}
