package build

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
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

// TestExpectVerifyIsCodedAndChecksModes (review F1): after the build every
// planned file must exist with its bytes and mode; otherwise the build fails
// with E452, never "applied".
func TestExpectVerifyIsCodedAndChecksModes(t *testing.T) {
	dir := t.TempDir()
	exp := &PlannedBuild{Files: map[string]PlannedFile{"a.txt": {Data: []byte("x"), Perm: 0o600}}}
	es := newExpectSink(dir, exp)
	if err := es.verify(); err == nil || errs.Describe(err) == nil || errs.Describe(err).Code != "E452" {
		t.Fatalf("a missing planned file must be E452, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := es.verify(); err == nil || errs.Describe(err) == nil || errs.Describe(err).Code != "E452" {
		t.Fatalf("wrong content must be E452, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "a.txt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := es.verify(); err == nil || errs.Describe(err) == nil || errs.Describe(err).Code != "E452" {
			t.Fatalf("a wrong mode must be E452, got %v", err)
		}
	}
	if err := os.Chmod(filepath.Join(dir, "a.txt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := es.verify(); err != nil {
		t.Fatalf("a matching file must verify: %v", err)
	}
}

// TestBackupRefusesSymlinkedBackupDir (review S): snapshot writes and prunes
// need .nself/backups to be a real directory inside the project; a symlinked
// component is refused by the sink and by the backup itself.
func TestBackupRefusesSymlinkedBackupDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir, outside := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, ".nself", "backups")); err != nil {
		t.Fatal(err)
	}
	sites := filepath.Join(dir, "nginx", "sites")
	if err := os.MkdirAll(sites, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sites, "a.conf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "nginx-sites-20260101-000000"), 0o755); err != nil {
		t.Fatal(err)
	}
	es := newExpectSink(dir, &PlannedBuild{Files: map[string]PlannedFile{}, Effects: []PlannedEffect{{Kind: EffectNginxSitesBackup}}})
	bk := filepath.Join(dir, ".nself", "backups", "nginx-sites-20260101-000000")
	if err := es.WriteFile(filepath.Join(bk, "a.conf"), []byte("x"), 0o644); err == nil {
		t.Fatal("the sink wrote through a symlinked backups dir")
	}
	if err := backupNginxSitesVia(newDiskSink(dir), dir, sites); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("the backup must refuse a symlinked backups dir, got %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(outside, "nginx-sites-20260101-000000")); len(entries) != 0 {
		t.Fatalf("a backup escaped to %s: %v", outside, entries)
	}
}

// TestBackupSurvivesSymlinkSwap (review round 4): .nself/backups is swapped for a
// symlink to an outside directory after the check and before the write; the
// os.Root-confined write fails and nothing lands outside the project.
func TestBackupSurvivesSymlinkSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir, outside := t.TempDir(), t.TempDir()
	sites := filepath.Join(dir, "nginx", "sites")
	for _, d := range []string{sites, filepath.Join(dir, ".nself", "backups")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(sites, "a.conf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	backupBeforeWrite = func() {
		_ = os.RemoveAll(filepath.Join(dir, ".nself", "backups"))
		_ = os.Symlink(outside, filepath.Join(dir, ".nself", "backups"))
	}
	t.Cleanup(func() { backupBeforeWrite = nil })
	if err := backupNginxSitesVia(newDiskSink(dir), dir, sites); err == nil {
		t.Fatal("a backup through a swapped-in symlink must fail")
	}
	var found []string
	_ = filepath.WalkDir(outside, func(p string, d os.DirEntry, err error) error {
		if err == nil && p != outside {
			found = append(found, p)
		}
		return nil
	})
	if len(found) != 0 {
		t.Fatalf("files were written outside the project: %v", found)
	}
}
