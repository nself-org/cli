// Store: read and atomically update the ledger file of one project.
//
// Purpose: the only code that touches <root>/.nself/state/bundles.json.
// Inputs: a project root and an optional Sources func used to bootstrap.
// Outputs: Load (read only), Update (read, change, validate, write).
// Constraints: writers take an OS lock on bundles.json.lock (next to the
// ledger), write a temp file in the same directory, fsync it, then rename over
// the ledger, so a reader sees the old or the new file, never a partial one,
// and a crash between the temp write and the rename leaves the old ledger
// intact. The file is 0644 (it holds no secret), its directory 0700 (same as
// the other .nself/state files).
// Load never writes and never creates a directory.
package ledger

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	fileName = "bundles.json"
	lockName = "bundles.json.lock"
	lockWait = 15 * time.Second
)

// Store is the ledger of one project.
type Store struct {
	root string
	// Sources supplies bootstrap input when the file does not exist yet. It
	// returns an error when the installed plugins or bundle membership cannot be
	// read. A missing file with a nil Sources, or a Sources error, is an error
	// (never an empty ledger): an empty ledger would be written and then hide
	// the real install state, because bootstrap runs only once.
	Sources func() (Sources, error)
	// Warn, when set, receives what Bootstrap had to skip (invalid slugs).
	Warn func(msg string)

	// beforeRename is a test seam: an error here aborts the write after the
	// temp file is complete and before it replaces the ledger.
	beforeRename func() error
}

// NewStore returns the store for the project at root.
func NewStore(root string) *Store { return &Store{root: root} }

// Dir is <root>/.nself/state.
func (s *Store) Dir() string { return filepath.Join(s.root, ".nself", "state") }

// Path is the ledger file.
func (s *Store) Path() string { return filepath.Join(s.Dir(), fileName) }

func (s *Store) bootstrap() (Ledger, error) {
	if s.Sources == nil {
		return Ledger{}, fmt.Errorf("ledger: %s does not exist and no bootstrap source is configured", s.Path())
	}
	src, err := s.Sources()
	if err != nil {
		return Ledger{}, fmt.Errorf("ledger: bootstrap source: %w", err)
	}
	l, skipped := BootstrapReport(src)
	if s.Warn != nil {
		for _, m := range skipped {
			s.Warn(m)
		}
	}
	return l, nil
}

// Load returns the ledger and whether it came from the file. A missing file is
// not an error: the bootstrap result is returned (persisted=false) and nothing
// is written. A damaged file is an error.
func (s *Store) Load() (l Ledger, persisted bool, err error) {
	data, err := readCapped(s.Path())
	if errors.Is(err, os.ErrNotExist) {
		l, err = s.bootstrap()
		return l, false, err
	}
	if err != nil {
		return Ledger{}, false, fmt.Errorf("ledger: read %s: %w", s.Path(), err)
	}
	l, err = Parse(data)
	if err != nil {
		return Ledger{}, false, fmt.Errorf("%w (file %s; remove it to re-bootstrap from the installed plugins)", err, s.Path())
	}
	return l, true, nil
}

// Update runs fn on the current ledger (bootstrapped first when the file is
// missing) under the writer lock and writes the result. fn's error aborts the
// write. Two concurrent Updates never lose each other's change.
func (s *Store) Update(fn func(*Ledger) error) error {
	if err := os.MkdirAll(s.Dir(), 0o700); err != nil {
		return fmt.Errorf("ledger: create %s: %w", s.Dir(), err)
	}
	lf, err := openLockFile(filepath.Join(s.Dir(), lockName))
	if err != nil {
		return fmt.Errorf("ledger: open lock: %w", err)
	}
	defer func() { _ = lf.Close() }()
	unlock, err := lockFile(lf, lockWait)
	if err != nil {
		return fmt.Errorf("ledger: lock: %w", err)
	}
	defer unlock()
	s.removeStaleTemps()

	l, _, err := s.Load()
	if err != nil {
		return err
	}
	if err := fn(&l); err != nil {
		return err
	}
	return s.write(l)
}

// removeStaleTemps deletes temp files a killed writer left behind. The caller
// holds the lock, so no live writer owns one; the age guard (older than the lock
// wait) is a second safeguard.
func (s *Store) removeStaleTemps() {
	old, _ := filepath.Glob(filepath.Join(s.Dir(), ".bundles.json.*.tmp"))
	for _, p := range old {
		if st, err := os.Lstat(p); err == nil && time.Since(st.ModTime()) > lockWait {
			_ = os.Remove(p)
		}
	}
}

// readCapped reads the file at path, refusing one larger than MaxBytes.
func readCapped(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("file is larger than %d bytes", MaxBytes)
	}
	return data, nil
}

// EnsureBootstrapped writes the bootstrap ledger when no file exists and
// reports whether it did. An existing ledger is left untouched.
func (s *Store) EnsureBootstrapped() (created bool, err error) {
	err = s.Update(func(*Ledger) error {
		if _, statErr := os.Stat(s.Path()); statErr == nil {
			return errAlreadyThere
		}
		created = true
		return nil
	})
	if errors.Is(err, errAlreadyThere) {
		return false, nil
	}
	return created, err
}

var errAlreadyThere = errors.New("ledger exists")

// write renders l and replaces the ledger atomically. The caller holds the lock.
func (s *Store) write(l Ledger) error {
	data, err := l.Marshal()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir(), ".bundles.json.*.tmp")
	if err != nil {
		return fmt.Errorf("ledger: temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("ledger: write temp: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("ledger: chmod temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("ledger: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("ledger: close temp: %w", err)
	}
	if s.beforeRename != nil {
		if err := s.beforeRename(); err != nil {
			cleanup()
			return err
		}
	}
	if err := os.Rename(tmpName, s.Path()); err != nil {
		cleanup()
		return fmt.Errorf("ledger: replace %s: %w", s.Path(), err)
	}
	syncDir(s.Dir())
	return nil
}

// syncDir flushes the directory entry; best effort (not every filesystem allows it).
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}
