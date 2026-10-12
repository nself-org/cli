package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// An isKeepTag table: "keep" and "keep-*" hold a backup, look-alikes do not.
func TestIsKeepTag(t *testing.T) {
	for tag, want := range map[string]bool{"keep": true, "keep-stack-1-ab": true, "stack-1-ab": false, "keeper": false, "": false, "pre-keep": false} {
		if got := isKeepTag(tag); got != want {
			t.Errorf("isKeepTag(%q) = %v, want %v", tag, got, want)
		}
	}
}

// Prune with a one-day retention lists the older same-day dump for pruning
// but never a keep- tagged one, even when a newer dump exists (dry run).
func TestPruneKeepsKeepPrefixTag(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	write := func(name string, age time.Duration) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("PGDMP"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	write("p_full_20260101T000000Z_keep-stack-1-ab.dump", 2*time.Hour)
	write("p_full_20260101T010000Z_stack-1-ab.dump", 90*time.Minute)
	write("p_full_20260101T020000Z_newest.dump", time.Minute)
	cfg := &config.Config{}
	cfg.Backup.Dir = dir
	res, err := Prune(cfg, PruneOptions{DryRun: true, KeepDaily: 1, KeepWeekly: 1, KeepMonthly: 1})
	if err != nil {
		t.Fatal(err)
	}
	has := func(list []string, id string) bool {
		for _, v := range list {
			if v == id {
				return true
			}
		}
		return false
	}
	if !has(res.Kept, "p_full_20260101T000000Z_keep-stack-1-ab") || has(res.Pruned, "p_full_20260101T000000Z_keep-stack-1-ab") {
		t.Fatalf("keep- tagged backup is not held: kept=%v pruned=%v", res.Kept, res.Pruned)
	}
	if !has(res.Pruned, "p_full_20260101T010000Z_stack-1-ab") {
		t.Fatalf("control: the untagged older backup must be pruned: kept=%v pruned=%v", res.Kept, res.Pruned)
	}
}
