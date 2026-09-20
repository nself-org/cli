//go:build darwin || linux

package maintenance

// runner_work_unreadable_test.go — pins the distinction between "there is no
// _work directory" and "there is one and we could not read it".
//
// Purpose: reclaimRunnerWork returned a bare (0, nil, nil) for BOTH, so a
//          cleanup pass that was locked out of a runner root logged exactly
//          what a healthy no-op logs. On a box under disk pressure that is the
//          difference between "nothing to reclaim" and "could not even look",
//          and only one of those is worth paging about.
// Inputs:  temp runner roots — one with no _work, one where _work is a regular
//          file (ReadDir returns ENOTDIR).
// Outputs: assertions on the skipped slice.
// Constraints: no runner, no systemd, no docker.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReclaimRunnerWork_MissingWorkDirIsSilent — absent is indistinguishable
// from already-clean, so it stays a silent zero with nothing recorded.
func TestReclaimRunnerWork_MissingWorkDirIsSilent(t *testing.T) {
	root := t.TempDir() // no _work inside

	bytes, reclaimed, skipped := reclaimRunnerWork(root, false)

	if bytes != 0 || len(reclaimed) != 0 {
		t.Errorf("fresh runner root: got (%d bytes, %d reclaimed), want (0, 0)", bytes, len(reclaimed))
	}
	if len(skipped) != 0 {
		t.Errorf("a missing _work dir is not a skip worth reporting, got %+v", skipped)
	}
}

// TestReclaimRunnerWork_UnreadableWorkDirIsReportedAsASkip — a read failure is
// NOT nothing to do. It must surface in the skipped slice with the reason, so
// the run stays diagnosable after the fact.
func TestReclaimRunnerWork_UnreadableWorkDirIsReportedAsASkip(t *testing.T) {
	root := t.TempDir()
	// A regular file where _work should be: ReadDir fails with ENOTDIR, which
	// os.IsNotExist does not match. This is the shape an upgrade or a bad
	// mount leaves behind.
	work := filepath.Join(root, "_work")
	if err := os.WriteFile(work, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("seed _work file: %v", err)
	}

	bytes, reclaimed, skipped := reclaimRunnerWork(root, false)

	if bytes != 0 || len(reclaimed) != 0 {
		t.Errorf("nothing should be reclaimed from an unreadable root, got (%d, %d)", bytes, len(reclaimed))
	}
	if len(skipped) != 1 {
		t.Fatalf("an unreadable _work must be recorded as a skip, got %d entries: %+v", len(skipped), skipped)
	}
	if skipped[0].Path != work {
		t.Errorf("skip path = %q, want %q", skipped[0].Path, work)
	}
	// The reason has to carry the underlying errno — "skipped" without a cause
	// is the same dead end as the bare zero it replaced.
	if !strings.Contains(strings.ToLower(skipped[0].Reason), "could not read") {
		t.Errorf("skip reason should say the directory could not be read, got %q", skipped[0].Reason)
	}
	if !strings.Contains(skipped[0].Reason, "directory") {
		t.Errorf("skip reason should carry the underlying error, got %q", skipped[0].Reason)
	}
}
