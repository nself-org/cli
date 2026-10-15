package acme

import (
	"fmt"
	"os"
	"path/filepath"
)

// InstallAccess selects access for the proxy's certificate readers.
// Purpose: apply caller-selected modes and ownership before switching or reload.
// Inputs: directory modes and uid/gid; -1 leaves an owner component unchanged.
// Outputs: new and retained generations have the same access, including a
// converted legacy directory, so rollback remains usable by the same proxy.
// Constraints: file modes stay private; nil preserves the install defaults.
type InstallAccess struct {
	DirMode, ParentMode os.FileMode
	UID, GID            int
}

// apply configures generations before they become visible to the reloader.
func (a *InstallAccess) apply(parent, base string) error {
	if a == nil {
		return nil
	}
	info, err := os.Stat(parent)
	if err != nil {
		return err
	}
	// A root timer may own the parent; an interactive renewal need not own
	// it when its permissions already match the selected policy.
	if info.Mode().Perm() != a.ParentMode.Perm() {
		if err := os.Chmod(parent, a.ParentMode.Perm()); err != nil {
			return fmt.Errorf("setting certificate parent permissions: %w", err)
		}
	}
	for _, n := range generations(parent, base) {
		dir := filepath.Join(parent, genName(base, n))
		if err := a.generation(dir); err != nil {
			return fmt.Errorf("setting access for %s: %w", dir, err)
		}
	}
	return nil
}

// generation changes file ownership before handing over its directory.
func (a *InstallAccess) generation(dir string) error {
	if a.UID >= 0 || a.GID >= 0 {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := os.Lchown(filepath.Join(dir, entry.Name()), a.UID, a.GID); err != nil {
				return err
			}
		}
	}
	if err := os.Chmod(dir, a.DirMode.Perm()); err != nil {
		return err
	}
	if a.UID >= 0 || a.GID >= 0 {
		return os.Lchown(dir, a.UID, a.GID)
	}
	return nil
}
