package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// backupIDFromName strips the payload extension and ".age"; ".tar.gz" counts as
// one extension so a metadata backup's tag does not end in ".tar" (D-0710).
func TestBackupIDFromName(t *testing.T) {
	for name, want := range map[string]string{
		"p_full_20260101_000000.dump":             "p_full_20260101_000000",
		"p_full_20260101_000000.dump.age":         "p_full_20260101_000000",
		"p_metadata_20260101_000000_keep.tar.gz":  "p_metadata_20260101_000000_keep",
		"p_metadata_20260101_000000.tar.gz.age":   "p_metadata_20260101_000000",
		"manual":                                  "manual",
		"hand.sql":                                "hand",
		"p_full_20260101_000000_pre.upgrade.dump": "p_full_20260101_000000_pre.upgrade",
	} {
		if got := backupIDFromName(name); got != want {
			t.Errorf("backupIDFromName(%q) = %q, want %q", name, got, want)
		}
	}
	if tag := newBackupEntry("p_metadata_20260101_000000_keep.tar.gz", 1, time.Now()).Tag; tag != "keep" {
		t.Errorf("metadata .tar.gz tag = %q, want keep", tag)
	}
}

// A real (non-dry) prune removes the file of every backup it reports as pruned,
// whatever the extension, and leaves the kept and held ones on disk.
func TestPruneDeletesPrunedFiles(t *testing.T) {
	dir := t.TempDir()
	n := time.Now()
	now := time.Date(n.Year(), n.Month(), n.Day(), 12, 0, 0, 0, n.Location())
	write := func(name string, age time.Duration) {
		p := filepath.Join(dir, name)
		if age > 24*time.Hour {
			// Old backups sit on fixed mid-month days, so the monthly slot (day 1) never keeps one.
			age = now.Sub(time.Date(2025, 3, 10+int(age/(24*time.Hour))%10, 12, 0, 0, 0, n.Location()))
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	old := []string{
		BackupFileName("my_proj", "full", "20250101_000000", "", ".dump"),
		BackupFileName("my_proj", "full", "20250102_000000", "", ".dump.age"),
		BackupFileName("my_proj", "metadata", "20250103_000000", "", ".tar.gz"),
		BackupFileName("my_proj", "metadata", "20250104_000000", "", ".tar.gz.age"),
	}
	for i, name := range old {
		write(name, time.Duration(400+i)*24*time.Hour)
	}
	held := BackupFileName("my_proj", "full", "20250105_000000", "_keep-stack-1-ab", ".dump")
	write(held, 399*24*time.Hour)
	fresh := BackupFileName("my_proj", "full", "20261019_110000", "", ".dump")
	write(fresh, time.Minute)

	// Old files that are not backups must never be touched.
	strays := []string{".gitkeep", "notes.txt", "restore.tmp"}
	for _, name := range strays {
		write(name, 500*24*time.Hour)
	}

	cfg := &config.Config{}
	cfg.Backup.Dir = dir
	res, err := Prune(cfg, PruneOptions{KeepDaily: 1, KeepWeekly: 1, KeepMonthly: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pruned) != len(old) {
		t.Fatalf("pruned %v, want %d entries", res.Pruned, len(old))
	}
	for _, name := range old {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s was reported pruned but is still on disk (err=%v)", name, err)
		}
	}
	for _, name := range append([]string{held, fresh}, strays...) {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s must survive the prune: %v", name, err)
		}
	}
}
