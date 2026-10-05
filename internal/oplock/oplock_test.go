//go:build !windows

package oplock

// Tests for the project operation lock: contention, SIGKILL release, token
// re-entrancy. Two-process cases re-run the test binary as a helper that
// acquires the lock and blocks until its stdin closes. No test sleeps: waiting
// is driven by an injected clock and by pipe reads.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeClock advances only when Sleep is called.
type fakeClock struct {
	now     time.Time
	sleeps  int
	onSleep func(n int)
}

func (c *fakeClock) clock() Clock {
	return Clock{
		Now: func() time.Time { return c.now },
		Sleep: func(_ context.Context, d time.Duration) error {
			c.sleeps++
			c.now = c.now.Add(d)
			if c.onSleep != nil {
				c.onSleep(c.sleeps)
			}
			return nil
		},
	}
}

func newFake() *fakeClock { return &fakeClock{now: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)} }

// TestOpLockHelperProcess is the helper body: it holds the lock of
// $OPLOCK_DIR as "nself build" until stdin closes. It does nothing in a
// normal test run.
func TestOpLockHelperProcess(t *testing.T) {
	dir := os.Getenv("OPLOCK_DIR")
	if os.Getenv("OPLOCK_HELPER") != "hold" || dir == "" {
		t.Skip("helper process only")
	}
	l, err := Acquire(context.Background(), dir, Opts{Command: "nself build"})
	if err != nil {
		fmt.Println("error", err)
		os.Exit(3)
	}
	fmt.Println("ready", l.Token())
	_, _ = io.Copy(io.Discard, os.Stdin)
	l.Release()
	os.Exit(0)
}

// startHolder runs the helper and returns it once it holds the lock.
func startHolder(t *testing.T, dir string) (cmd *exec.Cmd, stdin io.WriteCloser, token string) {
	t.Helper()
	cmd = exec.Command(os.Args[0], "-test.run=^TestOpLockHelperProcess$")
	cmd.Env = append(os.Environ(), "OPLOCK_HELPER=hold", "OPLOCK_DIR="+dir, EnvToken+"=")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "ready ") {
		t.Fatalf("helper did not acquire: %q %v", line, err)
	}
	return cmd, stdin, strings.TrimSpace(strings.TrimPrefix(line, "ready "))
}

func TestOpLockContention(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// Same process, two descriptors: the second is refused and names the first.
	a, err := Acquire(ctx, dir, Opts{Command: "nself build"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Acquire(ctx, dir, Opts{Command: "nself config set"})
	var he *HeldError
	if !errors.As(err, &he) || !errors.Is(err, ErrHeld) {
		t.Fatalf("want HeldError, got %v", err)
	}
	if !he.Known || he.Holder.PID != os.Getpid() || he.Holder.Command != "nself build" {
		t.Fatalf("holder not named: %+v", he)
	}
	if len(he.Holder.Token) != 32 || he.Holder.StartedAt == "" || he.Holder.Host == "" {
		t.Fatalf("holder record incomplete: %+v", he.Holder)
	}
	a.Release()
	a.Release() // idempotent
	b, err := Acquire(ctx, dir, Opts{Command: "nself config set"})
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	b.Release()

	// Another process holds it: fail fast names the helper's pid and command.
	helper, stdin, _ := startHolder(t, dir)
	_, err = Acquire(ctx, dir, Opts{Command: "nself build"})
	if !errors.As(err, &he) || he.Holder.PID != helper.Process.Pid || he.Holder.Command != "nself build" {
		t.Fatalf("second process: want holder pid %d, got %v (%+v)", helper.Process.Pid, err, he)
	}

	// Waiting with an injected clock: a poll budget that runs out is a HeldError.
	fc := newFake()
	_, err = Acquire(ctx, dir, Opts{Command: "x", Wait: 30 * time.Second, Poll: time.Second, Clock: fc.clock()})
	if !errors.As(err, &he) || he.Waited < 30*time.Second || fc.sleeps != 30 {
		t.Fatalf("wait budget: err=%v waited=%v sleeps=%d", err, he, fc.sleeps)
	}

	// The holder finishing mid-wait lets the waiter in.
	fc = newFake()
	fc.onSleep = func(n int) {
		if n == 3 {
			_ = stdin.Close()
			_ = helper.Wait()
		}
	}
	l, err := Acquire(ctx, dir, Opts{Command: "x", Wait: 30 * time.Second, Poll: time.Second, Clock: fc.clock()})
	if err != nil {
		t.Fatalf("waiter after holder exit: %v", err)
	}
	defer l.Release()
	if fc.sleeps != 3 {
		t.Fatalf("waited %d polls, want 3", fc.sleeps)
	}
}

func TestOpLockSIGKILL(t *testing.T) {
	dir := t.TempDir()
	helper, _, _ := startHolder(t, dir)
	if _, err := Acquire(context.Background(), dir, Opts{Command: "x"}); !errors.Is(err, ErrHeld) {
		t.Fatalf("holder alive: want ErrHeld, got %v", err)
	}
	if err := helper.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = helper.Wait()
	// The record of the dead holder is still in the file; only the flock counts.
	if h, err := ReadHolder(dir); err != nil || h.PID != helper.Process.Pid {
		t.Fatalf("stale record expected, got %+v %v", h, err)
	}
	l, err := Acquire(context.Background(), dir, Opts{Command: "nself build"})
	if err != nil {
		t.Fatalf("next acquire after SIGKILL must succeed without cleanup: %v", err)
	}
	defer l.Release()
	if h, _ := ReadHolder(dir); h.PID != os.Getpid() {
		t.Fatalf("record not replaced: %+v", h)
	}
}

func TestOpLockTokenReentry(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	parent, err := Acquire(ctx, dir, Opts{Command: "nself build"})
	if err != nil {
		t.Fatal(err)
	}
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == EnvToken {
				return v
			}
			return ""
		}
	}
	child, err := Acquire(ctx, dir, Opts{Command: "nself build", Getenv: env(parent.Token())})
	if err != nil || !child.Reentrant() {
		t.Fatalf("child with the holder's token must re-enter: %v", err)
	}
	child.Release() // owns nothing
	if _, err := Acquire(ctx, dir, Opts{Getenv: env("")}); !errors.Is(err, ErrHeld) {
		t.Fatalf("child without the token must be refused, got %v", err)
	}
	if _, err := Acquire(ctx, dir, Opts{Getenv: env(strings.Repeat("0", 32))}); !errors.Is(err, ErrHeld) {
		t.Fatalf("child with a wrong token must be refused, got %v", err)
	}
	parent.Release()
	// A token whose lock is no longer held is stale and grants nothing.
	l, err := Acquire(ctx, dir, Opts{Command: "next", Getenv: env(parent.Token())})
	if err != nil || l.Reentrant() {
		t.Fatalf("stale token must acquire for real: %v", err)
	}
	l.Release()
}

func TestOpLockRecreatedFile(t *testing.T) {
	// Documented limit (Safe-On-Live.md): a clean that deletes .nself while the
	// lock is held orphans that lock; the next process locks the new file. The
	// check after locking only guarantees a process never keeps a lock on a
	// file that is no longer at the path when it acquires.
	dir := t.TempDir()
	a, err := Acquire(context.Background(), dir, Opts{Command: "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	if err := os.Remove(filepath.Join(dir, ".nself", "op.lock")); err != nil {
		t.Fatal(err)
	}
	b, err := Acquire(context.Background(), dir, Opts{Command: "b"})
	if err != nil {
		t.Fatalf("a fresh file is a fresh lock: %v", err)
	}
	b.Release()
}
