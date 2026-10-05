package oplock

// Guard: the policy the invocation decorator applies around one command.
//
// Purpose: decide whether a command takes the lock (registry class), take it
// with the v1.4 or v1.5 contention policy, export the re-entrancy token for
// child processes, and hand back one release function for every exit path.
//
// Inputs: Request{Dir, Command, SideEffect, Output}. SideEffect and Output are
// the command's registry classes after flag escalation (EPIC D12).
//
// Outputs: a context carrying the Lock (FromContext, for early release), a
// release func that is always safe to defer, and an error only when v1.5
// contention must stop the command (E460).
//
// Constraints:
//   - Takes the lock only for side_effect write, remote or destructive with
//     output kind document. read, stream and interactive commands never do.
//   - v1.5: a held lock fails fast with E460. v1.4: poll up to 30 s, then warn
//     on stderr and run unlocked. Platform or I/O failures never block a
//     command: a notice goes to stderr and the command runs unlocked.
//   - Notices go to Stderr, never stdout, so a --json envelope stays one
//     document.

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
)

// Output kind and side-effect class names mirror internal/canon; this package
// stays stdlib-light, and a test over the real registry pins the mapping.
const (
	outputDocument = "document"
	waitV14        = 30 * time.Second
)

// Stderr receives notices and warnings. A variable so tests can capture it.
var Stderr io.Writer = os.Stderr

// DefaultClock is the clock Guard uses. Tests replace it.
var DefaultClock = RealClock()

var unsupportedOnce sync.Once

// notef writes one notice line to Stderr; a failed write has no better channel.
func notef(format string, args ...any) { _, _ = fmt.Fprintf(Stderr, format, args...) }

// Request describes one invocation.
type Request struct {
	// Dir is the resolved project root.
	Dir string
	// Command is the display name recorded in the holder file ("nself build").
	Command string
	// SideEffect and Output are the effective registry classes.
	SideEffect string
	Output     string
}

type ctxKey struct{}

// WithLock returns ctx carrying l.
func WithLock(ctx context.Context, l *Lock) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext returns the Lock Guard stored in ctx, or nil.
func FromContext(ctx context.Context) *Lock {
	if ctx == nil {
		return nil
	}
	l, _ := ctx.Value(ctxKey{}).(*Lock)
	return l
}

// Takes reports whether a command class takes the lock.
func Takes(sideEffect, output string) bool {
	switch sideEffect {
	case "write", "remote", "destructive":
		return output == outputDocument
	}
	return false
}

// Guard applies the lock policy for r. See the file comment.
func Guard(ctx context.Context, r Request) (context.Context, func(), error) {
	noop := func() {}
	if !Takes(r.SideEffect, r.Output) {
		return ctx, noop, nil
	}
	o := Opts{Command: r.Command, Poll: 250 * time.Millisecond, Clock: DefaultClock}
	// compat.V15(P7-LIVE-13): a held lock is polled for 30 s, then a warning and the command runs unlocked -> a held lock fails at once with E460
	v15 := compat.V15()
	if !v15 {
		o.Wait = waitV14
		o.OnWait = func(h Holder, known bool) {
			notef("nself: waiting up to %s for %s\n", waitV14, holderText(h, known))
		}
	}
	lock, err := Acquire(ctx, r.Dir, o)
	switch {
	case err == nil:
	case err == ErrUnsupported:
		unsupportedOnce.Do(func() {
			notef("nself: the project operation lock is not supported on this platform; running without it\n")
		})
		return ctx, noop, nil
	case isHeld(err):
		he := err.(*HeldError)
		if v15 {
			return ctx, noop, heldCLIError(he)
		}
		notef("nself: warning: %s after %s; continuing without the lock\n", holderText(he.Holder, he.Known), he.Waited.Round(time.Second))
		return ctx, noop, nil
	default:
		notef("nself: warning: project operation lock unavailable (%v); continuing without it\n", err)
		return ctx, noop, nil
	}
	restore := exportToken(lock)
	return WithLock(ctx, lock), func() { lock.Release(); restore() }, nil
}

// exportToken sets EnvToken for children and returns the undo. A re-entered
// lock already inherited the token.
func exportToken(l *Lock) func() {
	if l.Reentrant() {
		return func() {}
	}
	prev, had := os.LookupEnv(EnvToken)
	_ = os.Setenv(EnvToken, l.Token())
	return func() {
		if had {
			_ = os.Setenv(EnvToken, prev)
		} else {
			_ = os.Unsetenv(EnvToken)
		}
	}
}

func isHeld(err error) bool { _, ok := err.(*HeldError); return ok }

// holderText renders the holder for a message.
func holderText(h Holder, known bool) string {
	if !known {
		return "another nself operation"
	}
	t := fmt.Sprintf("%s (pid %d)", h.Command, h.PID)
	if host, _ := os.Hostname(); h.Host != "" && h.Host != host {
		t += " on " + h.Host
	}
	return t
}

// heldCLIError builds the E460 error naming the holder.
func heldCLIError(he *HeldError) error {
	e := errs.New("E460", "another nself operation holds the project lock: "+holderText(he.Holder, he.Known))
	if he.Known {
		e.Fix = fmt.Sprintf("wait for %s or stop it, then run this command again", holderText(he.Holder, true))
	}
	e.Wrapped = he
	return e
}
