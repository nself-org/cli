package commands

// config_set_restore.go — the env-line write of `nself config set` and its
// rollback (P7-LIVE-05).
//
// Purpose: write the line inside reconcile.Apply's held apply, remember the
// bytes it replaced, and put them back if the apply then fails, without ever
// leaving a truncated env file: the restore writes a temp file in the same
// directory, syncs it and renames it over the target (a symlinked env file is
// restored at its target, the link kept). Interrupts are held from the write
// until the apply returns.
// Inputs: the env file path, key and value. Outputs: the write, the restore.
// Constraints: the restored file keeps the bytes it had; its mode is 0600, as
// every env write leaves it (setEnvFileLine already forced 0600).

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/nself-org/cli/internal/security"
)

// envLineWrite is one env-line write and what it needs to undo it.
type envLineWrite struct {
	path             string
	prev             []byte
	existed, written bool
	updated          bool
	sigs             chan os.Signal
	held             os.Signal
}

// write holds interrupts, records the current bytes, then writes the line.
func (w *envLineWrite) write(key, value string) error {
	w.holdSignals()
	data, err := os.ReadFile(w.path)
	switch {
	case err == nil:
		w.prev, w.existed = data, true
	case !os.IsNotExist(err):
		return fmt.Errorf("reading %s: %w", w.path, err)
	}
	w.written = true // set before the write: a partial write is restored too
	w.updated, err = setEnvFileLine(w.path, key, value)
	return err
}

// restore puts back the bytes the write replaced, atomically, or removes a
// file the write created. Nothing to do when the write never ran.
func (w *envLineWrite) restore() error {
	if !w.written {
		return nil
	}
	if !w.existed {
		if err := os.Remove(w.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	target, err := filepath.EvalSymlinks(w.path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".nself-restore-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(w.prev); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := security.EnforceFilePermissions(tmp.Name(), 0600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}

// holdSignals starts holding SIGINT and SIGTERM (once).
func (w *envLineWrite) holdSignals() {
	if w.sigs != nil {
		return
	}
	w.sigs = make(chan os.Signal, 1)
	signal.Notify(w.sigs, os.Interrupt, syscall.SIGTERM)
}

// releaseSignals stops holding and returns the first signal held, if any.
// Safe to call more than once.
func (w *envLineWrite) releaseSignals() os.Signal {
	if w.sigs == nil {
		return nil
	}
	signal.Stop(w.sigs)
	select {
	case s := <-w.sigs:
		w.held = s
	default:
	}
	w.sigs = nil
	held := w.held
	w.held = nil
	return held
}
