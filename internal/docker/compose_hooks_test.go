package docker

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestPreDownHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir := t.TempDir()
	order := filepath.Join(dir, "order.log")
	bin := filepath.Join(dir, "docker")
	script := "#!/bin/sh\nprintf 'docker %s\\n' \"$*\" >> '" + order + "'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Compose{DockerPath: bin}

	var got []string
	unregister := RegisterPreDownHook(func(_ context.Context, workdir string) func() {
		got = append(got, workdir)
		b, _ := os.ReadFile(order)
		if len(b) != 0 {
			t.Errorf("the hook ran after docker: %q", b)
		}
		return nil
	})
	if err := c.ComposeDown(context.Background(), dir, DownOptions{RemoveOrphans: true}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{dir}) {
		t.Errorf("hook calls = %v, want one call for %s", got, dir)
	}
	b, _ := os.ReadFile(order)
	if !strings.HasPrefix(string(b), "docker compose down") {
		t.Errorf("docker was not run after the hook: %q", b)
	}

	// An unregistered hook is not called again.
	unregister()
	if err := c.ComposeDown(context.Background(), dir, DownOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("an unregistered hook ran again: %v", got)
	}
}

func TestComposeDownRemoveImages(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir := t.TempDir()
	logf := filepath.Join(dir, "calls.log")
	bin := filepath.Join(dir, "docker")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> '"+logf+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Compose{DockerPath: bin}
	if err := c.ComposeDown(context.Background(), dir, DownOptions{RemoveVolumes: true, RemoveImages: true, RemoveOrphans: true, Timeout: 5}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(logf)
	if got, want := strings.TrimSpace(string(b)), "compose down -v --rmi all --remove-orphans -t 5"; got != want {
		t.Errorf("args = %q, want %q", got, want)
	}
}

// D-0929: a hook's release func runs after docker down (even when it fails),
// newest hook first, so a lock taken by the hook spans the down.
func TestPreDownHookReleaseRunsAfterDown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	for _, fail := range []bool{false, true} {
		dir := t.TempDir()
		order := filepath.Join(dir, "order.log")
		bin := filepath.Join(dir, "docker")
		script := "#!/bin/sh\nprintf 'docker %s\\n' \"$*\" >> '" + order + "'\n"
		if fail {
			script += "exit 1\n"
		}
		if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		c := &Compose{DockerPath: bin}
		var events []string
		mark := func(name string) func() {
			return func() {
				b, _ := os.ReadFile(order)
				events = append(events, name+" docker="+strings.TrimSpace(string(b)))
			}
		}
		u1 := RegisterPreDownHook(func(context.Context, string) func() { return mark("first") })
		u2 := RegisterPreDownHook(func(context.Context, string) func() { return nil })
		u3 := RegisterPreDownHook(func(context.Context, string) func() { return mark("third") })
		err := c.ComposeDown(context.Background(), dir, DownOptions{})
		u1()
		u2()
		u3()
		if (err != nil) != fail {
			t.Fatalf("fail=%v: ComposeDown error = %v", fail, err)
		}
		want := []string{"third docker=docker compose down", "first docker=docker compose down"}
		if !slices.Equal(events, want) {
			t.Errorf("fail=%v: releases = %q, want %q (after docker, newest first)", fail, events, want)
		}
	}
}
