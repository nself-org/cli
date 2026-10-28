//go:build !windows

package docker

// stream_test.go — Stream and StreamLines against a fake docker script (D-0711):
// output is forwarded without a TTY, a stop leaves no orphan, SIGINT is caught.

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeDockerOnPath puts a docker stub with body first on PATH.
func fakeDockerOnPath(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\n"+body), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestStreamForwardsBothStreamsWithoutTTY(t *testing.T) {
	fakeDockerOnPath(t, "echo \"args:$*\"\necho oops >&2\nexit 0\n")
	var out, errb bytes.Buffer
	if err := Stream(context.Background(), t.TempDir(), []string{"compose", "logs", "-f"}, &out, &errb); err != nil {
		t.Fatal(err)
	}
	if out.String() != "args:compose logs -f\n" || errb.String() != "oops\n" {
		t.Errorf("stdout %q, stderr %q", out.String(), errb.String())
	}
}

func TestStreamReturnsNonZeroExit(t *testing.T) {
	fakeDockerOnPath(t, "exit 3\n")
	err := Stream(context.Background(), t.TempDir(), []string{"logs"}, &bytes.Buffer{}, &bytes.Buffer{})
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 3 {
		t.Errorf("err = %v, want an ExitError with code 3", err)
	}
}

// waitFor polls until path exists.
func waitFor(t *testing.T, path string) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", path)
}

// alive reports whether pid still runs. A zombie (killed but not yet reaped)
// does not count: when the test binary runs under a parent that is the new
// parent of orphans and never reaps them (the nself-ci runner), kill(pid, 0)
// succeeds for a dead child forever.
func alive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // fixed argv, pid is an int
	if err != nil {
		return false
	}
	return !strings.HasPrefix(strings.TrimSpace(string(out)), "Z")
}

// longRunning is a docker stub that records a grandchild's pid in marks/gc and
// then waits on it, like docker spawning the compose plugin.
func longRunning(t *testing.T) (marks string) {
	t.Helper()
	marks = t.TempDir()
	fakeDockerOnPath(t, "sleep 30 &\necho $! > '"+marks+"/gc.tmp'\nmv '"+marks+"/gc.tmp' '"+marks+"/gc'\nwait\n")
	return marks
}

func grandchild(t *testing.T, marks string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(marks, "gc"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func awaitGone(t *testing.T, pid int) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if !alive(pid) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Errorf("process %d was orphaned", pid)
}

func TestStreamContextStopLeavesNoOrphan(t *testing.T) {
	marks := longRunning(t)
	ctx, cancel := context.WithCancel(context.Background())
	res := make(chan error, 1)
	go func() { res <- Stream(ctx, t.TempDir(), []string{"logs"}, &bytes.Buffer{}, &bytes.Buffer{}) }()
	waitFor(t, filepath.Join(marks, "gc"))
	cancel()
	if err := <-res; err != nil {
		t.Errorf("a stop is not an error: %v", err)
	}
	awaitGone(t, grandchild(t, marks))
}

// TestStreamSIGINTStopsTheChildNotTheProcess: Ctrl-C reaches the stream through
// its NotifyContext; the test process survives and the child group is gone.
func TestStreamSIGINTStopsTheChildNotTheProcess(t *testing.T) {
	// On Linux CI (non-TTY, no controlling terminal), sending SIGINT to
	// os.Getpid() from the test goroutine races with signal.NotifyContext
	// registration inside Stream's goroutine: if the signal arrives first,
	// the default action kills the test binary. Instead, have the fake docker
	// send SIGINT to its parent after it starts, guaranteeing the handler
	// is installed before the signal lands.
	marks := t.TempDir()
	fakeDockerOnPath(t, "sleep 30 &\necho $! > '"+marks+"/gc.tmp'\nmv '"+marks+"/gc.tmp' '"+marks+"/gc'\nkill -INT $PPID\nwait\n")
	res := make(chan error, 1)
	go func() {
		res <- Stream(context.Background(), t.TempDir(), []string{"logs"}, &bytes.Buffer{}, &bytes.Buffer{})
	}()
	waitFor(t, filepath.Join(marks, "gc"))
	select {
	case err := <-res:
		if err != nil {
			t.Errorf("a stop is not an error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("SIGINT did not stop the stream")
	}
	awaitGone(t, grandchild(t, marks))
}

func TestStreamLines(t *testing.T) {
	fakeDockerOnPath(t, "echo one\necho err1 >&2\necho two\n")
	collect := func(merge bool) (lines []string, errb string) {
		var eb bytes.Buffer
		err := StreamLines(context.Background(), t.TempDir(), []string{"logs"}, &eb, merge, 1<<20, func(l string) error {
			lines = append(lines, l)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return lines, eb.String()
	}
	if lines, eb := collect(false); strings.Join(lines, ",") != "one,two" || eb != "err1\n" {
		t.Errorf("unmerged: lines %q, stderr %q", lines, eb)
	}
	if lines, eb := collect(true); len(lines) != 3 || eb != "" {
		t.Errorf("merged: lines %q, stderr %q", lines, eb)
	}
}

func TestStreamLinesCallbackErrorStopsDocker(t *testing.T) {
	marks := t.TempDir()
	fakeDockerOnPath(t, "sleep 30 &\necho $! > '"+marks+"/gc'\necho first\nwait\n")
	boom := errors.New("boom")
	err := StreamLines(context.Background(), t.TempDir(), []string{"logs"}, nil, false, 1<<20, func(string) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	awaitGone(t, grandchild(t, marks))
}

func TestStreamLinesOverlongLineIsAnError(t *testing.T) {
	fakeDockerOnPath(t, "head -c 5000 /dev/zero | tr '\\0' x\necho\n")
	err := StreamLines(context.Background(), t.TempDir(), []string{"logs"}, nil, false, 1024, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "reading docker output") {
		t.Errorf("err = %v, want a read error", err)
	}
}
