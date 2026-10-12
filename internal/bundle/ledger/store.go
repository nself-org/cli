// Store: read and atomically update the ledger file of one plugin directory.
//
// Purpose: the only code that touches <pluginDir>/.nself-state/bundles.json.
// Inputs: the plugin directory ($NSELF_PLUGIN_DIR or ~/.nself/plugins) and an
// optional Sources func used to bootstrap. The ledger lives with the plugins it
// describes (D-0284 option A), so it is the same for every project that shares
// that plugin directory and needs no project root.
// Outputs: Load (read only), Update (read, change, validate, write), Locked (a
// held writer lock for several steps with durable writes in between, tx.go).
// Constraints: writers take an OS lock on bundles.json.lock (next to the
// ledger), write a temp file in the same directory, fsync it, then rename over
// the ledger, so a reader sees the old or the new file, never a partial one,
// and a crash between the temp write and the rename leaves the old ledger
// intact. The file is 0644 (it holds no secret), its directory 0700 (same as
// the other state directories). The directory name starts with a dot and holds
// no plugin.json, so plugin scans skip it.
// Load never writes and never creates a directory. A bootstrap scan that could
// not read every plugin manifest (Sources.Unreadable) may be shown by Load but
// is never persisted by Update (D-0374): bootstrap runs once, so a persisted
// partial scan would hide those plugins' bundle references for good.
package ledger

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	fileName = "bundles.json"
	lockName = "bundles.json.lock"
	lockWait = 15 * time.Second

	ledgerTempPattern = ".bundles.json.*.tmp"
)

// Store is the ledger of one plugin directory.
type Store struct {
	root string // the plugin directory
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
	// writeHook is a test seam (store_testhook.go): an error here fails a write
	// to that path before any temp file exists.
	writeHook func(path string) error
}

// StateDirName is the directory inside the plugin directory that holds the ledger.
const StateDirName = ".nself-state"

// NewStore returns the store for the plugin directory pluginDir.
func NewStore(pluginDir string) *Store { return &Store{root: pluginDir} }

// Dir is <pluginDir>/.nself-state.
func (s *Store) Dir() string { return filepath.Join(s.root, StateDirName) }

// Path is the ledger file.
func (s *Store) Path() string { return filepath.Join(s.Dir(), fileName) }

func (s *Store) bootstrap(persist bool) (Ledger, error) {
	if s.Sources == nil {
		return Ledger{}, fmt.Errorf("ledger: %s does not exist and no bootstrap source is configured", s.Path())
	}
	src, err := s.Sources()
	if err != nil {
		return Ledger{}, fmt.Errorf("ledger: bootstrap source: %w", err)
	}
	if len(src.Unreadable) > 0 {
		if persist {
			return Ledger{}, fmt.Errorf("ledger: cannot bootstrap %s: plugin manifests that exist could not be read (fix or remove them, then retry): %s",
				s.Path(), strings.Join(src.Unreadable, "; "))
		}
		if s.Warn != nil {
			for _, u := range src.Unreadable {
				s.Warn("plugin manifest could not be read and is not in this view: " + u)
			}
		}
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
	return s.load(false)
}

// load is Load; persist says the result will be written, which makes an
// incomplete bootstrap scan an error instead of a warning.
func (s *Store) load(persist bool) (l Ledger, persisted bool, err error) {
	data, err := readCapped(s.Path())
	if errors.Is(err, os.ErrNotExist) {
		l, err = s.bootstrap(persist)
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
	return s.Locked(func(tx *Tx, l Ledger) error {
		if err := fn(&l); err != nil {
			return err
		}
		return tx.Write(l)
	})
}

// removeStaleTemps deletes temp files a killed writer left behind. The caller
// holds the lock, so no live writer owns one; the age guard (older than the lock
// wait) is a second safeguard.
func (s *Store) removeStaleTemps() {
	for _, pattern := range tempPatterns {
		old, _ := filepath.Glob(filepath.Join(s.Dir(), pattern))
		for _, p := range old {
			if st, err := os.Lstat(p); err == nil && time.Since(st.ModTime()) > lockWait {
				_ = os.Remove(p)
			}
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
	// Load refuses a file over MaxBytes, so writing one would brick every later
	// Load and Update (D-0374). Refuse before the temp file exists.
	if len(data) > MaxBytes {
		return fmt.Errorf("ledger: refusing to write %d bytes: over the %d-byte limit Load accepts", len(data), MaxBytes)
	}
	return s.writeAtomic(s.Path(), ledgerTempPattern, data)
}
