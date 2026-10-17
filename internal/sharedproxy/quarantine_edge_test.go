package sharedproxy

// Tests for the refusal and skip branches of quarantine.go: paths outside the
// owner, files that are missing or not regular, symlinked ancestors, and the
// permission failures of the directory helpers.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply to root")
	}
}

func openRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func loadSet(t *testing.T, f *proxyFixture) Set {
	t.Helper()
	set, err := Load(DirOf(f.owner), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestOwnerRel(t *testing.T) {
	owner := filepath.Join(string(filepath.Separator), "srv", "owner")
	rel, err := ownerRel(owner, filepath.Join(owner, "nginx", "shared"))
	if err != nil || rel != filepath.Join("nginx", "shared") {
		t.Errorf("a path inside the owner: %q %v", rel, err)
	}
	for _, outside := range []string{filepath.Join(string(filepath.Separator), "srv", "other"), filepath.Dir(owner)} {
		if rel, err := ownerRel(owner, outside); err == nil {
			t.Errorf("%s is outside the owner but got %q", outside, rel)
		}
	}
}

func TestRestoreModeKeepsKeysPrivate(t *testing.T) {
	for rel, want := range map[string]os.FileMode{
		"ssl/certificates/a/privkey.pem":   0o600,
		"ssl/certificates/a/" + CertMarker: 0o600,
		"ssl/certificates/a/site.key":      0o600,
		"ssl/certificates/a/fullchain.pem": 0o644,
		"nginx/sites/a.conf":               0o644,
	} {
		if got := restoreMode(rel); got != want {
			t.Errorf("restoreMode(%s) = %v, want %v", rel, got, want)
		}
	}
}

func TestQuarantineRefusals(t *testing.T) {
	t.Run("owner dir missing", func(t *testing.T) {
		_, err := quarantine(filepath.Join(t.TempDir(), "gone"), Set{}, Registration{Project: "b"})
		if err == nil {
			t.Error("a missing owner dir must fail")
		}
	})
	t.Run("project name that climbs out of the owner", func(t *testing.T) {
		f := newProxyFixture(t)
		f.owner0()
		reg := Registration{Project: strings.Repeat("../", 8) + "escape"}
		moved, err := quarantine(f.owner, Set{}, reg)
		if err == nil || len(moved) != 0 || !strings.Contains(err.Error(), "outside") {
			t.Errorf("want an 'outside' error, got %v %v", moved, err)
		}
	})
	t.Run("a registered file that is gone is skipped", func(t *testing.T) {
		f := newProxyFixture(t)
		f.owner0()
		b := f.project("b")
		if err := os.Remove(filepath.Join(f.owner, "ssl", "certificates", "b-cert", "privkey.pem")); err != nil {
			t.Fatal(err)
		}
		moved, err := quarantine(f.owner, loadSet(t, f), b)
		if err != nil || len(moved) != 2 {
			t.Fatalf("the two files that exist must move: %v %v", moved, err)
		}
		for _, m := range moved {
			if strings.HasSuffix(m, "privkey.pem") {
				t.Errorf("a file that does not exist cannot have moved: %v", moved)
			}
		}
	})
	t.Run("a directory where a file is registered", func(t *testing.T) {
		f := newProxyFixture(t)
		f.owner0()
		b := f.project("b")
		full := filepath.Join(f.owner, "ssl", "certificates", "b-cert", "fullchain.pem")
		if err := os.Remove(full); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(full, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := quarantine(f.owner, loadSet(t, f), b)
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("want a not-a-regular-file error, got %v", err)
		}
		if fi, serr := os.Stat(full); serr != nil || !fi.IsDir() {
			t.Errorf("the directory must stay in place: %v", serr)
		}
	})
	t.Run("the destination is a non-empty directory", func(t *testing.T) {
		f := newProxyFixture(t)
		f.owner0()
		b := f.project("b")
		dst := filepath.Join(quarantineBase(f.owner, "b"), "nginx", "sites", "b.api.conf")
		f.write(filepath.Join(dst, "child"), "x")
		_, err := quarantine(f.owner, loadSet(t, f), b)
		if err == nil || !strings.Contains(err.Error(), "quarantine nginx/sites/b.api.conf") {
			t.Fatalf("want the rename failure named, got %v", err)
		}
		if !f.live("b") {
			t.Error("a file that could not be moved must stay live")
		}
	})
}

func TestRestoreSkipsWhatItMustNotTouch(t *testing.T) {
	site := "nginx/sites/b.api.conf"
	t.Run("an invalid registered path", func(t *testing.T) {
		f := newProxyFixture(t)
		f.owner0()
		reg := Registration{Project: "b", Files: []string{"../../etc/passwd", "nginx/sites/../x"}}
		f.write(filepath.Join(quarantineBase(f.owner, "b"), "marker"), "x")
		back, err := restore(f.owner, reg)
		if err != nil || len(back) != 0 {
			t.Errorf("invalid paths are skipped: %v %v", back, err)
		}
	})
	t.Run("a quarantine subdirectory that is a symlink", func(t *testing.T) {
		f := newProxyFixture(t)
		f.owner0()
		outside := t.TempDir()
		f.write(filepath.Join(outside, "sites", "b.api.conf"), "from outside")
		base := quarantineBase(f.owner, "b")
		if err := os.MkdirAll(base, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(base, "nginx")); err != nil {
			t.Skip("symlinks unavailable:", err)
		}
		back, err := restore(f.owner, Registration{Project: "b", Files: []string{site}})
		if err != nil || len(back) != 0 {
			t.Fatalf("a symlinked subdirectory is left alone: %v %v", back, err)
		}
		if _, err := os.Lstat(filepath.Join(f.owner, filepath.FromSlash(site))); err == nil {
			t.Error("nothing may be restored from outside the owner")
		}
	})
	t.Run("a quarantined entry that is a directory", func(t *testing.T) {
		f := newProxyFixture(t)
		f.owner0()
		f.write(filepath.Join(quarantineBase(f.owner, "b"), filepath.FromSlash(site), "child"), "x")
		back, err := restore(f.owner, Registration{Project: "b", Files: []string{site}})
		if err != nil || len(back) != 0 {
			t.Errorf("a directory is not a quarantined file: %v %v", back, err)
		}
	})
	t.Run("a live file already exists", func(t *testing.T) {
		f := newProxyFixture(t)
		f.owner0()
		q := filepath.Join(quarantineBase(f.owner, "b"), filepath.FromSlash(site))
		f.write(q, "stale")
		live := filepath.Join(f.owner, filepath.FromSlash(site))
		f.write(live, "rebuilt")
		back, err := restore(f.owner, Registration{Project: "b", Files: []string{site}})
		if err != nil || len(back) != 0 {
			t.Fatalf("the rebuilt file wins: %v %v", back, err)
		}
		got, _ := os.ReadFile(live)
		if string(got) != "rebuilt" {
			t.Errorf("the live file was overwritten with %q", got)
		}
		if _, err := os.Lstat(q); err == nil {
			t.Error("the stale quarantined copy must be deleted")
		}
	})
	t.Run("the live directory is a symlink", func(t *testing.T) {
		f := newProxyFixture(t)
		f.owner0()
		outside := t.TempDir()
		f.write(filepath.Join(quarantineBase(f.owner, "b"), filepath.FromSlash(site)), "q")
		if err := os.MkdirAll(filepath.Join(f.owner, "nginx"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(f.owner, "nginx", "sites")); err != nil {
			t.Skip("symlinks unavailable:", err)
		}
		back, err := restore(f.owner, Registration{Project: "b", Files: []string{site}})
		if err != nil || len(back) != 0 {
			t.Fatalf("a symlinked destination is skipped: %v %v", back, err)
		}
		if entries, _ := os.ReadDir(outside); len(entries) != 0 {
			t.Errorf("nothing may be written through the symlink: %v", entries)
		}
	})
}

func TestRestoreFailures(t *testing.T) {
	site := "nginx/sites/b.api.conf"
	t.Run("a file is where the directory must be", func(t *testing.T) {
		f := newProxyFixture(t)
		f.owner0()
		f.write(filepath.Join(quarantineBase(f.owner, "b"), filepath.FromSlash(site)), "q")
		f.write(filepath.Join(f.owner, "nginx", "sites"), "a file")
		back, err := restore(f.owner, Registration{Project: "b", Files: []string{site}})
		if err == nil || len(back) != 0 {
			t.Errorf("want an error and nothing restored: %v %v", back, err)
		}
	})
	t.Run("the destination directory is read-only", func(t *testing.T) {
		skipOnWindows(t)
		skipIfRoot(t)
		f := newProxyFixture(t)
		f.owner0()
		q := filepath.Join(quarantineBase(f.owner, "b"), filepath.FromSlash(site))
		f.write(q, "q")
		dir := filepath.Join(f.owner, "nginx", "sites")
		if err := os.MkdirAll(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		back, err := restore(f.owner, Registration{Project: "b", Files: []string{site}})
		if err == nil || !strings.Contains(err.Error(), "restore "+site) || len(back) != 0 {
			t.Fatalf("want the rename failure named: %v %v", back, err)
		}
		if _, err := os.Stat(q); err != nil {
			t.Errorf("the quarantined copy must survive a failed restore: %v", err)
		}
	})
}
