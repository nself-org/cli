// tx.go — a held writer lock on the ledger directory.
//
// Purpose: let a caller do several steps (plan, checkpoint, side effects, final
// write) under ONE lock, with durable writes in between (D-0368).
// Inputs: a func run under the lock, with the loaded ledger.
// Outputs: Locked runs it; Tx.Write persists a ledger; the intent methods in
// intent.go read and write the remove-intent file through the same Tx.
// Constraints: Locked takes the operation lock first (oplock.go). Nothing is written unless fn calls Write (or an intent method).
// Tx is valid only inside fn; it must not be kept or shared across goroutines.
package ledger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Tx is a held writer lock (see Store.Locked).
type Tx struct{ s *Store }

// Locked takes the writer lock, removes stale temp files, loads the ledger
// (bootstrapping a missing file with the same rules as Update) and runs fn. A
// missing ledger is NOT written by Locked itself: call tx.Write to persist it.
func (s *Store) Locked(fn func(tx *Tx, l Ledger) error) error {
	return s.Operation(context.Background(), "bundle ledger write", func() error { return s.locked(fn) })
}

// locked is Locked under the operation lock (oplock.go), which makes a ledger
// writer wait behind a long bundle install or remove instead of failing.
func (s *Store) locked(fn func(tx *Tx, l Ledger) error) error {
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

	l, _, err := s.load(true)
	if err != nil {
		return err
	}
	return fn(&Tx{s: s}, l)
}

// Write validates and atomically, durably replaces the ledger. The lock stays
// held, so several Writes in one Locked are fine.
func (tx *Tx) Write(l Ledger) error { return tx.s.write(l) }
