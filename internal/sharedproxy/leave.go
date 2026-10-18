package sharedproxy

// leave.go — a fronted project leaves the shared host proxy for good.
//
// Purpose: review R2b of EPIC ADOPT D2. `nself uninstall` in a fronted project
// detaches the owner nginx, removes the project's files and registration from
// the owner (Unregister), and reloads the owner so it stops serving them.
// Inputs: the owner dir and the project name.
// Outputs: the owner-relative files removed.
// Constraints: the reload happens only after a passing `nginx -t`; an owner
// whose nginx is not running is not an error (the files are gone either way).
// The project's quarantine tree is removed inside an os.Root on the owner dir
// after a plainDirs check, and before anything else is removed: a symlink on
// the way (nginx/shared/quarantine pointing elsewhere) is an E506 error and
// nothing is deleted.
// A reload that fails after the registration is gone (D-0931) must still happen
// on the next Leave, which finds no registration: a `<project>.reload-pending`
// file in the registration dir is written before anything is removed and
// deleted once the owner reloaded (or is not running, so it reads fresh files
// on its next start). Load ignores it (only `.json` files are registrations).

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// reloadPendingSuffix names the marker of a leave whose reload is still owed.
const reloadPendingSuffix = ".reload-pending"

// pendingRel returns the owner-relative path of project's reload marker.
func pendingRel(ownerDir, project string) (string, error) {
	return ownerRel(ownerDir, filepath.Join(DirOf(ownerDir), project+reloadPendingSuffix))
}

// reloadPending reports whether project left but the owner was not reloaded.
func reloadPending(ownerDir, project string) bool {
	rel, err := pendingRel(ownerDir, project)
	if err != nil {
		return false
	}
	fi, err := os.Lstat(filepath.Join(ownerDir, rel))
	return err == nil && fi.Mode().IsRegular()
}

// setReloadPending writes the marker inside an os.Root on the owner dir.
func setReloadPending(ownerDir, project string) error {
	root, err := os.OpenRoot(ownerDir)
	if err != nil {
		return e506("cannot open the shared proxy owner dir "+ownerDir, err, "")
	}
	defer func() { _ = root.Close() }()
	rel, err := pendingRel(ownerDir, project)
	if err != nil {
		return e506("cannot record the pending reload of "+project, err, "")
	}
	if err := plainDirs(root, filepath.Dir(rel)); err != nil {
		return e506("refusing to record the pending reload of "+project, err, "")
	}
	if err := root.WriteFile(rel, []byte(project+"\n"), 0o600); err != nil {
		return e506("cannot record the pending reload of "+project, err, "")
	}
	return nil
}

// clearReloadPending deletes the marker; a missing one is not an error.
func clearReloadPending(ownerDir, project string) error {
	root, err := os.OpenRoot(ownerDir)
	if err != nil {
		return e506("cannot open the shared proxy owner dir "+ownerDir, err, "")
	}
	defer func() { _ = root.Close() }()
	rel, err := pendingRel(ownerDir, project)
	if err != nil {
		return e506("cannot clear the pending reload of "+project, err, "")
	}
	_, err = removeIn(root, filepath.ToSlash(rel))
	return err
}

// reloadOwner tests and reloads the owner's nginx after project's files went,
// then clears the marker. A stopped owner is not an error: it serves nothing
// and reads the files on its next start.
func reloadOwner(ctx context.Context, ownerDir, project string) error {
	nginx, nerr := OwnerNginx(ctx, ownerDir)
	switch {
	case errors.Is(nerr, ErrOwnerNotRunning):
		// Warning level: the files are gone and a stopped owner serves nothing.
	case nerr != nil:
		// The files are gone but nginx still serves them; say so.
		return ownerErr(nerr)
	default:
		if _, stderr, terr := nginxTest(ctx, nginx); terr != nil {
			return e506("the owner's nginx -t fails after removing "+project+"; it was not reloaded", terr, stderr)
		}
		var o Outcome
		if err := reload(ctx, nginx, &o, func() {}); err != nil {
			return err
		}
	}
	return clearReloadPending(ownerDir, project)
}

// removeQuarantine checks project's quarantine path and, when remove is true,
// deletes the tree. A symlink on the way is refused, never followed.
func removeQuarantine(ownerDir, project string, remove bool) error {
	root, err := os.OpenRoot(ownerDir)
	if err != nil {
		return e506("cannot open the shared proxy owner dir "+ownerDir, err, "")
	}
	defer func() { _ = root.Close() }()
	baseRel, err := ownerRel(ownerDir, quarantineBase(ownerDir, project))
	if err != nil {
		return e506("cannot remove the quarantine of "+project, err, "")
	}
	if err := plainDirs(root, baseRel); err != nil {
		return e506("refusing to remove the quarantine of "+project, err, "")
	}
	if !remove {
		return nil
	}
	if err := root.RemoveAll(baseRel); err != nil {
		return e506("cannot remove the quarantine of "+project, err, "")
	}
	return nil
}

// leave detaches, unregisters and reloads. A project with no registration is a
// no-op. Leave is the locked entry.
func leave(ctx context.Context, ownerDir, project string) ([]string, error) {
	set, err := Load(DirOf(ownerDir), ownerDir)
	if err != nil {
		return nil, e506("the shared proxy registrations are not readable", err, "")
	}
	reg := set.Find(project)
	if reg == nil {
		if !reloadPending(ownerDir, project) {
			return nil, nil
		}
		// An earlier leave removed the registration but its reload failed.
		return nil, reloadOwner(ctx, ownerDir, project)
	}
	// Checked first: a refused quarantine path leaves registration and files alone.
	if err := removeQuarantine(ownerDir, project, false); err != nil {
		return nil, err
	}
	if err := setReloadPending(ownerDir, project); err != nil {
		return nil, err
	}
	detachErr := detach(ctx, ownerDir, *reg)
	if errors.Is(detachErr, ErrOwnerNotRunning) {
		detachErr = nil
	}
	removed, err := unregister(ownerDir, project)
	if err != nil {
		return removed, err
	}
	if err := removeQuarantine(ownerDir, project, true); err != nil {
		return removed, err
	}
	if err := reloadOwner(ctx, ownerDir, project); err != nil {
		return removed, errors.Join(err, detachErr)
	}
	return removed, detachErr
}
