package sharedproxy

// Tests for the failure branches of Lock, tryLock and Write: an owner dir that
// cannot be locked, a cancelled wait, and a registration write that fails
// without leaving a half-written file.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLockRefusals(t *testing.T) {
	ctx := context.Background()
	t.Run("owner dir missing", func(t *testing.T) {
		_, err := Lock(ctx, filepath.Join(t.TempDir(), "gone"))
		if !isE506(err) || !strings.Contains(err.Error(), "cannot open") {
			t.Errorf("want an E506, got %v", err)
		}
	})
	t.Run("nginx/shared is a symlink", func(t *testing.T) {
		owner, outside := t.TempDir(), t.TempDir()
		if err := os.MkdirAll(filepath.Join(owner, "nginx"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, DirOf(owner)); err != nil {
			t.Skip("symlinks unavailable:", err)
		}
		if _, err := Lock(ctx, owner); !isE506(err) {
			t.Errorf("a symlinked shared dir must be refused: %v", err)
		}
		if entries, _ := os.ReadDir(outside); len(entries) != 0 {
			t.Errorf("no lock file may be created through the symlink: %v", entries)
		}
	})
	t.Run("the lock path is a directory", func(t *testing.T) {
		owner := t.TempDir()
		if err := os.MkdirAll(filepath.Join(DirOf(owner), LockFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := Lock(ctx, owner); !isE506(err) || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("want a not-a-regular-file E506, got %v", err)
		}
	})
	t.Run("the lock file cannot be created", func(t *testing.T) {
		skipIfRoot(t)
		owner := t.TempDir()
		shared := DirOf(owner)
		if err := os.MkdirAll(shared, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(shared, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(shared, 0o755) })
		if _, err := Lock(ctx, owner); !isE506(err) || !strings.Contains(err.Error(), "lock file") {
			t.Errorf("want an E506 about the lock file, got %v", err)
		}
	})
}

func TestLockWaitCancelled(t *testing.T) {
	f := newProxyFixture(t)
	f.owner0()
	release, err := Lock(context.Background(), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = Lock(ctx, f.owner)
	if !isE506(err) || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("a cancelled wait must be an E506, got %v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Error("the wait must end when the context ends, not when lockWait does")
	}
}

func TestTryLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	a, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	unlock, ok, err := tryLock(a, path)
	if err != nil || !ok {
		t.Fatalf("first lock: %v %v", ok, err)
	}
	if _, ok, err := tryLock(b, path); ok || err != nil {
		t.Errorf("a held lock is 'not ok' with no error: %v %v", ok, err)
	}
	unlock()
	if unlock2, ok, err := tryLock(b, path); !ok || err != nil {
		t.Errorf("after unlock the other descriptor can lock: %v %v", ok, err)
	} else {
		unlock2()
	}
	_ = a.Close()
	if _, ok, err := tryLock(a, path); ok || err == nil {
		t.Errorf("a closed descriptor must report an error, got ok=%v err=%v", ok, err)
	}
}

func TestWriteFailures(t *testing.T) {
	reg := Registration{Project: "demo", ProjectDir: "/p", Files: []string{"nginx/sites/a.conf"}}
	t.Run("the directory is read-only", func(t *testing.T) {
		skipIfRoot(t)
		dir := t.TempDir()
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		if err := Write(dir, FileName("demo"), reg); err == nil {
			t.Fatal("a read-only directory must fail")
		}
		if _, err := os.Stat(filepath.Join(dir, FileName("demo"))); err == nil {
			t.Error("no registration may exist after a failed write")
		}
	})
	t.Run("the target is a non-empty directory", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, FileName("demo"))
		if err := os.MkdirAll(filepath.Join(target, "child"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := Write(dir, FileName("demo"), reg); err == nil {
			t.Fatal("renaming over a directory must fail")
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 1 {
			t.Errorf("the temp file must be removed after a failed rename: %v", entries)
		}
	})
	t.Run("an existing registration is replaced whole", func(t *testing.T) {
		dir := t.TempDir()
		if err := Write(dir, FileName("demo"), reg); err != nil {
			t.Fatal(err)
		}
		reg2 := reg
		reg2.ProjectDir = "/other"
		if err := Write(dir, FileName("demo"), reg2); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, FileName("demo")))
		if err != nil || !strings.Contains(string(got), "/other") || strings.Contains(string(got), `"/p"`) {
			t.Errorf("the second write must replace the first: %s %v", got, err)
		}
	})
}
