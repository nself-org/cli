package sharedproxy

// pending.go — remember that the owner's nginx still has to reload after a leave.
//
// Purpose: D-0931. Leave removes the project's registration and files and then
// reloads the owner. If that reload fails, a retry finds no registration, so
// the registration cannot say the reload is owed and the running nginx would
// keep serving the removed project until its next reload. A marker file in
// nginx/shared records the debt: Leave writes it once the registration is
// gone, clears it when the owner reloaded (or is not running, so it reads its
// configuration afresh on start), and ReloadPending/ReloadIfPending pay it.
// Inputs: the owner dir.
// Outputs: a bool, or the error of the reload.
// Constraints: the marker is a regular file inside nginx/shared, written and
// removed through an os.Root on the owner dir; a symlink on the way is an E506
// error. Load ignores it (it reads only `*.json`). Not a registration: it
// never lists files, so Unregister and Quarantine leave it alone.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// ReloadPendingFile is the marker's name inside nginx/shared/.
const ReloadPendingFile = ".reload-pending"

func pendingRel(ownerDir string) (string, error) {
	return ownerRel(ownerDir, filepath.Join(DirOf(ownerDir), ReloadPendingFile))
}

// ReloadPending reports whether a leave still owes the owner a reload.
func ReloadPending(ownerDir string) bool {
	_, err := os.Lstat(filepath.Join(DirOf(ownerDir), ReloadPendingFile))
	return err == nil
}

// markReloadPending writes the marker.
func markReloadPending(ownerDir string) error {
	root, err := os.OpenRoot(ownerDir)
	if err != nil {
		return e506("cannot open the shared proxy owner dir "+ownerDir, err, "")
	}
	defer func() { _ = root.Close() }()
	rel, err := pendingRel(ownerDir)
	if err != nil {
		return e506("cannot record the pending reload", err, "")
	}
	if err := plainDirs(root, filepath.Dir(rel)); err != nil {
		return e506("cannot record the pending reload", err, "")
	}
	f, err := root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return e506("cannot record the pending reload", err, "")
	}
	return f.Close()
}

// clearReloadPending removes the marker; a missing one is not an error.
func clearReloadPending(ownerDir string) {
	root, err := os.OpenRoot(ownerDir)
	if err != nil {
		return
	}
	defer func() { _ = root.Close() }()
	rel, err := pendingRel(ownerDir)
	if err != nil || plainDirs(root, filepath.Dir(rel)) != nil {
		return
	}
	_ = root.Remove(rel)
}

// reloadOwner tests and reloads the owner's nginx. A stopped owner is success:
// it reads the files fresh when it starts.
func reloadOwner(ctx context.Context, ownerDir string) error {
	nginx, nerr := OwnerNginx(ctx, ownerDir)
	switch {
	case errors.Is(nerr, ErrOwnerNotRunning):
		return nil
	case nerr != nil:
		return ownerErr(nerr)
	}
	if _, stderr, terr := nginxTest(ctx, nginx); terr != nil {
		return e506("the owner's nginx -t fails; it was not reloaded", terr, stderr)
	}
	var o Outcome
	return reload(ctx, nginx, &o, func() {})
}

// settleReload runs reloadOwner and clears the marker when it succeeded.
func settleReload(ctx context.Context, ownerDir string) error {
	err := reloadOwner(ctx, ownerDir)
	if err == nil {
		clearReloadPending(ownerDir)
	}
	return err
}

// ReloadIfPending pays a reload a failed leave owed, under the owner lock. It
// is a no-op (false, nil) when none is owed.
func ReloadIfPending(ctx context.Context, ownerDir string) (bool, error) {
	if !ReloadPending(ownerDir) {
		return false, nil
	}
	return locked(ctx, ownerDir, func() (bool, error) {
		if !ReloadPending(ownerDir) { // paid while this call waited for the lock
			return false, nil
		}
		return true, settleReload(ctx, ownerDir)
	})
}
