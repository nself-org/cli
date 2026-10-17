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

import (
	"context"
	"errors"
	"os"
)

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
		return nil, nil
	}
	// Checked first: a refused quarantine path leaves registration and files alone.
	if err := removeQuarantine(ownerDir, project, false); err != nil {
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
	nginx, nerr := OwnerNginx(ctx, ownerDir)
	switch {
	case errors.Is(nerr, ErrOwnerNotRunning):
		// Warning level: the files are gone and a stopped owner serves nothing.
	case nerr != nil:
		// The files are gone but nginx still serves them; say so.
		return removed, errors.Join(ownerErr(nerr), detachErr)
	default:
		if _, stderr, terr := nginxTest(ctx, nginx); terr != nil {
			return removed, e506("the owner's nginx -t fails after removing "+project+"; it was not reloaded", terr, stderr)
		}
		var o Outcome
		if err := reload(ctx, nginx, &o, func() {}); err != nil {
			return removed, err
		}
	}
	return removed, detachErr
}
