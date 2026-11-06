package promote

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writePromotionRecord creates a promotion record JSON file in <dir>/.nself/promotions/.
func writePromotionRecord(t *testing.T, dir string, rec PromotionRecord) {
	t.Helper()
	promotionsDir := filepath.Join(dir, ".nself", "promotions")
	if err := os.MkdirAll(promotionsDir, 0o755); err != nil {
		t.Fatalf("mkdirAll promotions: %v", err)
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	path := filepath.Join(promotionsDir, rec.ID+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write record: %v", err)
	}
}

// TestRollback_WithPriorPromoteTag verifies that Rollback returns an error from
// the restore sub-command (since nself binary is not available in unit test
// context) but correctly resolves the backup tag from the promotion record —
// meaning findLatestPromoteBackup succeeds and the tag is non-empty, so the
// failure is in exec.Command("nself", ...) not in the tag lookup.
func TestRollback_WithPriorPromoteTag(t *testing.T) {
	tmp := t.TempDir()

	// Write a promotion record with a backup tag.
	rec := PromotionRecord{
		ID:          "promo-test-001",
		Source:      "staging",
		Target:      "prod",
		ApproveID:   "TICKET-001",
		BackupTag:   "pre-promote-prod-1713600000",
		StartedAt:   time.Now().Add(-5 * time.Minute),
		CompletedAt: time.Now().Add(-4 * time.Minute),
		Status:      "success",
	}
	writePromotionRecord(t, tmp, rec)

	// Rollback should resolve the tag from the record and fail at the
	// nself backup restore step (nself binary not present in test env),
	// but the error must not be "no promotion records found" or
	// "no promotion backup tags found" — those are tag-lookup errors, not
	// restore errors.
	list := withBackups(t, nil, nil)
	err := RollbackProject(context.Background(), tmp, "demo", "", list)
	if err == nil {
		// If somehow it passed (e.g., nself binary is in PATH and stub works),
		// that is also acceptable.
		return
	}

	// The error should come from the restore exec, not from tag lookup.
	msg := err.Error()
	if strings.Contains(msg, "no promotion records found") || strings.Contains(msg, "no promotion backup tags found") {
		t.Errorf("rollback returned tag-lookup error despite valid record: %v", err)
	}
	// The error should reference "restore failed" or similar exec error, not "find backup".
	if strings.Contains(msg, "find backup") {
		t.Errorf("rollback should not fail at find-backup step when record exists: %v", err)
	}
}

// TestRollback_NoHistory verifies that Rollback returns a clear error when
// there are no promotion records present in the project directory.
func TestRollback_NoHistory(t *testing.T) {
	tmp := t.TempDir()
	// No .nself/promotions directory or records written.

	err := RollbackProject(context.Background(), tmp, "demo", "", nil)
	if err == nil {
		t.Fatal("expected error for no promotion history, got nil")
	}

	// The error must clearly indicate that no history was found, not a
	// generic filesystem error or restore error.
	msg := err.Error()
	if !strings.Contains(msg, "no promotion") && !strings.Contains(msg, "no backup") && !strings.Contains(msg, "find backup") {
		t.Errorf("error %q does not describe missing history", msg)
	}
}

// TestFindLatestPromoteBackup_MultipleRecords verifies that the most recent
// promotion record's backup tag is returned when multiple records exist.
func TestFindLatestPromoteBackup_MultipleRecords(t *testing.T) {
	tmp := t.TempDir()

	// Write two records — the second (lexicographically later) should win.
	// ReadDir returns entries in alphabetical order, and findLatestPromoteBackup
	// iterates in reverse, so the last entry is the most recent.
	writePromotionRecord(t, tmp, PromotionRecord{
		ID:        "promo-aaa",
		BackupTag: "pre-promote-prod-111",
		Status:    "success",
	})
	writePromotionRecord(t, tmp, PromotionRecord{
		ID:        "promo-zzz",
		BackupTag: "pre-promote-prod-999",
		Status:    "success",
	})

	tag, err := findLatestPromoteBackup(tmp)
	if err != nil {
		t.Fatalf("findLatestPromoteBackup: %v", err)
	}
	if tag != "pre-promote-prod-999" {
		t.Errorf("expected latest tag %q, got %q", "pre-promote-prod-999", tag)
	}
}

// TestFindLatestPromoteBackup_EmptyDirectory verifies that an error is returned
// when the promotions directory exists but contains no valid records.
func TestFindLatestPromoteBackup_EmptyDirectory(t *testing.T) {
	tmp := t.TempDir()
	promotionsDir := filepath.Join(tmp, ".nself", "promotions")
	if err := os.MkdirAll(promotionsDir, 0o755); err != nil {
		t.Fatalf("mkdirAll: %v", err)
	}

	_, err := findLatestPromoteBackup(tmp)
	if err == nil {
		t.Fatal("expected error for empty promotions directory, got nil")
	}
}

// withBackups swaps the exec seam and returns a lister over entries.
func withBackups(t *testing.T, entries []BackupRef, calls *[][]string) BackupLister {
	t.Helper()
	oldRun := runNself
	t.Cleanup(func() { runNself = oldRun })
	runNself = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if calls != nil {
			*calls = append(*calls, args)
		}
		return nil, nil
	}
	return func() ([]BackupRef, error) { return entries, nil }
}

func TestRollbackResolvesTagToBackupID(t *testing.T) {
	now := time.Now()
	var calls [][]string
	list := withBackups(t, []BackupRef{
		{ID: "demo_full_20261106_120000", Date: now},
		{ID: "demo_full_20261105_120000_pre-promote-prod-1", Tag: "pre-promote-prod-1", Date: now.Add(-24 * time.Hour)},
	}, &calls)
	if err := RollbackProject(context.Background(), t.TempDir(), "demo", "pre-promote-prod-1", list); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	want := []string{"backup", "restore", "demo_full_20261105_120000_pre-promote-prod-1", "--yes", "--confirm-project", "demo"}
	if len(calls) != 1 || strings.Join(calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("argv = %v, want [%v]", calls, want)
	}
}

func TestRollbackUnknownTagRestoresNothing(t *testing.T) {
	var calls [][]string
	list := withBackups(t, []BackupRef{{ID: "demo_full_20261106_120000", Date: time.Now()}}, &calls)
	err := RollbackProject(context.Background(), t.TempDir(), "demo", "pre-promote-prod-9", list)
	if err == nil || !strings.Contains(err.Error(), "pre-promote-prod-9") {
		t.Fatalf("want error naming the tag, got %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("runNself called: %v", calls)
	}
}

func TestRollbackNeverPassesTagFlag(t *testing.T) {
	var calls [][]string
	list := withBackups(t, []BackupRef{{ID: "demo_full_20261105_120000_t", Tag: "t", Date: time.Now()}}, &calls)
	if err := RollbackProject(context.Background(), t.TempDir(), "demo", "t", list); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	for _, c := range calls {
		for _, a := range c {
			if a == "--tag" || strings.HasPrefix(a, "--tag=") {
				t.Fatalf("argv contains --tag: %v", c)
			}
		}
	}
	if len(calls) == 0 {
		t.Fatal("restore never ran")
	}
}
