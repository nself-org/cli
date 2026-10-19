package backup

import (
	"os"
	"path/filepath"
	"strings"
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

// realName builds a backup file name with the same function create.go uses.
func realName(project, ts, tag string) string {
	if tag != "" {
		tag = "_" + tag
	}
	return BackupFileName(project, "full", ts, tag, ".dump")
}

// parseBackupTag reads the tag back from real names, for project names with
// and without "_", for no tag, the bare keep tag and the stack keep- tag.
func TestParseBackupTagRealNames(t *testing.T) {
	for _, project := range []string{"proj", "my_proj", "a_full_b"} {
		for _, tag := range []string{"", "keep", "keep-stack-1-ab", "pre_upgrade"} {
			name := realName(project, "20261012_150405", tag)
			e := newBackupEntry(name, 1, time.Now())
			if e.Tag != tag {
				t.Errorf("%s: Tag = %q, want %q", name, e.Tag, tag)
			}
			if e.Type != "full" {
				t.Errorf("%s: Type = %q, want full", name, e.Type)
			}
			if got := isKeepTag(e.Tag); got != (tag == "keep" || tag == "keep-stack-1-ab") {
				t.Errorf("%s: isKeepTag = %v", name, got)
			}
			if enc := newBackupEntry(name+".age", 1, time.Now()); enc.Tag != tag {
				t.Errorf("%s.age: Tag = %q, want %q", name, enc.Tag, tag)
			}
		}
	}
	// Names outside create.go's layout keep the legacy split.
	for name, want := range map[string]string{"p_full_20260101T000000Z_keep-x.dump": "keep-x", "manual.dump": "", "a_b_c.dump": ""} {
		if got := newBackupEntry(name, 1, time.Now()).Tag; got != want {
			t.Errorf("%s: Tag = %q, want %q", name, got, want)
		}
	}
}

// Prune with a one-day retention lists the older dumps for pruning but never
// a keep or keep- tagged one, even when a newer dump exists (dry run). Names
// come from the real naming function, with a "_" in the project name.
func TestPruneKeepsKeepPrefixTag(t *testing.T) {
	dir := t.TempDir()
	// Noon today: the ages below (up to 3 h) then stay on one calendar day, so the daily
	// bucket does not depend on the wall-clock hour the test runs at (it failed 00:00-03:00 UTC).
	n := time.Now()
	now := time.Date(n.Year(), n.Month(), n.Day(), 12, 0, 0, 0, n.Location())
	write := func(name string, age time.Duration) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("PGDMP"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
		return strings.TrimSuffix(name, ".dump")
	}
	stack := write(realName("my_proj", "20260101_000000", "keep-stack-1-ab"), 3*time.Hour)
	bare := write(realName("my_proj", "20260101_000500", "keep"), 150*time.Minute)
	plain := write(realName("my_proj", "20260101_010000", "stack-1-ab"), 90*time.Minute)
	write(realName("my_proj", "20260101_020000", ""), time.Minute)
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
	for _, id := range []string{stack, bare} {
		if !has(res.Kept, id) || has(res.Pruned, id) {
			t.Fatalf("%s is not held: kept=%v pruned=%v", id, res.Kept, res.Pruned)
		}
	}
	if !has(res.Pruned, plain) {
		t.Fatalf("control: the untagged older backup must be pruned: kept=%v pruned=%v", res.Kept, res.Pruned)
	}
}
