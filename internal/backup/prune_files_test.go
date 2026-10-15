package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// A listed ID drops the whole backup extension (.dump, .tar.gz, then .age) and
// the tag of a .tar.gz metadata backup parses without a ".tar" tail (D-0710).
func TestBackupIDStripsFullExtension(t *testing.T) {
	cases := []struct{ name, id, tag string }{
		{BackupFileName("my_proj", "full", "20261012_150405", "_keep-stack-1", ".dump"), "my_proj_full_20261012_150405_keep-stack-1", "keep-stack-1"},
		{BackupFileName("my_proj", "full", "20261012_150405", "", ".dump") + ".age", "my_proj_full_20261012_150405", ""},
		{BackupFileName("proj", "metadata", "20261012_150405", "", ".tar.gz"), "proj_metadata_20261012_150405", ""},
		{BackupFileName("proj", "metadata", "20261012_150405", "_keep", ".tar.gz") + ".age", "proj_metadata_20261012_150405_keep", "keep"},
		{"manual.sql", "manual", ""},
	}
	for _, c := range cases {
		e := newBackupEntry(c.name, 1, time.Now())
		if e.ID != c.id || e.Tag != c.tag {
			t.Errorf("%s: ID=%q Tag=%q, want ID=%q Tag=%q", c.name, e.ID, e.Tag, c.id, c.tag)
		}
	}
}

// A real prune (not a dry run) removes the old .dump, .dump.age and .tar.gz
// files it reports as pruned, and leaves held and recent ones (D-0710).
func TestPruneDeletesPrunedFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	write := func(name string, age time.Duration) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
		return p
	}
	oldDump := write(BackupFileName("p", "full", "20260101_000000", "", ".dump"), 5*time.Hour)
	oldAge := write(BackupFileName("p", "full", "20260101_010000", "", ".dump")+".age", 4*time.Hour)
	oldTar := write(BackupFileName("p", "metadata", "20260101_020000", "", ".tar.gz"), 3*time.Hour)
	heldTar := write(BackupFileName("p", "metadata", "20260101_025000", "_keep", ".tar.gz"), 2*time.Hour)
	heldDump := write(BackupFileName("p", "full", "20260101_030000", "_keep-stack-1", ".dump"), 110*time.Minute)
	newest := write(BackupFileName("p", "full", "20260101_040000", "", ".dump"), time.Minute)
	cfg := &config.Config{}
	cfg.Backup.Dir = dir
	res, err := Prune(cfg, PruneOptions{KeepDaily: 1, KeepWeekly: 1, KeepMonthly: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pruned) != 3 {
		t.Fatalf("pruned = %v, want the 3 old untagged backups", res.Pruned)
	}
	for _, p := range []string{oldDump, oldAge, oldTar} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s reported pruned but still on disk (err=%v)", filepath.Base(p), err)
		}
	}
	for _, p := range []string{heldTar, heldDump, newest} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s must survive: %v", filepath.Base(p), err)
		}
	}
}

// Every listed ID still resolves to its file for restore (D-0710).
func TestListedIDResolvesForRestore(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{BackupFileName("p", "full", "20260101_000000", "", ".dump") + ".age", BackupFileName("p", "metadata", "20260101_010000", "", ".tar.gz")} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{}
	cfg.Backup.Dir = dir
	list, err := List(cfg, ListOptions{})
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %v, %v", list, err)
	}
	for _, e := range list {
		got, err := resolveBackupFile(dir, e.ID)
		if err != nil || filepath.Base(got) == "" {
			t.Errorf("ID %q does not resolve: %v", e.ID, err)
		}
	}
}
