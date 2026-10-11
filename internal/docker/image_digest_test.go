package docker

// Tests for ImageRepoDigests and LocalRepoDigest. internal/docker has no exec
// seam, so a fake `docker` shell script goes first on PATH and logs its calls.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const dgA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const dgB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// fakeImageDocker installs a docker that answers `image inspect` with out
// (exit 0), or with the stderr line msg (exit 1) when msg is set. It returns
// the call log path.
func fakeImageDocker(t *testing.T, out, msg string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" >> \"" + filepath.Join(dir, "calls.log") + "\"\n"
	if msg != "" {
		script += "echo '" + msg + "' >&2\nexit 1\n"
	} else {
		script += "echo '" + out + "'\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return filepath.Join(dir, "calls.log")
}

func TestImageRepoDigests(t *testing.T) {
	t.Run("lists the digests through image inspect", func(t *testing.T) {
		log := fakeImageDocker(t, `["redis@`+dgA+`"]`, "")
		got, err := ImageRepoDigests(context.Background(), "redis:7-alpine")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"redis@" + dgA}; !reflect.DeepEqual(got, want) {
			t.Fatalf("digests = %v, want %v", got, want)
		}
		calls, _ := os.ReadFile(log)
		if !strings.HasPrefix(string(calls), "image inspect ") || !strings.Contains(string(calls), "redis:7-alpine") {
			t.Fatalf("docker was called with %q", calls)
		}
	})
	t.Run("an image with no digest is empty, not an error", func(t *testing.T) {
		fakeImageDocker(t, "null", "")
		got, err := ImageRepoDigests(context.Background(), "local:dev")
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, %v; want empty and nil", got, err)
		}
	})
	t.Run("absent image is ErrImageNotFound", func(t *testing.T) {
		fakeImageDocker(t, "", "Error: No such image: ghost:1")
		_, err := ImageRepoDigests(context.Background(), "ghost:1")
		if !errors.Is(err, ErrImageNotFound) {
			t.Fatalf("err = %v, want ErrImageNotFound", err)
		}
	})
	t.Run("a daemon failure is not reported as absent", func(t *testing.T) {
		fakeImageDocker(t, "", "Cannot connect to the Docker daemon")
		_, err := ImageRepoDigests(context.Background(), "redis:7")
		if err == nil || errors.Is(err, ErrImageNotFound) {
			t.Fatalf("err = %v, want a non-ErrImageNotFound error", err)
		}
	})
	t.Run("empty reference", func(t *testing.T) {
		if _, err := ImageRepoDigests(context.Background(), " "); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestLocalRepoDigest(t *testing.T) {
	t.Run("pins the digest of the reference's repository", func(t *testing.T) {
		fakeImageDocker(t, `["redis@`+dgA+`"]`, "")
		if got := LocalRepoDigest(context.Background(), "redis:7-alpine"); got != dgA {
			t.Fatalf("digest = %q, want %q", got, dgA)
		}
	})
	t.Run("no digest or no image is empty so the caller falls back", func(t *testing.T) {
		fakeImageDocker(t, "null", "")
		if got := LocalRepoDigest(context.Background(), "redis:7"); got != "" {
			t.Fatalf("digest = %q, want empty", got)
		}
		fakeImageDocker(t, "", "Error: No such image: redis:7")
		if got := LocalRepoDigest(context.Background(), "redis:7"); got != "" {
			t.Fatalf("digest = %q, want empty", got)
		}
	})
}

func TestPickRepoDigest(t *testing.T) {
	cases := []struct {
		name, ref string
		entries   []string
		want      string
	}{
		{"hub library image", "redis:7-alpine", []string{"redis@" + dgA}, dgA},
		{"docker.io prefix on either side", "docker.io/library/redis:7", []string{"redis@" + dgA}, dgA},
		{"registry with port", "localhost:5000/app:1", []string{"localhost:5000/app@" + dgA}, dgA},
		{"another repository never counts", "redis:7", []string{"other/redis@" + dgA}, ""},
		{"two different digests for the repo", "redis:7", []string{"redis@" + dgA, "redis@" + dgB}, ""},
		{"same digest twice", "redis:7", []string{"redis@" + dgA, "docker.io/library/redis@" + dgA}, dgA},
		{"malformed digest", "redis:7", []string{"redis@sha256:abc"}, ""},
		{"no entries", "redis:7", nil, ""},
	}
	for _, c := range cases {
		if got := pickRepoDigest(c.ref, c.entries); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
