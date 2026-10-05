// Package oplock is the project operation lock (EPIC P7-LIVE D12, A-21).
//
// Purpose: one write, remote or destructive nself command at a time per
// project, across processes. An exclusive flock on `<project>/.nself/op.lock`
// is the lock; the kernel drops it when the holder dies, so a crash never
// leaves it held (the old `.nself/build.lock` was O_EXCL and went stale).
//
// Inputs: the project directory, the command name, and Opts (wait budget,
// injected clock, environment lookup).
//
// Outputs: a *Lock whose Release is idempotent and nil-safe; *HeldError
// (errors.Is ErrHeld) when another process holds the lock; ErrUnsupported on
// platforms without flock. The holder record (contract:cli.oplock v1) is
// written into the file after locking; see holder.go.
//
// Constraints:
//   - Cooperative lock: a same-user process can ignore it, and the token in the
//     holder file lets any same-user reader pass for a child (Safe-On-Live.md).
//   - Re-entrancy: a process whose NSELF_OPLOCK_TOKEN equals the token in the
//     holder file of a currently held lock runs without acquiring, so a child
//     `nself` spawned by the holder never deadlocks. A child that outlives its
//     parent runs unlocked.
//   - Layer L1: standard library, plus internal/compat and internal/errs (L0)
//     in guard.go.
package oplock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EnvToken is the environment variable that carries the holder's token to
// child processes.
const EnvToken = "NSELF_OPLOCK_TOKEN"

// ErrHeld is matched (errors.Is) by every *HeldError.
var ErrHeld = errors.New("project operation lock is held")

// ErrUnsupported is returned by Acquire on a platform without flock.
var ErrUnsupported = errors.New("project operation lock is not supported on this platform")

// HeldError reports a lock held by another process.
type HeldError struct {
	// Holder is the holder record; meaningful only when Known.
	Holder Holder
	// Known is false when the file was empty or unreadable (a holder that has
	// locked but not yet written its record).
	Known bool
	// Waited is how long Acquire polled before giving up.
	Waited time.Duration
}

// Error names the holder when it is known.
func (e *HeldError) Error() string {
	if !e.Known {
		return "another nself operation holds the project lock"
	}
	return fmt.Sprintf("%s (pid %d) holds the project lock", e.Holder.Command, e.Holder.PID)
}

// Is makes errors.Is(err, ErrHeld) true.
func (e *HeldError) Is(target error) bool { return target == ErrHeld }

// Clock injects time so tests need no bare sleeps.
type Clock struct {
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration) error
}

// RealClock is the wall clock.
func RealClock() Clock {
	return Clock{Now: time.Now, Sleep: func(ctx context.Context, d time.Duration) error {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}}
}

// Opts configures one Acquire.
type Opts struct {
	// Command is recorded in the holder file, e.g. "nself build".
	Command string
	// Wait is how long to poll a held lock; 0 fails fast.
	Wait time.Duration
	// Poll is the polling interval while waiting (default 250 ms).
	Poll time.Duration
	// Clock defaults to RealClock.
	Clock Clock
	// Getenv defaults to os.Getenv; it supplies EnvToken.
	Getenv func(string) string
	// OnWait, when set, is called once, on the first sight of a held lock
	// that Acquire is going to poll.
	OnWait func(h Holder, known bool)
}

func (o Opts) withDefaults() Opts {
	if o.Clock.Now == nil || o.Clock.Sleep == nil {
		o.Clock = RealClock()
	}
	if o.Poll <= 0 {
		o.Poll = 250 * time.Millisecond
	}
	if o.Getenv == nil {
		o.Getenv = os.Getenv
	}
	return o
}

// Lock is a held (or re-entered) operation lock.
type Lock struct {
	f         *os.File
	holder    Holder
	reentrant bool
	once      sync.Once
}

// Holder returns the record of this lock (the parent's record when re-entered).
func (l *Lock) Holder() Holder { return l.holder }

// Token returns the token children must inherit as EnvToken.
func (l *Lock) Token() string { return l.holder.Token }

// Reentrant reports that this process re-entered its ancestor's lock and owns
// nothing: Release does nothing.
func (l *Lock) Reentrant() bool { return l != nil && l.reentrant }

// Release drops the flock. It is idempotent and safe on a nil Lock. The holder
// file stays; only the flock means "held".
func (l *Lock) Release() {
	if l == nil || l.f == nil {
		return
	}
	l.once.Do(func() {
		unlock(l.f)
		_ = l.f.Close()
	})
}

// Acquire takes the lock of projectDir, creating `.nself/op.lock` when needed.
// It polls a held lock for opts.Wait and then returns a *HeldError. A process
// that carries the holder's token returns a re-entrant Lock without locking.
func Acquire(ctx context.Context, projectDir string, opts Opts) (*Lock, error) {
	if !supported {
		return nil, ErrUnsupported
	}
	o := opts.withDefaults()
	token := o.Getenv(EnvToken)
	start := o.Clock.Now()
	waiting := false
	for tries := 0; ; {
		f, err := openLockFile(projectDir)
		if err != nil {
			return nil, err
		}
		held, err := tryLock(f)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		if !held {
			if sameAsPath(f, projectDir) || tries >= 5 {
				return finishAcquire(f, o)
			}
			// The file was unlinked or replaced while we locked it (a clean
			// removed .nself): the lock we hold guards nothing. Retry.
			tries++
			unlock(f)
			_ = f.Close()
			continue
		}
		_ = f.Close()
		h, known := readHolderSettled(ctx, projectDir, o)
		if known && token != "" && h.Token == token {
			return &Lock{holder: h, reentrant: true}, nil
		}
		waited := o.Clock.Now().Sub(start)
		if o.Wait <= 0 || waited >= o.Wait {
			return nil, &HeldError{Holder: h, Known: known, Waited: waited}
		}
		if !waiting && o.OnWait != nil {
			waiting = true
			o.OnWait(h, known)
		}
		if err := o.Clock.Sleep(ctx, o.Poll); err != nil {
			return nil, err
		}
	}
}

// finishAcquire writes the holder record into the locked file f.
func finishAcquire(f *os.File, o Opts) (*Lock, error) {
	tok, err := newToken()
	if err != nil {
		unlock(f)
		_ = f.Close()
		return nil, err
	}
	host, _ := os.Hostname()
	h := Holder{PID: os.Getpid(), Command: o.Command, StartedAt: o.Clock.Now().UTC().Format(time.RFC3339), Token: tok, Host: host}
	if err := writeHolder(f, h); err != nil {
		unlock(f)
		_ = f.Close()
		return nil, err
	}
	return &Lock{f: f, holder: h}, nil
}

// openLockFile opens (creating) the lock file with the directory it needs.
func openLockFile(projectDir string) (*os.File, error) {
	path := LockPath(projectDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
}

// sameAsPath reports whether f is still the file at the lock path.
func sameAsPath(f *os.File, projectDir string) bool {
	a, err := f.Stat()
	if err != nil {
		return false
	}
	b, err := os.Stat(LockPath(projectDir))
	return err == nil && os.SameFile(a, b)
}

// readHolderSettled reads the holder record, retrying briefly: a holder that
// has just locked has not necessarily written its record yet.
func readHolderSettled(ctx context.Context, projectDir string, o Opts) (Holder, bool) {
	for i := 0; i < 5; i++ {
		if h, err := ReadHolder(projectDir); err == nil {
			return h, true
		}
		if o.Clock.Sleep(ctx, 10*time.Millisecond) != nil {
			break
		}
	}
	return Holder{}, false
}
