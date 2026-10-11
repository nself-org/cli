package plugin

// Purpose: Plugin install, remove, and update operations with file-lock serialization,
//          dependency resolution, rollback on failure, and license/security pre-checks.
// Inputs:  context.Context, *config.Config, plugin name string, pluginDir path string.
// Outputs: error on failure; side effects are plugin directory and database schema changes.
// Constraints: Acquires {pluginDir}/.install.lock (O_CREATE|O_EXCL) to prevent concurrent
//              installs. Calls installLocked for dependency recursion to avoid deadlock.
// SPORT: install/remove/update operations; callers: cmd/plugin/install.go, cmd/plugin/remove.go

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// dangerousPermissions lists permission strings that warrant a visible warning
// on install. system:exec and network:internet are the two highest-risk grants.
// S71-T02.
var dangerousPermissions = map[string]bool{
	"system:exec":      true,
	"network:internet": true,
}

// installLockPath returns the path to the install lock file for pluginDir.
func installLockPath(pluginDir string) string {
	return filepath.Join(pluginDir, ".install.lock")
}

// acquireInstallLock attempts to create an exclusive lock file at
// {pluginDir}/.install.lock using O_CREATE|O_EXCL (atomic on POSIX). It polls
// every 250 ms until the lock is acquired or the 5-second timeout elapses.
// Returns the open lock file on success so the caller can defer its cleanup.
func acquireInstallLock(pluginDir string) (*os.File, error) {
	// Ensure the plugin directory exists so the lock file can be created.
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating plugin directory: %w", err)
	}

	lockPath := installLockPath(pluginDir)
	deadline := time.Now().Add(5 * time.Second)

	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			// Lock acquired — write our PID for diagnostic purposes.
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			return f, nil
		}
		if !os.IsExist(err) {
			// Unexpected error (permissions, etc.).
			return nil, fmt.Errorf("acquiring install lock: %w", err)
		}
		// Lock file already exists — check timeout before polling again.
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another plugin install is already running")
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// releaseInstallLock closes and removes the lock file returned by
// acquireInstallLock. Errors are silently ignored; a stale lock file will be
// cleaned up on the next acquire attempt.
func releaseInstallLock(f *os.File, pluginDir string) {
	_ = f.Close()
	_ = os.Remove(installLockPath(pluginDir))
}

// Install downloads, extracts, and configures a plugin. For paid plugins it
// checks the license first. Dependencies are resolved and recursively
// installed before the target plugin. If any step after extraction fails, the
// extracted directory and database schema are rolled back.
//
// A file lock on {pluginDir}/.install.lock is held for the duration of the
// operation to prevent concurrent installs from corrupting plugin state.
func Install(ctx context.Context, cfg *config.Config, name string, pluginDir string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	lock, err := acquireInstallLock(pluginDir)
	if err != nil {
		return err
	}
	defer releaseInstallLock(lock, pluginDir)

	return installLocked(ctx, cfg, name, pluginDir)
}
