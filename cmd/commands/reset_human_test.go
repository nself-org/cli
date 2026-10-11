package commands

// Purpose: human-mode output when a path cannot be inspected (D-0375): the
// failure is warned about on stderr and the run never ends with an "already
// clean"/"removed" success line. Exit status stays 0 in human mode.
// Constraints: chmod 000 in t.TempDir(); skipped as root.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdStreams runs fn with os.Stdout and os.Stderr redirected and
// returns what was written to each.
func captureStdStreams(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	ro, wo, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	re, we, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	var bo, be bytes.Buffer
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(&bo, ro); done <- struct{}{} }()
	go func() { _, _ = io.Copy(&be, re); done <- struct{}{} }()
	os.Stdout, os.Stderr = wo, we
	func() {
		defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
		fn()
	}()
	_ = wo.Close()
	_ = we.Close()
	<-done
	<-done
	return bo.String(), be.String()
}

func TestHumanModeUninspectablePathIsNotReportedClean(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	cases := []struct {
		name, lock, warn string
		ok               []string
	}{
		{"reset", "ssl", "listing ssl", []string{"Reset complete", "already clean", "Removed "}},
		{"uninstall", ".nself", "checking .nself/cache/", []string{"project files removed", "To start fresh"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := buildFixture(t)
			if err := os.MkdirAll(filepath.Join(dir, "ssl"), 0o755); err != nil {
				t.Fatal(err)
			}
			locked := filepath.Join(dir, tc.lock)
			if err := os.Chmod(locked, 0o000); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

			var runErr error
			var stdout, stderr string
			inDir(t, dir, func() {
				stdout, stderr = captureStdStreams(t, func() {
					_, runErr = runBuildCmd(t, tc.name, "", "--yes")
				})
			})
			if runErr != nil {
				t.Fatalf("human mode keeps exit 0, got %v", runErr)
			}
			if !strings.Contains(stderr, tc.warn) {
				t.Errorf("stderr missing warning %q:\n%s", tc.warn, stderr)
			}
			if !strings.Contains(stderr, "could not be inspected or removed") {
				t.Errorf("stderr missing the incomplete-removal line:\n%s", stderr)
			}
			for _, bad := range tc.ok {
				if strings.Contains(stdout, bad) {
					t.Errorf("stdout must not contain success text %q:\n%s", bad, stdout)
				}
			}
		})
	}
}
