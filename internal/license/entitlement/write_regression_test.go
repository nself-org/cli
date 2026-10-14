package entitlement_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license/entitlement"
)

func TestRefreshPreservesExistingInode(t *testing.T) {
	compattest.Set(t, true)
	entry := seedVerifiedCache(t)
	dir := t.TempDir()
	path := entitlement.Path(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("old", len(entry.JWT))), 0644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pinned.Close() }()
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("refresh replaced the inode pinned by a single-file bind mount")
	}
	data := make([]byte, len(entry.JWT)+1)
	n, _ := pinned.Read(data)
	if string(data[:n]) != entry.JWT {
		t.Fatal("pinned inode must contain the complete private replacement token")
	}
	if runtime.GOOS != "windows" && after.Mode().Perm() != 0600 {
		t.Fatal("refreshed inode must have Unix permissions 0600")
	}
}

func TestRefreshRepairsEmptyDirectoryAndRejectsNonempty(t *testing.T) {
	compattest.Set(t, true)
	seedVerifiedCache(t)
	for _, nonempty := range []bool{false, true} {
		dir := t.TempDir()
		path := entitlement.Path(dir)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if nonempty {
			if err := os.WriteFile(filepath.Join(path, "keep"), []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		err := entitlement.Refresh(context.Background(), dir)
		if nonempty {
			if errs.ExitCodeFor(err) != 3 || !strings.Contains(err.Error(), "directory is not empty") {
				t.Fatalf("nonempty directory requires clear E125: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("repair Docker's empty directory: %v", err)
		}
	}
}

func TestRefreshRepairsReadOnlyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are required")
	}
	compattest.Set(t, true)
	entry := seedVerifiedCache(t)
	for name, mode := range map[string]os.FileMode{"owner-read": 0400, "world-read": 0444, "owner-write": 0200} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := entitlement.Path(dir)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("old"), mode); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := entitlement.Refresh(context.Background(), dir); err != nil {
				t.Fatal(err)
			}
			after, err := os.Stat(path)
			if err != nil || !os.SameFile(before, after) || after.Mode().Perm() != 0600 {
				t.Fatalf("read-only refresh must preserve inode and normalize permissions: %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != entry.JWT {
				t.Fatalf("read-only refresh must publish the verified token: %v", err)
			}
		})
	}
}

func TestRefreshWithoutConfiguredKey(t *testing.T) {
	compattest.Set(t, true)
	seedVerifiedCache(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	dir := t.TempDir()
	if err := entitlement.Refresh(context.Background(), dir); errs.ExitCodeFor(err) != 3 {
		t.Fatalf("missing configured key requires E125: %v", err)
	}
	data, err := os.ReadFile(entitlement.Path(dir))
	if err != nil || len(data) != 0 {
		t.Fatalf("unconfigured cached JWT must not be published: %v", err)
	}
}

func TestRefreshCanceledContextIsCoded(t *testing.T) {
	compattest.Set(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := entitlement.Refresh(ctx, t.TempDir())
	if errs.ExitCodeFor(err) != 3 || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation requires wrapped E125: %v", err)
	}
}

func TestRefreshRefusesSymlink(t *testing.T) {
	compattest.Set(t, true)
	seedVerifiedCache(t)
	dir := t.TempDir()
	path := entitlement.Path(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(target, []byte("untouched"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
			t.Skip("Windows account lacks the symlink creation privilege")
		}
		t.Fatal(err)
	}
	if err := entitlement.Refresh(context.Background(), dir); errs.ExitCodeFor(err) != 3 {
		t.Fatalf("symlink requires E125: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "untouched" {
		t.Fatalf("symlink destination changed: %v", err)
	}
}
