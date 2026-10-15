package docker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// installCancellableDocker models a daemon container surviving its client.
func installCancellableDocker(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
run)
  printf '%s\n' "$@" > "$FAKE_DIR/run"
  touch "$FAKE_DIR/alive"
  if [ "$FAKE_COMPLETE" = 1 ]; then
    rm "$FAKE_DIR/alive"
    exit 0
  fi
  exec sleep 30
  ;;
rm)
  printf '%s\n' "$@" > "$FAKE_DIR/remove"
  if [ "$FAKE_REMOVE_FAIL" = 1 ]; then
    echo cleanup-denied >&2
    exit 1
  fi
  rm "$FAKE_DIR/alive"
  ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DIR", dir)
	return dir
}

func TestRunOneShotCancellationRemovesContainer(t *testing.T) {
	for _, tc := range []struct {
		title                  string
		name                   string
		deadline, cleanupFails bool
	}{
		{title: "generated name"},
		{title: "caller name", name: "caller-owned"},
		{title: "deadline", deadline: true},
		{title: "cleanup failure", cleanupFails: true},
	} {
		t.Run(tc.title, func(t *testing.T) {
			dir := installCancellableDocker(t)
			if tc.cleanupFails {
				t.Setenv("FAKE_REMOVE_FAIL", "1")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.deadline {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, time.Second)
				defer stop()
			}
			result := make(chan error, 1)
			go func() {
				_, _, err := RunOneShot(ctx, RunSpec{Image: "restore", Name: tc.name})
				result <- err
			}()
			ready := time.NewTimer(5 * time.Second)
			defer ready.Stop()
			tick := time.NewTicker(5 * time.Millisecond)
			defer tick.Stop()
			for {
				if _, err := os.Stat(filepath.Join(dir, "alive")); err == nil {
					break
				}
				select {
				case <-ready.C:
					t.Fatal("docker run did not start")
				case <-tick.C:
				}
			}
			if !tc.deadline {
				cancel()
			}
			select {
			case err := <-result:
				if !errors.Is(err, ctx.Err()) {
					t.Errorf("want context error %v, got %v", ctx.Err(), err)
				}
				if tc.cleanupFails && !strings.Contains(err.Error(), "cleanup-denied") {
					t.Errorf("cleanup failure was lost: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("RunOneShot did not finish cleanup")
			}
			run, err := os.ReadFile(filepath.Join(dir, "run"))
			if err != nil {
				t.Fatal(err)
			}
			args := strings.Fields(string(run))
			name := ""
			for i := 0; i+1 < len(args); i++ {
				if args[i] == "--name" {
					name = args[i+1]
				}
			}
			if name == "" || (tc.name != "" && name != tc.name) {
				t.Fatalf("missing or changed container name: %q", run)
			}
			removed, err := os.ReadFile(filepath.Join(dir, "remove"))
			if err != nil || string(removed) != "rm\n-f\n"+name+"\n" {
				t.Fatalf("cleanup did not target the run container: %q, %v", removed, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "alive")); !tc.cleanupFails && !os.IsNotExist(err) {
				t.Fatal("daemon container survived cancellation")
			}
		})
	}
}

func TestRunOneShotCompletionUsesUniqueNamesWithoutCleanup(t *testing.T) {
	dir := installCancellableDocker(t)
	t.Setenv("FAKE_COMPLETE", "1")
	seen := map[string]bool{}
	for range 2 {
		if _, _, err := RunOneShot(context.Background(), RunSpec{Image: "restore"}); err != nil {
			t.Fatal(err)
		}
		run, err := os.ReadFile(filepath.Join(dir, "run"))
		if err != nil {
			t.Fatal(err)
		}
		args := strings.Fields(string(run))
		if len(args) != 5 || args[2] != "--name" || !strings.HasPrefix(args[3], "nself-oneshot-") || seen[args[3]] {
			t.Fatalf("want a unique container name, got %q", run)
		}
		seen[args[3]] = true
	}
	if _, err := os.Stat(filepath.Join(dir, "remove")); !os.IsNotExist(err) {
		t.Fatal("successful run triggered cancellation cleanup")
	}
}

func TestRunOneShotAlreadyCancelledDoesNotRemoveCallerContainer(t *testing.T) {
	dir := installCancellableDocker(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := RunOneShot(ctx, RunSpec{Image: "restore", Name: "existing"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want cancellation, got %v", err)
	}
	for _, file := range []string{"run", "remove"} {
		if _, err := os.Stat(filepath.Join(dir, file)); !os.IsNotExist(err) {
			t.Fatalf("already cancelled context invoked docker %s", file)
		}
	}
}
