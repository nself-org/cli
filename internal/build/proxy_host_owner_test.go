package build

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/docker"
)

// stubDaemonRemapsUIDs makes the daemon report remapped (or not) for one test.
func stubDaemonRemapsUIDs(t *testing.T, remapped bool) {
	t.Helper()
	prev := docker.RemapsUIDsOnce
	docker.RemapsUIDsOnce = func() bool { return remapped }
	t.Cleanup(func() { docker.RemapsUIDsOnce = prev })
}

// TestProxyHostOwnerRemappedDaemon runs the whole build against a stubbed
// rootless daemon: no owner ids reach compose.env and no directory is pre-created.
func TestProxyHostOwnerRemappedDaemon(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the export is Linux only; internal/docker tests the daemon rule")
	}
	for _, name := range []string{"rootless", "userns"} {
		t.Run(name, func(t *testing.T) {
			stubDaemonRemapsUIDs(t, true)
			f := freshProxyFixture(t)
			installProxy(t, f.plugins)
			if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(f.workdir, ".nself", "compose.env"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "NSELF_HOST_") {
				t.Fatalf("%s daemon still got a host owner:\n%s", name, data)
			}
			for _, name := range proxyOutputNames {
				if exists(proxyOutputPath(f.workdir, name)) {
					t.Fatalf("%s pre-created for a daemon that remaps uids", name)
				}
			}
		})
	}
}

// TestProxyHostOwner covers the NSELF_HOST_UID/GID export the Traefik fragment reads.
func TestProxyHostOwner(t *testing.T) {
	read := func(t *testing.T, f *planFixture) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(f.workdir, ".nself", "compose.env"))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	t.Run("traefik exports the build user on linux only", func(t *testing.T) {
		stubDaemonRemapsUIDs(t, false)
		f := freshProxyFixture(t)
		installProxy(t, f.plugins)
		if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
			t.Fatal(err)
		}
		env := read(t, f)
		want := runtime.GOOS == "linux"
		uid := "NSELF_HOST_UID=" + strconv.Itoa(os.Getuid())
		gid := "NSELF_HOST_GID=" + strconv.Itoa(os.Getgid())
		if got := strings.Contains(env, uid) && strings.Contains(env, gid); got != want {
			t.Fatalf("host owner exported = %v, want %v\n%s", got, want, env)
		}
		// D-0782: the renderer's directories exist, user-owned, before Docker can create them.
		for _, name := range proxyOutputNames {
			if got := exists(proxyOutputPath(f.workdir, name)); got != want {
				t.Fatalf("%s pre-created = %v, want %v", name, got, want)
			}
		}
	})
	t.Run("nginx compose.env carries no host owner", func(t *testing.T) {
		f := freshProxyFixture(t)
		if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
			t.Fatal(err)
		}
		if env := read(t, f); strings.Contains(env, "NSELF_HOST_") {
			t.Fatalf("nginx project exports a host owner:\n%s", env)
		}
	})
	t.Run("withHostOwner does not mutate its input", func(t *testing.T) {
		vars := map[string]string{"A": "1"}
		got := withHostOwner(vars, map[string]string{"NSELF_HOST_UID": "7"})
		if _, leaked := vars["NSELF_HOST_UID"]; leaked || got["A"] != "1" || got["NSELF_HOST_UID"] != "7" {
			t.Fatalf("vars=%v got=%v", vars, got)
		}
		if same := withHostOwner(vars, nil); len(same) != 1 {
			t.Fatalf("nil owner changed vars: %v", same)
		}
	})
}
