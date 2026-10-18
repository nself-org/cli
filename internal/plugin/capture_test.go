package plugin

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// stub writes an executable shell script and returns its path.
func stub(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stubs need a POSIX shell")
	}
	p := filepath.Join(t.TempDir(), "stub")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCaptureOutput(t *testing.T) {
	t.Run("stdout and zero exit", func(t *testing.T) {
		out, exit, err := CaptureOutput(context.Background(), stub(t, `echo "args: $*"`), []string{"doctor", "--json"})
		if err != nil || exit != 0 || string(out) != "args: doctor --json\n" {
			t.Fatalf("out=%q exit=%d err=%v", out, exit, err)
		}
	})
	t.Run("non-zero exit is not an error", func(t *testing.T) {
		out, exit, err := CaptureOutput(context.Background(), stub(t, `echo partial; exit 12`), nil)
		if err != nil || exit != 12 || string(out) != "partial\n" {
			t.Fatalf("out=%q exit=%d err=%v", out, exit, err)
		}
	})
	t.Run("stderr is not captured", func(t *testing.T) {
		out, _, err := CaptureOutput(context.Background(), stub(t, `echo err >&2; echo out`), nil)
		if err != nil || string(out) != "out\n" {
			t.Fatalf("out=%q err=%v", out, err)
		}
	})
	t.Run("environment comes from ExecEnv", func(t *testing.T) {
		out, _, err := CaptureOutput(context.Background(), stub(t, `echo "$NSELF_CLI_VERSION|$NSELF_V15"`), nil)
		if err != nil {
			t.Fatal(err)
		}
		ver, mode, ok := strings.Cut(strings.TrimSpace(string(out)), "|")
		if !ok || ver == "" || (mode != "0" && mode != "1") {
			t.Fatalf("env line %q", out)
		}
	})
	t.Run("deadline kills the process and returns promptly", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, exit, err := CaptureOutput(ctx, stub(t, `exec sleep 30`), nil)
		if err == nil || exit != -1 || !strings.Contains(err.Error(), "deadline") {
			t.Fatalf("exit=%d err=%v", exit, err)
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Fatalf("took %s", d)
		}
	})
	t.Run("a shell whose child holds the pipe still returns", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, _, err := CaptureOutput(ctx, stub(t, `sleep 30`), nil)
		if err == nil {
			t.Fatal("want a deadline error")
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Fatalf("took %s", d)
		}
	})
	t.Run("missing binary is an error", func(t *testing.T) {
		_, exit, err := CaptureOutput(context.Background(), filepath.Join(t.TempDir(), "nope"), nil)
		if err == nil || exit != -1 {
			t.Fatalf("exit=%d err=%v", exit, err)
		}
	})
	t.Run("output over the cap is an error", func(t *testing.T) {
		out, exit, err := CaptureOutput(context.Background(), stub(t, `yes | head -c 5000000`), nil)
		if err == nil || exit != -1 || len(out) != maxCaptureBytes {
			t.Fatalf("len=%d exit=%d err=%v", len(out), exit, err)
		}
	})
}
