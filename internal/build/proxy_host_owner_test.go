package build

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

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

func TestProxyHostOwnerRemappedDaemon(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("host owner export is Linux-only")
	}
	for _, tc := range []struct {
		name, response string
		wantOwner      bool
	}{
		{"rootful", `printf '%s' '["name=seccomp,profile=builtin"]'`, true},
		{"rootless", `printf '%s' '["name=rootless"]'`, false},
		{"userns", `printf '%s' '["name=userns"]'`, false},
		{"offline", "exit 1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := freshProxyFixture(t)
			installProxy(t, f.plugins)
			writeFixtureFile(t, filepath.Join(f.root, "stub-bin", "docker"), "#!/bin/sh\n"+tc.response+"\n", 0755)
			owner := proxyHostOwner(nil, f.workdir, f.plugins)
			if got := len(owner) != 0; got != tc.wantOwner {
				t.Fatalf("owner=%v, want export=%v", owner, tc.wantOwner)
			}
			if tc.wantOwner && (owner["NSELF_HOST_UID"] != strconv.Itoa(os.Getuid()) || owner["NSELF_HOST_GID"] != strconv.Itoa(os.Getgid())) {
				t.Fatalf("wrong host ids: %v", owner)
			}
		})
	}
}
