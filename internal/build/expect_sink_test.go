package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExpectSinkBackupOnlyWithEffect: nginx/sites snapshot writes and prunes
// under .nself/backups are allowed only when the confirmed render carries the
// nginx-sites-backup effect, and nowhere else.
func TestExpectSinkBackupOnlyWithEffect(t *testing.T) {
	dir := t.TempDir()
	bk := filepath.Join(dir, ".nself", "backups", "nginx-sites-20260101-000000")
	other := filepath.Join(dir, "nginx", "sites", "x.conf")
	for _, with := range []bool{true, false} {
		exp := &PlannedBuild{Files: map[string]PlannedFile{}}
		if with {
			exp.Effects = []PlannedEffect{{Kind: EffectNginxSitesBackup}}
		}
		es := newExpectSink(dir, exp)
		if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(bk, 0o755); err != nil {
			t.Fatal(err)
		}
		err := es.WriteFile(filepath.Join(bk, "a.conf"), []byte("x"), 0o644)
		if with && err != nil {
			t.Fatalf("snapshot write must be allowed with the effect: %v", err)
		}
		if !with && (err == nil || !strings.Contains(err.Error(), "was not in the plan")) {
			t.Fatalf("snapshot write without the effect must be refused, got %v", err)
		}
		if err := es.WriteFile(other, []byte("x"), 0o644); err == nil {
			t.Fatal("a write outside the plan and the backup dir must be refused")
		}
		rerr := es.Remove(filepath.Join(bk, "a.conf"))
		if with && rerr != nil && !os.IsNotExist(rerr) {
			t.Fatalf("snapshot prune must be allowed with the effect: %v", rerr)
		}
		if !with && rerr == nil {
			t.Fatal("a removal under backups without the effect must be refused")
		}
	}
}
