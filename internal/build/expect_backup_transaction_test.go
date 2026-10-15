package build

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestExpectTransactionBackupsWaitForCompleteRender(t *testing.T) {
	dir := t.TempDir()
	sites := filepath.Join(dir, "nginx", "sites")
	if err := os.MkdirAll(sites, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sites, "route.conf"), []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	exp := &PlannedBuild{Files: map[string]PlannedFile{"late": {Data: []byte("planned"), Perm: 0600}}, Effects: []PlannedEffect{{Kind: EffectNginxSitesBackup}}}
	sink := newExpectSink(dir, exp)
	if err := backupNginxSitesVia(sink, dir, sites); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".nself")); !os.IsNotExist(err) {
		t.Fatalf("backup created before verification: %v", err)
	}
	if err := sink.commit(); err == nil {
		t.Fatal("accepted incomplete render")
	}
	if _, err := os.Stat(filepath.Join(dir, ".nself")); !os.IsNotExist(err) {
		t.Fatalf("backup leaked on mismatch: %v", err)
	}
	if err := sink.WriteAtomic("late", []byte("planned"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := sink.commit(); err != nil {
		t.Fatal(err)
	}
	snapshots, err := os.ReadDir(filepath.Join(dir, ".nself", "backups"))
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("missing published snapshot: %v %v", snapshots, err)
	}
	assertArtifact(t, filepath.Join(dir, ".nself", "backups", snapshots[0].Name(), "route.conf"), "original", 0644)
}

func TestExpectTransactionBackupsRetentionAndRollback(t *testing.T) {
	dir := t.TempDir()
	sites := filepath.Join(dir, "nginx", "sites")
	if err := os.MkdirAll(sites, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sites, "route.conf"), []byte("new snapshot"), 0644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxNginxSitesBackups; i++ {
		snapshot := filepath.Join(dir, ".nself", "backups", fmt.Sprintf("nginx-sites-20200101-%06d", i))
		if err := os.MkdirAll(snapshot, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(snapshot, "route.conf"), []byte("old snapshot"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	sink := newExpectSink(dir, &PlannedBuild{Effects: []PlannedEffect{{Kind: EffectNginxSitesBackup}}})
	if err := backupNginxSitesVia(sink, dir, sites); err != nil {
		t.Fatal(err)
	}
	if err := sink.commit(func() error { return fmt.Errorf("reject") }); err == nil {
		t.Fatal("accepted rejected transaction")
	}
	for i := 0; i < maxNginxSitesBackups; i++ {
		assertArtifact(t, filepath.Join(dir, ".nself", "backups", fmt.Sprintf("nginx-sites-20200101-%06d", i), "route.conf"), "old snapshot", 0644)
	}
	if err := sink.commit(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".nself", "backups"))
	if err != nil || len(entries) != maxNginxSitesBackups {
		t.Fatalf("retention: %d snapshots, %v", len(entries), err)
	}
}

func TestExpectTransactionFrontingAndExternalReads(t *testing.T) {
	dir, fronting, external := t.TempDir(), t.TempDir(), t.TempDir()
	path := filepath.Join(external, "read-only")
	if err := os.WriteFile(path, []byte("external"), 0600); err != nil {
		t.Fatal(err)
	}
	sink := newExpectSink(dir, &PlannedBuild{Files: map[string]PlannedFile{"@fronting/route.conf": {Data: []byte("route"), Perm: 0644}}})
	sink.setFronting(fronting)
	if got, err := sink.ReadFile(path); err != nil || string(got) != "external" {
		t.Fatalf("external read: %q %v", got, err)
	}
	if err := sink.WriteAtomic(filepath.Join(fronting, "route.conf"), []byte("route"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fronting, "route.conf")); !os.IsNotExist(err) {
		t.Fatal("fronting write escaped staging")
	}
	if err := sink.commit(); err != nil {
		t.Fatal(err)
	}
	assertArtifact(t, filepath.Join(fronting, "route.conf"), "route", 0644)
}
