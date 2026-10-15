package commands

// config_set_restore.go — the env-line write of `nself config set` and its
// rollback (P7-LIVE-05).
//
// Purpose: write the line inside reconcile.Apply's held apply, remember the
// bytes it replaced, and put them back if the apply then fails. The restore
// writes a temp file next to the target, syncs it, gives it the target's
// owner and renames it over the target, so a crash cannot leave a truncated
// env file; where that is not possible (a read-only directory, an owner it
// cannot set) it rewrites the file in place, as the line write itself did. A
// symlinked env file is restored at its target and the link is kept; a target
// the write created through a dangling link is removed again. SIGINT, SIGTERM,
// SIGHUP and SIGQUIT are held from the write until the apply returns.
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
	prevInfo         os.FileInfo // the target's info before the write (owner)
	danglingTarget   string      // the file a write through a dangling link creates
	existed, written bool
	updated          bool
	sigs             chan os.Signal
	held             os.Signal
}

// write holds interrupts, records the current bytes, then writes the line.
func (w *envLineWrite) write(key, value string) error {
	return w.writeEnvPresence(key, value, true)
}

// writeEnvPresence records the file before setting or removing one dotenv statement.
func (w *envLineWrite) writeEnvPresence(key, value string, present bool) error {
	w.holdSignals()
	data, err := os.ReadFile(w.path)
	switch {
	case err == nil:
		w.prev, w.existed = data, true
		w.prevInfo, _ = os.Stat(w.path)
	case !os.IsNotExist(err):
		return fmt.Errorf("reading %s: %w", w.path, err)
	default:
		w.danglingTarget = danglingLinkTarget(w.path)
	}
	w.written = true // set before the write: a partial write is restored too
	if !present {
		return w.removeEnvLine(key)
	}
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
		victim := w.path
		if w.danglingTarget != "" {
			victim = w.danglingTarget // keep the user's link, remove what it pointed the write at
		}
		if err := os.Remove(victim); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	target, err := filepath.EvalSymlinks(w.path)
	if err != nil {
		return err
	}
	if w.restoreAtomic(target) == nil {
		return nil
	}
	// The same in-place write the line write made: it needs no new directory
	// entry and keeps the file's owner.
	if err := os.WriteFile(target, w.prev, 0600); err != nil {
		return err
	}
	return security.EnforceFilePermissions(target, 0600)
}

// restoreAtomic writes prev to a temp file beside target, syncs it, gives it
// the target's owner and renames it over target. Any error leaves target as
// it was (the temp file is removed).
func (w *envLineWrite) restoreAtomic(target string) error {
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
	if err := keepOwner(tmp.Name(), w.prevInfo); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}

// danglingLinkTarget returns the path a write through p would create when p
// is a symlink whose target does not exist, else "".
func danglingLinkTarget(p string) string {
	info, err := os.Lstat(p)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return ""
	}
	dest, err := os.Readlink(p)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(filepath.Dir(p), dest)
	}
	return dest
}

// holdSignals starts holding SIGINT, SIGTERM, SIGHUP (an SSH drop) and
// SIGQUIT (once).
func (w *envLineWrite) holdSignals() {
	if w.sigs != nil {
		return
	}
	w.sigs = make(chan os.Signal, 1)
	signal.Notify(w.sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
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
