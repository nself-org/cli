package build

// Purpose: prove prepareProxyOutputDirs and the empty-directory cleanup (D-0782).

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrepareProxyOutputDirs(t *testing.T) {
	owner := map[string]string{"NSELF_HOST_UID": "1", "NSELF_HOST_GID": "1"}
	t.Run("nil owner creates nothing", func(t *testing.T) {
		workdir := t.TempDir()
		if err := prepareProxyOutputDirs(newDiskSink(workdir), workdir, nil); err != nil {
			t.Fatal(err)
		}
		if exists(filepath.Join(workdir, ".nself")) {
			t.Fatal("created directories without a host owner")
		}
	})
	t.Run("creates both directories user-owned and 0755", func(t *testing.T) {
		workdir := t.TempDir()
		if err := prepareProxyOutputDirs(newDiskSink(workdir), workdir, owner); err != nil {
			t.Fatal(err)
		}
		for _, name := range proxyOutputNames {
			fi, err := os.Lstat(proxyOutputPath(workdir, name))
			if err != nil || !fi.IsDir() {
				t.Fatalf("%s not created: %v", name, err)
			}
			if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o755 {
				t.Fatalf("%s mode %v", name, fi.Mode().Perm())
			}
		}
	})
	t.Run("leaves existing content and a symlink alone", func(t *testing.T) {
		workdir := t.TempDir()
		dir := proxyOutputDir(workdir)
		writeFixtureFile(t, filepath.Join(dir, "dynamic.yml"), "# keep\n", 0644)
		other := t.TempDir()
		if err := os.MkdirAll(filepath.Dir(proxyOutputPath(workdir, "traefik-static")), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(other, proxyOutputPath(workdir, "traefik-static")); err != nil {
			t.Fatal(err)
		}
		if err := prepareProxyOutputDirs(newDiskSink(workdir), workdir, owner); err != nil {
			t.Fatal(err)
		}
		if !exists(filepath.Join(dir, "dynamic.yml")) {
			t.Fatal("existing file lost")
		}
		if fi, _ := os.Lstat(proxyOutputPath(workdir, "traefik-static")); fi.Mode()&os.ModeSymlink == 0 {
			t.Fatal("symlink replaced")
		}
	})
	t.Run("plan mode only records", func(t *testing.T) {
		workdir := t.TempDir()
		if err := prepareProxyOutputDirs(newMemSink(workdir, ""), workdir, owner); err != nil {
			t.Fatal(err)
		}
		if exists(proxyOutputDir(workdir)) {
			t.Fatal("plan mode created a directory")
		}
	})
}

func TestProxyRestoreEmptyPrecreatedDirRemoved(t *testing.T) {
	workdir := t.TempDir()
	if err := prepareProxyOutputDirs(newDiskSink(workdir), workdir, map[string]string{"NSELF_HOST_UID": "1"}); err != nil {
		t.Fatal(err)
	}
	if err := cleanupProviderOutput(newDiskSink(workdir), workdir, ProviderNginx); err != nil {
		t.Fatal(err)
	}
	for _, name := range proxyOutputNames {
		if exists(proxyOutputPath(workdir, name)) {
			t.Fatalf("empty pre-created %s survived the swap to nginx", name)
		}
	}
}

func TestProxyRestoreEmptyDirBehindSymlinkKept(t *testing.T) {
	workdir := t.TempDir()
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".nself", "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, proxyOutputDir(workdir)); err != nil {
		t.Fatal(err)
	}
	if err := cleanupProviderOutput(newDiskSink(workdir), workdir, ProviderNginx); err != nil {
		t.Fatal(err)
	}
	if !exists(target) {
		t.Fatal("symlink target removed")
	}
}
