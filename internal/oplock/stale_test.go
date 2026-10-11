//go:build !windows

package oplock

// Tests for D-0241: a waiter that reads the previous, dead holder's record
// treats it as unknown and re-reads; the platform notice respects --quiet and
// a non-terminal stderr. Temp dirs and an injected clock only.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

const deadPID = 999_999

// heldWithRecord holds the lock of a temp project and replaces its record
// with h, as if the previous holder's record were still in the file.
func heldWithRecord(t *testing.T, h Holder) string {
	t.Helper()
	dir := t.TempDir()
	l, err := Acquire(context.Background(), dir, Opts{Command: "nself build"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	writeRecord(t, dir, h)
	return dir
}

func writeRecord(t *testing.T, dir string, h Holder) {
	t.Helper()
	raw, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LockPath(dir), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// fakeAlive makes deadPID the only dead process.
func fakeAlive(t *testing.T) {
	t.Helper()
	old := processAlive
	processAlive = func(pid int) bool { return pid != deadPID }
	t.Cleanup(func() { processAlive = old })
}

func tokenEnv(tok string) func(string) string {
	return func(k string) string {
		if k == EnvToken {
			return tok
		}
		return ""
	}
}

func TestOpLockStaleHolderIsUnknown(t *testing.T) {
	fakeAlive(t)
	host, _ := os.Hostname()
	stale := strings.Repeat("a", 32)
	cases := []struct {
		name      string
		holder    Holder
		wantKnown bool
		wantReent bool
	}{
		// The dead previous holder on this host: unknown, and its token (held
		// by a surviving child) grants nothing.
		{name: "dead pid same host", holder: Holder{PID: deadPID, Command: "old", Token: stale, Host: host}},
		// Control: a live pid with the same token re-enters, so the refusal
		// above comes from the stale check alone.
		{name: "live pid same host", holder: Holder{PID: os.Getpid(), Command: "old", Token: stale, Host: host}, wantKnown: true, wantReent: true},
		// Another host cannot be checked: the record is taken as is.
		{name: "dead pid other host", holder: Holder{PID: deadPID, Command: "old", Token: stale, Host: host + "-elsewhere"}, wantKnown: true, wantReent: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := heldWithRecord(t, tc.holder)
			fc := newFake()
			l, err := Acquire(context.Background(), dir, Opts{Clock: fc.clock(), Getenv: tokenEnv(stale)})
			if tc.wantReent {
				if err != nil || !l.Reentrant() {
					t.Fatalf("want re-entry, got %v", err)
				}
				return
			}
			var he *HeldError
			if !errors.As(err, &he) {
				t.Fatalf("want *HeldError, got %v", err)
			}
			if he.Known != tc.wantKnown {
				t.Fatalf("Known = %v, want %v (holder %+v)", he.Known, tc.wantKnown, he.Holder)
			}
			if strings.Contains(he.Error(), "999999") {
				t.Errorf("error names the dead pid: %q", he.Error())
			}
		})
	}
}

func TestOpLockWaiterRereadsUntilNewRecord(t *testing.T) {
	fakeAlive(t)
	host, _ := os.Hostname()
	dir := heldWithRecord(t, Holder{PID: deadPID, Command: "old", Token: strings.Repeat("a", 32), Host: host})
	fresh := Holder{PID: os.Getpid(), Command: "nself deploy", Token: strings.Repeat("b", 32), Host: host}
	fc := newFake()
	fc.onSleep = func(n int) {
		if n == 2 { // the new holder writes its record while the waiter retries
			writeRecord(t, dir, fresh)
		}
	}
	_, err := Acquire(context.Background(), dir, Opts{Clock: fc.clock(), Getenv: tokenEnv("")})
	var he *HeldError
	if !errors.As(err, &he) || !he.Known || he.Holder.Command != "nself deploy" {
		t.Fatalf("want the new holder named, got %v", err)
	}
}

func TestOpLockPidAlive(t *testing.T) {
	if !pidAlive(os.Getpid()) {
		t.Error("own pid must be alive")
	}
	if pidAlive(0) || pidAlive(-1) {
		t.Error("pid <= 0 is never alive")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if pidAlive(cmd.Process.Pid) {
		// Either pidAlive is wrong or the kernel already reused the pid; tell
		// them apart by a second, unrelated reaped child.
		again := exec.Command(os.Args[0], "-test.run=^$")
		if err := again.Run(); err != nil {
			t.Fatal(err)
		}
		if pidAlive(again.Process.Pid) {
			t.Errorf("reaped children %d and %d reported alive", cmd.Process.Pid, again.Process.Pid)
		}
	}
}

func TestOpLockUnsupportedNotice(t *testing.T) {
	cases := []struct {
		name     string
		quiet    bool
		terminal bool
		want     bool
	}{
		{name: "terminal", terminal: true, want: true},
		{name: "quiet", quiet: true, terminal: true},
		{name: "not a terminal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldErr, oldTTY := Stderr, stderrIsTerminal
			buf := &bytes.Buffer{}
			Stderr, stderrIsTerminal = buf, func() bool { return tc.terminal }
			unsupportedOnce = sync.Once{}
			t.Cleanup(func() { Stderr, stderrIsTerminal, unsupportedOnce = oldErr, oldTTY, sync.Once{} })
			noteUnsupported(tc.quiet)
			noteUnsupported(tc.quiet)
			n := strings.Count(buf.String(), "not supported on this platform")
			if tc.want && n != 1 || !tc.want && n != 0 {
				t.Fatalf("notice printed %d times: %q", n, buf.String())
			}
		})
	}
}
