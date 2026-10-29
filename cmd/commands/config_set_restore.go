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
	"os"

	"github.com/nself-org/cli/internal/envrestore"
	"github.com/nself-org/cli/internal/reconcile"
)

// envLineWrite is one env-line write and what it needs to undo it.
type envLineWrite struct {
	path    string
	file    envrestore.File
	updated bool
}

// written reports whether the file was captured, so a restore has work to do.
func (w *envLineWrite) written() bool { return w.file.Written }

// write holds interrupts, records the current bytes, then writes the line.
func (w *envLineWrite) write(key, value string) error {
	return w.writeEnvPresence(key, value, true)
}

// writeEnvPresence records the file before setting or removing one dotenv statement.
func (w *envLineWrite) writeEnvPresence(key, value string, present bool) error {
	w.file.Path = w.path
	if err := w.file.Capture(); err != nil {
		return err
	}
	if !present {
		return w.file.RemoveLine(key, reconcile.EnvFileWithoutLine)
	}
	var err error
	w.updated, err = setEnvFileLine(w.path, key, value)
	return err
}

// restore puts back the bytes the write replaced; see envrestore.File.Restore.
func (w *envLineWrite) restore() error { return w.file.Restore() }

// releaseSignals stops holding interrupts and returns the first one held.
func (w *envLineWrite) releaseSignals() os.Signal { return w.file.ReleaseSignals() }
