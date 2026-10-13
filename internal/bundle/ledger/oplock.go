// oplock.go — the install/remove operation lock (D-0322).
//
// Purpose: one lock, separate from the short ledger write lock, that a long
// bundle install or remove holds from its first ledger read to its last write.
// Everything that writes the ledger (Update, Locked, EnsureBootstrapped, and the
// plugin install/remove callers outside this package) takes it first, so a writer
// that arrives during a long operation waits behind it with progress instead of
// failing after the 15 s ledger lock wait.
// Inputs: a Store (the plugin directory), a short label for the operation, a func.
// Outputs: Operation runs the func while holding <pluginDir>/.nself-state/operation.lock.
// Constraints: the lock serialises nself processes; callers inside one process
// share it (a CLI runs one command per process), and the ledger write lock still
// serialises the actual writes. It is reentrant per process, keyed by the cleaned
// absolute state directory: bundle install calls Store.Update while it holds the
// lock, and a second flock on a new descriptor in the same process would block
// for ever. Waiters poll with no overall deadline, report progress, and stop on
// context cancel. The holder writes one line (pid, label, start time) into the
// lock file for the waiters' message; the label never carries a secret. On
// Windows lockFile is a no-op, so this lock is too.
package ledger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const opLockName = "operation.lock"

// Test seams: how long one lock attempt polls, when the first progress line
// appears, and how often it repeats.
var (
	opPollSlice     = 250 * time.Millisecond
	opProgressFirst = 2 * time.Second
	opProgressEvery = 15 * time.Second
)

// errLockBusy is what lockFile returns when its wait elapsed.
var errLockBusy = errors.New("lock is held")

// opHold is the process-wide state of one operation lock.
type opHold struct {
	count int           // holders in this process (0 while still acquiring)
	ready chan struct{} // closed when acquisition finished (success or not)
	err   error         // acquisition error, valid after ready closes
	drop  func()        // releases the OS lock
}

var opProcess = struct {
	mu   sync.Mutex
	held map[string]*opHold
}{held: map[string]*opHold{}}

// Operation runs fn while this process holds the operation lock of the plugin
// directory, waiting (with progress on s.Progress or stderr) while another nself
// process holds it. what labels the operation for the waiters, e.g.
// "bundle install nchat". A cancelled ctx stops the wait with an error that wraps
// ctx.Err(); fn then never runs.
func (s *Store) Operation(ctx context.Context, what string, fn func() error) error {
	// Resolve the key only after the state directory exists. Otherwise the first
	// hold uses an unresolved path, while a nested writer resolves its symlinked
	// parents (e.g. macOS /tmp), misses the hold, and waits on its own flock.
	if err := os.MkdirAll(s.Dir(), 0o700); err != nil {
		return fmt.Errorf("ledger: create %s: %w", s.Dir(), err)
	}
	key := opKey(s.Dir())
	h, err := s.opAcquire(ctx, key, what)
	if err != nil {
		return err
	}
	defer opRelease(key, h)
	return fn()
}

// opKey is the process-wide key of a state directory: absolute, cleaned and with
// symlinks resolved when the directory exists, so two spellings of one directory
// (macOS /tmp vs /private/tmp) share one hold instead of deadlocking on a second flock.
func opKey(dir string) string {
	key, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		key = filepath.Clean(dir)
	}
	if real, err := filepath.EvalSymlinks(key); err == nil {
		key = real
	}
	return key
}

// opAcquire joins the hold of this process or becomes the one that takes the OS lock.
func (s *Store) opAcquire(ctx context.Context, key, what string) (*opHold, error) {
	for {
		opProcess.mu.Lock()
		h, ok := opProcess.held[key]
		if !ok {
			h = &opHold{ready: make(chan struct{})}
			opProcess.held[key] = h
			h.count = 1
			opProcess.mu.Unlock()
			return h, s.opTake(ctx, key, h, what)
		}
		h.count++
		opProcess.mu.Unlock()
		select {
		case <-h.ready:
		case <-ctx.Done():
			opRelease(key, h)
			return nil, fmt.Errorf("waiting for the bundle operation lock: %w", ctx.Err())
		}
		if h.err == nil {
			return h, nil
		}
		opRelease(key, h) // the acquirer failed and removed the entry: try again
	}
}

// opTake takes the OS lock for h. On failure it removes the entry, wakes the
// goroutines that joined, and returns the error.
func (s *Store) opTake(ctx context.Context, key string, h *opHold, what string) error {
	drop, err := s.opLockOS(ctx, what)
	opProcess.mu.Lock()
	h.err = err
	h.drop = drop
	if err != nil {
		delete(opProcess.held, key)
		h.count--
	}
	opProcess.mu.Unlock()
	close(h.ready)
	return err
}

// opRelease drops one hold; the last one releases the OS lock.
func opRelease(key string, h *opHold) {
	opProcess.mu.Lock()
	h.count--
	last := h.count <= 0 && h.err == nil
	if last && opProcess.held[key] == h {
		delete(opProcess.held, key)
	}
	opProcess.mu.Unlock()
	if last && h.drop != nil {
		h.drop()
	}
}

// opLockOS polls for the OS lock and writes the holder note once it has it.
func (s *Store) opLockOS(ctx context.Context, what string) (func(), error) {
	path := filepath.Join(s.Dir(), opLockName)
	f, err := openLockFile(path)
	if err != nil {
		return nil, fmt.Errorf("ledger: open operation lock: %w", err)
	}
	start := time.Now()
	nextNote := opProgressFirst
	for {
		if err := ctx.Err(); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("waiting for the bundle operation lock: %w", err)
		}
		unlock, lerr := lockFile(f, opPollSlice)
		if lerr == nil {
			writeHolderNote(f, what)
			return func() {
				_ = f.Truncate(0)
				unlock()
				_ = f.Close()
			}, nil
		}
		if !errors.Is(lerr, errLockBusy) {
			_ = f.Close()
			return nil, fmt.Errorf("ledger: operation lock: %w", lerr)
		}
		if waited := time.Since(start); waited >= nextNote {
			s.progress(fmt.Sprintf("waiting for another nself operation on %s to finish (%s; waited %s); press Ctrl-C to stop",
				s.root, readHolderNote(path), roundWait(waited)))
			nextNote = waited + opProgressEvery
		}
	}
}

// progress sends one line to Store.Progress, or to stderr when it is nil.
func (s *Store) progress(msg string) {
	if s.Progress != nil {
		s.Progress(msg)
		return
	}
	_, _ = fmt.Fprintln(os.Stderr, msg)
}

func roundWait(d time.Duration) time.Duration {
	if d < 10*time.Second {
		return d.Round(10 * time.Millisecond)
	}
	return d.Round(time.Second)
}

// writeHolderNote records who holds the lock; best effort.
func writeHolderNote(f *os.File, what string) {
	note := fmt.Sprintf("pid=%d op=%s since=%s", os.Getpid(), cleanNote(what), time.Now().UTC().Format(time.RFC3339))
	if f.Truncate(0) == nil {
		_, _ = f.WriteAt([]byte(note), 0)
	}
}

// readHolderNote returns the holder's note, or "holder unknown"; best effort.
func readHolderNote(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "holder unknown"
	}
	defer func() { _ = f.Close() }()
	data, _ := io.ReadAll(io.LimitReader(f, 200))
	if note := cleanNote(string(data)); note != "" {
		return note
	}
	return "holder unknown"
}

// cleanNote keeps printable ASCII only, so a note from another process cannot
// inject control characters into the terminal.
func cleanNote(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e {
			return -1
		}
		return r
	}, s))
}
