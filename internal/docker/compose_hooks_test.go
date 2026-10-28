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
	released := 0
	unregister := RegisterPreDownHook(func(_ context.Context, workdir string) func() {
		got = append(got, workdir)
		b, _ := os.ReadFile(order)
		if len(b) != 0 {
			t.Errorf("the hook ran after docker: %q", b)
		}
		return func() {
			released++
			if b, _ := os.ReadFile(order); !strings.HasPrefix(string(b), "docker compose down") {
				t.Errorf("the hook was released before docker finished: %q", b)
			}
		}
	})
	if err := c.ComposeDown(context.Background(), dir, DownOptions{RemoveOrphans: true}); err != nil {
		t.Fatal(err)
	}
	if released != 1 {
		t.Errorf("release ran %d times after the down, want 1 (D-0929)", released)
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
