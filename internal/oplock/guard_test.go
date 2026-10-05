//go:build !windows

package oplock

// Tests for the guard policy: which classes take the lock, the v1.5 and v1.4
// contention paths, token export and restore, and fail-open on I/O errors.
// Time is injected (fakeClock, oplock_test.go); nothing sleeps.

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

func useGuardEnv(t *testing.T, fc *fakeClock) *bytes.Buffer {
	t.Helper()
	oldClock, oldErr := DefaultClock, Stderr
	buf := &bytes.Buffer{}
	DefaultClock, Stderr = fc.clock(), buf
	t.Cleanup(func() { DefaultClock, Stderr = oldClock, oldErr })
	t.Setenv(EnvToken, "")
	_ = os.Unsetenv(EnvToken)
	return buf
}

func TestOpLockTakes(t *testing.T) {
	cases := []struct {
		side, out string
		want      bool
	}{
		{"read", "document", false},
		{"read", "stream", false},
		{"write", "document", true},
		{"remote", "document", true},
		{"destructive", "document", true},
		{"write", "stream", false},
		{"destructive", "interactive", false},
		{"", "document", false},
		{"bogus", "document", false},
	}
	for _, c := range cases {
		if got := Takes(c.side, c.out); got != c.want {
			t.Errorf("Takes(%q,%q)=%v want %v", c.side, c.out, got, c.want)
		}
	}
}

func TestOpLockGuardSkipsOtherClasses(t *testing.T) {
	dir := t.TempDir()
	useGuardEnv(t, newFake())
	ctx, release, err := Guard(context.Background(), Request{Dir: dir, Command: "nself status", SideEffect: "read", Output: "document"})
	if err != nil || FromContext(ctx) != nil {
		t.Fatalf("read command: err=%v lock=%v", err, FromContext(ctx))
	}
	release()
	if _, err := os.Stat(LockPath(dir)); err == nil {
		t.Fatal("a read command created the lock file")
	}
	if FromContext(nil) != nil { //nolint:staticcheck // nil context is part of the contract
		t.Fatal("nil context must yield no lock")
	}
}

func TestOpLockGuardV15Held(t *testing.T) {
	compattest.Set(t, true)
	dir := t.TempDir()
	useGuardEnv(t, newFake())
	held, err := Acquire(context.Background(), dir, Opts{Command: "nself build"})
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	_, _, err = Guard(context.Background(), Request{Dir: dir, Command: "nself config set", SideEffect: "write", Output: "document"})
	d := errs.Describe(err)
	if d == nil || d.Code != "E460" || errs.ExitCodeFor(err) != 1 || !errors.Is(err, ErrHeld) {
		t.Fatalf("want E460 exit 1 wrapping ErrHeld, got %v", err)
	}
	if !strings.Contains(err.Error(), "nself build (pid ") || !strings.Contains(d.Remediation, "wait for nself build (pid ") {
		t.Fatalf("holder not named: %v / %q", err, d.Remediation)
	}
	// A holder with no record yet: generic text, registry default fix.
	g := heldCLIError(&HeldError{})
	if !strings.Contains(g.Error(), "another nself operation") || strings.Contains(g.Error(), "pid") {
		t.Fatalf("unknown holder text: %v", g)
	}
	if (&HeldError{}).Error() == "" || (&HeldError{Known: true, Holder: Holder{PID: 7, Command: "c"}}).Error() != "c (pid 7) holds the project lock" {
		t.Fatal("HeldError text")
	}
}

func TestOpLockGuardV14WaitsThenProceeds(t *testing.T) {
	compattest.Set(t, false)
	dir := t.TempDir()
	fc := newFake()
	stderr := useGuardEnv(t, fc)
	held, err := Acquire(context.Background(), dir, Opts{Command: "nself build"})
	if err != nil {
		t.Fatal(err)
	}
	fc.onSleep = func(n int) {
		if n == 2 {
			held.Release()
		}
	}
	req := Request{Dir: dir, Command: "nself config set", SideEffect: "write", Output: "document"}
	ctx, release, err := Guard(context.Background(), req)
	if err != nil || FromContext(ctx) == nil || fc.sleeps != 2 {
		t.Fatalf("v1.4 wait: err=%v lock=%v sleeps=%d", err, FromContext(ctx), fc.sleeps)
	}
	if !strings.Contains(stderr.String(), "waiting up to 30s for nself build") {
		t.Fatalf("no wait notice: %q", stderr.String())
	}
	// The token is exported while held and restored afterwards.
	if os.Getenv(EnvToken) != FromContext(ctx).Token() || len(os.Getenv(EnvToken)) != 32 {
		t.Fatalf("token not exported: %q", os.Getenv(EnvToken))
	}
	// A child with that token re-enters; its guard exports nothing new.
	ctx2, release2, err := Guard(context.Background(), req)
	if err != nil || !FromContext(ctx2).Reentrant() {
		t.Fatalf("re-entry: err=%v", err)
	}
	release2()
	if os.Getenv(EnvToken) == "" {
		t.Fatal("a re-entrant release must not clear the parent's token")
	}
	release()
	release() // idempotent
	if v, ok := os.LookupEnv(EnvToken); ok {
		t.Fatalf("token leaked after release: %q", v)
	}
}

func TestOpLockGuardV14TimeoutWarns(t *testing.T) {
	compattest.Set(t, false)
	dir := t.TempDir()
	fc := newFake()
	stderr := useGuardEnv(t, fc)
	held, err := Acquire(context.Background(), dir, Opts{Command: "nself build"})
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	start := fc.now
	ctx, release, err := Guard(context.Background(), Request{Dir: dir, Command: "x", SideEffect: "destructive", Output: "document"})
	release()
	if err != nil || FromContext(ctx) != nil {
		t.Fatalf("v1.4 timeout proceeds unlocked: err=%v", err)
	}
	if fc.now.Sub(start) < 30*time.Second || !strings.Contains(stderr.String(), "continuing without the lock") {
		t.Fatalf("waited %v, stderr %q", fc.now.Sub(start), stderr.String())
	}
}

func TestOpLockGuardFailsOpenOnIOError(t *testing.T) {
	compattest.Set(t, true)
	// The project path is a file, so .nself cannot be created.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	stderr := useGuardEnv(t, newFake())
	ctx, release, err := Guard(context.Background(), Request{Dir: blocker, Command: "x", SideEffect: "write", Output: "document"})
	release()
	if err != nil || FromContext(ctx) != nil || !strings.Contains(stderr.String(), "unavailable") {
		t.Fatalf("I/O failure must not block the command: err=%v stderr=%q", err, stderr.String())
	}
}

func TestOpLockWaitHonoursContext(t *testing.T) {
	dir := t.TempDir()
	held, err := Acquire(context.Background(), dir, Opts{Command: "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	ctx, cancel := context.WithCancel(context.Background())
	fc := newFake()
	fc.onSleep = func(int) { cancel() }
	c := fc.clock()
	c.Sleep = func(ctx context.Context, d time.Duration) error { cancel(); return ctx.Err() }
	_, err = Acquire(ctx, dir, Opts{Command: "b", Wait: time.Minute, Clock: c})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if _, err := ReadHolder(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing holder file must be an error")
	}
}
