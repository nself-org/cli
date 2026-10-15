package docker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func logsComposeStub(t *testing.T, script string) (*Compose, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "docker")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	c := NewCompose("base.yml")
	c.EnvFiles = []string{"test.env"}
	c.DockerPath = bin
	return c, dir
}

func TestComposeLogAndPsOutput(t *testing.T) {
	c, dir := logsComposeStub(t, "printf '%s\\n' \"$PWD\" \"$*\"; printf 'diagnostic' >&2")
	var out, stderr bytes.Buffer
	if err := c.PsOutput(context.Background(), dir, &out, &stderr, "--services"); err != nil {
		t.Fatal(err)
	}
	want := dir + "\ncompose -f base.yml --env-file test.env ps --services\n"
	// macOS may resolve /var to /private/var in the subprocess cwd.
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	want = strings.Replace(want, dir, resolved, 1)
	if out.String() != want || stderr.String() != "diagnostic" {
		t.Fatalf("stdout=%q stderr=%q, want %q and diagnostic", out.String(), stderr.String(), want)
	}
	raw, err := c.LogsOutput(context.Background(), dir, "--tail", "7", "api")
	if err != nil || !strings.HasSuffix(string(raw), "logs --tail 7 api\n") {
		t.Fatalf("LogsOutput=%q, %v", raw, err)
	}
}

func TestComposeStreamLogs(t *testing.T) {
	c, dir := logsComposeStub(t, "printf '%s\\n' \"$*\"; cat; printf 'last'; printf 'diagnostic' >&2")
	var lines []string
	var stderr bytes.Buffer
	err := c.StreamLogs(context.Background(), dir, []string{"--follow", "api"}, strings.NewReader("first\n\n"), &stderr, func(line string) error {
		lines = append(lines, line)
		return nil
	})
	want := []string{"compose -f base.yml --env-file test.env logs --follow api", "first", "", "last"}
	if err != nil || !reflect.DeepEqual(lines, want) || stderr.String() != "diagnostic" {
		t.Fatalf("lines=%q stderr=%q err=%v", lines, stderr.String(), err)
	}
}

func TestComposeStreamLogsStops(t *testing.T) {
	for _, mode := range []string{"callback", "cancel", "oversize", "exit"} {
		t.Run(mode, func(t *testing.T) {
			script := "printf 'ready\\n'; while :; do printf 'line\\n'; done"
			if mode == "oversize" {
				script = "head -c 4194305 /dev/zero | tr '\\000' x"
			} else if mode == "exit" {
				script = "exit 23"
			}
			c, dir := logsComposeStub(t, script)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			stop := errors.New("writer failed")
			err := c.StreamLogs(ctx, dir, nil, nil, io.Discard, func(string) error {
				if mode == "cancel" {
					cancel()
					return nil
				}
				return stop
			})
			switch mode {
			case "callback":
				if !errors.Is(err, stop) {
					t.Fatalf("callback error lost: %v", err)
				}
			case "cancel":
				if err != nil {
					t.Fatalf("cancel should end stream cleanly: %v", err)
				}
			default:
				if err == nil {
					t.Fatal("expected subprocess/scanner error")
				}
			}
		})
	}
}
