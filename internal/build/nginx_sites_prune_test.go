package build

// nginx_sites_prune_test.go — proves the nginx/sites regeneration sweep is
// additive-safe: only marker-carrying generated files are removed, a
// hand-written conf (the production-box incident's shape — a foreign or
// hand-authored file sharing the directory) survives untouched, and the
// prior contents are snapshotted to .nself/backups/ first with old
// snapshots pruned beyond the retention window.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPruneGeneratedNginxSites_LeavesHandWrittenFileUntouched is the fixture
// test named in the fix plan: a directory with one generator-marked file
// and one hand-written conf. Only the marked file is removed; the
// hand-written one is left in place and reported as foreign.
func TestPruneGeneratedNginxSites_LeavesHandWrittenFileUntouched(t *testing.T) {
	dir := t.TempDir()
	mustWriteBuildFile(t, filepath.Join(dir, "api.conf"),
		nginxGeneratedMarker+"\nserver { listen 80; }\n")
	mustWriteBuildFile(t, filepath.Join(dir, "hand-written.conf"),
		"# hand-authored by an operator, not nself build\nserver { listen 8080; }\n")

	removed, foreign, err := pruneGeneratedNginxSites(dir)
	if err != nil {
		t.Fatalf("pruneGeneratedNginxSites: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if len(foreign) != 1 || foreign[0] != "hand-written.conf" {
		t.Errorf("foreign = %v, want [hand-written.conf]", foreign)
	}
	if _, err := os.Stat(filepath.Join(dir, "api.conf")); !os.IsNotExist(err) {
		t.Errorf("api.conf (generated) should have been removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hand-written.conf")); err != nil {
		t.Errorf("hand-written.conf should still exist: %v", err)
	}
}

// TestPruneGeneratedNginxSites_EmptyOrMissingDirIsNoop verifies the sweep
// never errors on a first build (directory does not exist yet) or an
// already-empty sites dir.
func TestPruneGeneratedNginxSites_EmptyOrMissingDirIsNoop(t *testing.T) {
	base := t.TempDir()

	if removed, foreign, err := pruneGeneratedNginxSites(filepath.Join(base, "does-not-exist")); err != nil || removed != 0 || foreign != nil {
		t.Errorf("missing dir: removed=%d foreign=%v err=%v, want 0, nil, nil", removed, foreign, err)
	}

	empty := filepath.Join(base, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if removed, foreign, err := pruneGeneratedNginxSites(empty); err != nil || removed != 0 || len(foreign) != 0 {
		t.Errorf("empty dir: removed=%d foreign=%v err=%v, want 0, [], nil", removed, foreign, err)
	}
}

// TestBackupNginxSites_SnapshotsBeforeAnythingIsRemoved verifies that
// backupNginxSites copies the current directory contents into
// .nself/backups/nginx-sites-<timestamp>/ before the caller prunes
// anything, so a wrongly-deleted foreign or generated file can be
// recovered.
func TestBackupNginxSites_SnapshotsBeforeAnythingIsRemoved(t *testing.T) {
	workdir := t.TempDir()
	sitesDir := filepath.Join(workdir, "nginx", "sites")
	if err := os.MkdirAll(sitesDir, 0o755); err != nil {
		t.Fatalf("mkdir sitesDir: %v", err)
	}
	mustWriteBuildFile(t, filepath.Join(sitesDir, "api.conf"), nginxGeneratedMarker+"\n...\n")
	mustWriteBuildFile(t, filepath.Join(sitesDir, "hand-written.conf"), "server {}\n")

	if err := backupNginxSites(workdir, sitesDir); err != nil {
		t.Fatalf("backupNginxSites: %v", err)
	}

	backupsRoot := filepath.Join(workdir, ".nself", "backups")
	entries, err := os.ReadDir(backupsRoot)
	if err != nil {
		t.Fatalf("reading backups root: %v", err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("expected exactly one snapshot dir, got %v", entries)
	}

	snapshotDir := filepath.Join(backupsRoot, entries[0].Name())
	for _, name := range []string{"api.conf", "hand-written.conf"} {
		if _, err := os.Stat(filepath.Join(snapshotDir, name)); err != nil {
			t.Errorf("snapshot missing %s: %v", name, err)
		}
	}

	// The originals must still be present too — backup is a copy, not a move.
	for _, name := range []string{"api.conf", "hand-written.conf"} {
		if _, err := os.Stat(filepath.Join(sitesDir, name)); err != nil {
			t.Errorf("original %s should be untouched by backup: %v", name, err)
		}
	}
}

// TestBackupNginxSites_EmptyOrMissingDirIsNoop verifies no snapshot is
// created (and no error raised) on a first build.
func TestBackupNginxSites_EmptyOrMissingDirIsNoop(t *testing.T) {
	workdir := t.TempDir()
	sitesDir := filepath.Join(workdir, "nginx", "sites")

	if err := backupNginxSites(workdir, sitesDir); err != nil {
		t.Fatalf("missing sitesDir: backupNginxSites: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workdir, ".nself", "backups")); !os.IsNotExist(err) {
		t.Errorf("expected no backups dir created for a missing sitesDir, stat err = %v", err)
	}

	if err := os.MkdirAll(sitesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := backupNginxSites(workdir, sitesDir); err != nil {
		t.Fatalf("empty sitesDir: backupNginxSites: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workdir, ".nself", "backups")); !os.IsNotExist(err) {
		t.Errorf("expected no backups dir created for an empty sitesDir, stat err = %v", err)
	}
}

// TestPruneOldNginxSitesBackups_KeepsOnlyNewestN verifies retention: with
// more than maxNginxSitesBackups snapshots present, only the newest
// maxNginxSitesBackups survive pruning.
func TestPruneOldNginxSitesBackups_KeepsOnlyNewestN(t *testing.T) {
	backupsRoot := t.TempDir()
	names := []string{
		"nginx-sites-20260101-000000",
		"nginx-sites-20260102-000000",
		"nginx-sites-20260103-000000",
		"nginx-sites-20260104-000000",
		"nginx-sites-20260105-000000",
		"nginx-sites-20260106-000000",
		"nginx-sites-20260107-000000",
	}
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(backupsRoot, n), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", n, err)
		}
	}

	if err := pruneOldNginxSitesBackups(backupsRoot); err != nil {
		t.Fatalf("pruneOldNginxSitesBackups: %v", err)
	}

	entries, err := os.ReadDir(backupsRoot)
	if err != nil {
		t.Fatalf("reading backupsRoot: %v", err)
	}
	if len(entries) != maxNginxSitesBackups {
		t.Fatalf("got %d snapshots remaining, want %d", len(entries), maxNginxSitesBackups)
	}
	// The newest maxNginxSitesBackups (...03 through ...07) must survive;
	// the two oldest (...01, ...02) must be gone.
	for _, stale := range names[:2] {
		if _, err := os.Stat(filepath.Join(backupsRoot, stale)); !os.IsNotExist(err) {
			t.Errorf("stale snapshot %s should have been pruned, stat err = %v", stale, err)
		}
	}
	for _, kept := range names[2:] {
		if _, err := os.Stat(filepath.Join(backupsRoot, kept)); err != nil {
			t.Errorf("recent snapshot %s should have survived pruning: %v", kept, err)
		}
	}
}

func mustWriteBuildFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
