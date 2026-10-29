package envrestore

// Purpose: capture an env file before one line write and put its bytes back if
// the surrounding apply then fails (P7-LIVE-05). The restore writes a temp file
// next to the target, syncs it, gives it the target's owner and renames it over
// the target, so a crash cannot leave a truncated env file; where that is not
// possible (a read-only directory, an owner it cannot set) it rewrites the file
// in place. A symlinked env file is restored at its target and the link is
// kept; a target the write created through a dangling link is removed again.
// SIGINT, SIGTERM, SIGHUP and SIGQUIT are held from Capture until
// ReleaseSignals.
// Inputs: the env file path. Outputs: the captured state, the restore.
// Constraints: the restored file keeps the bytes it had; its mode is 0600.

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/nself-org/cli/internal/security"
)

// File is one env file and what it needs to undo a write to it.
type File struct {
	Path           string
	Written        bool // set by Capture, before the caller writes: a partial write is restored too
	prev           []byte
	prevInfo       os.FileInfo // the target's info before the write (owner)
	danglingTarget string      // the file a write through a dangling link creates
	existed        bool
	sigs           chan os.Signal
	held           os.Signal
}

// Capture holds interrupts and records the current bytes of the file.
func (f *File) Capture() error {
	f.holdSignals()
	data, err := os.ReadFile(f.Path)
	switch {
	case err == nil:
		f.prev, f.existed = data, true
		f.prevInfo, _ = os.Stat(f.Path)
	case !os.IsNotExist(err):
		return fmt.Errorf("reading %s: %w", f.Path, err)
	default:
		f.danglingTarget = danglingLinkTarget(f.Path)
	}
	f.Written = true
	return nil
}

// RemoveLine drops key's statement from the captured bytes with without (the
// planner's dotenv grammar, injected to keep this package below reconcile). A
// file that did not exist stays absent.
func (f *File) RemoveLine(key string, without func(prev []byte, key string) ([]byte, error)) error {
	if !f.existed {
		return nil
	}
	data, err := without(f.prev, key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(f.Path, data, 0o600); err != nil {
		return err
	}
	return security.EnforceFilePermissions(f.Path, 0o600)
}

// Restore puts back the bytes the write replaced, atomically, or removes a
// file the write created. Nothing to do when Capture never ran.
func (f *File) Restore() error {
	if !f.Written {
		return nil
	}
	if !f.existed {
		victim := f.Path
		if f.danglingTarget != "" {
			victim = f.danglingTarget // keep the user's link, remove what it pointed the write at
		}
		if err := os.Remove(victim); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	target, err := filepath.EvalSymlinks(f.Path)
	if err != nil {
		return err
	}
	if f.restoreAtomic(target) == nil {
		return nil
	}
	// The same in-place write the line write made: it needs no new directory
	// entry and keeps the file's owner.
	if err := os.WriteFile(target, f.prev, 0600); err != nil {
		return err
	}
	return security.EnforceFilePermissions(target, 0600)
}

// restoreAtomic writes prev to a temp file beside target, syncs it, gives it
// the target's owner and renames it over target. Any error leaves target as
// it was (the temp file is removed).
func (f *File) restoreAtomic(target string) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".nself-restore-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(f.prev); err != nil {
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
	if err := keepOwner(tmp.Name(), f.prevInfo); err != nil {
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
func (f *File) holdSignals() {
	if f.sigs != nil {
		return
	}
	f.sigs = make(chan os.Signal, 1)
	signal.Notify(f.sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
}

// ReleaseSignals stops holding and returns the first signal held, if any.
// Safe to call more than once.
func (f *File) ReleaseSignals() os.Signal {
	if f.sigs == nil {
		return nil
	}
	signal.Stop(f.sigs)
	select {
	case s := <-f.sigs:
		f.held = s
	default:
	}
	f.sigs = nil
	held := f.held
	f.held = nil
	return held
}
