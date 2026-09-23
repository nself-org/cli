package plugin

// installer_remove_update.go — plugin removal and update operations.
//
// Purpose: remove or update an installed plugin, including reverse-dependency checks and dangerous-permission logging, split out of installer.go for file size.
// Inputs: the plugin name, target config and pluginDir.
// Outputs: the plugin removed or updated on disk, or an error if reverse dependencies block removal.
// Constraints: pure move from installer.go (CLI-R12 Batch F); no behaviour change.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// logDangerousPermissions emits a stderr warning for any dangerous permissions
// held by the named plugin. Called immediately after schema creation so the
// warning appears before the "Run nself build" footer line. S71-T02.
func logDangerousPermissions(pluginName string, permissions []string) {
	for _, perm := range permissions {
		if dangerousPermissions[perm] {
			fmt.Fprintf(os.Stderr,
				"warning: plugin %q holds elevated permission %q — review with 'nself plugin info %s'\n",
				pluginName, perm, pluginName,
			)
		}
	}
}

// checkReverseDependencies scans all installed plugins in pluginDir and returns
// the names of any that declare name as a dependency. This prevents silently
// breaking dependent plugins during uninstall.
func checkReverseDependencies(pluginDir, name string) ([]string, error) {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading plugin directory: %w", err)
	}

	var dependents []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == name {
			continue
		}
		manifestPath := filepath.Join(pluginDir, entry.Name(), "plugin.json")
		m, err := parseManifest(manifestPath)
		if err != nil {
			continue // skip directories without valid manifests
		}
		for _, dep := range m.Dependencies {
			if strings.EqualFold(dep, name) {
				dependents = append(dependents, m.Name)
				break
			}
		}
	}
	return dependents, nil
}

// Remove stops a plugin (if running), optionally drops its database schema,
// and removes its directory from disk. When force is false and other installed
// plugins depend on the target, Remove returns an error listing them.
//
// A file lock on {pluginDir}/.install.lock is held for the duration of the
// operation so that concurrent install and remove calls serialize correctly.
func Remove(ctx context.Context, cfg *config.Config, name string, pluginDir string, keepData bool, force bool) error {
	lock, err := acquireInstallLock(pluginDir)
	if err != nil {
		return err
	}
	defer releaseInstallLock(lock, pluginDir)

	destDir := filepath.Join(pluginDir, name)
	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		return fmt.Errorf("plugin %q is not installed", name)
	}

	// Check for reverse dependencies before doing anything destructive.
	if !force {
		dependents, err := checkReverseDependencies(pluginDir, name)
		if err != nil {
			return fmt.Errorf("checking reverse dependencies: %w", err)
		}
		if len(dependents) > 0 {
			return fmt.Errorf("plugin %s depends on %q. Use --force to remove anyway",
				strings.Join(dependents, ", "), name)
		}
	}

	// Stop if running.
	st, err := Status(name)
	if err == nil && st.State == "running" {
		if stopErr := Stop(ctx, name); stopErr != nil {
			return fmt.Errorf("stopping plugin %q: %w", name, stopErr)
		}
	}

	// The manifest has to be read before the directory goes, because it names
	// the command binaries to unpublish and says whether there is a schema.
	manifest := readPluginManifest(destDir)

	// Drop schema unless the caller wants to keep data — and only when the
	// plugin owns tables. Dropping reaches for Postgres through Docker, so
	// without this a command-line plugin could be installed on a machine with
	// no stack running and then never removed from it.
	if !keepData && pluginOwnsTables(manifest) {
		if err := dropPluginSchema(ctx, cfg, name); err != nil {
			return fmt.Errorf("dropping schema for plugin %q: %w", name, err)
		}
	}

	// Unpublish the command binary. unlinkCLIBinary has existed since the
	// install side was written and was never called from anywhere, so removing
	// a plugin left its command working: `nself remove foo` then `nself foo`
	// still ran the removed plugin.
	if err := unlinkCLIBinary(name, manifest); err != nil {
		return fmt.Errorf("unpublishing plugin %q command: %w", name, err)
	}

	// Remove plugin directory.
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("removing plugin directory %q: %w", destDir, err)
	}

	fmt.Fprintf(os.Stderr, "\nℹ Run 'nself build' to update your stack.\n")
	return nil
}

// Update backs up the current installation, then reinstalls from the registry.
// If the new install fails, the previous version is restored automatically.
func Update(ctx context.Context, cfg *config.Config, name string, pluginDir string) error {
	currentDir := filepath.Join(pluginDir, name)
	backupDir := filepath.Join(pluginDir, name+".prev")

	// Rename current install to .prev so we can restore on failure.
	if _, err := os.Stat(currentDir); err == nil {
		// Remove any stale backup from a prior interrupted update.
		_ = os.RemoveAll(backupDir)
		if err := os.Rename(currentDir, backupDir); err != nil {
			return fmt.Errorf("backing up plugin %q for update: %w", name, err)
		}
	}

	// Install the new version.
	if err := Install(ctx, cfg, name, pluginDir); err != nil {
		// Restore the previous version from backup.
		if _, statErr := os.Stat(backupDir); statErr == nil {
			_ = os.RemoveAll(currentDir) // clean partial install if any
			if renameErr := os.Rename(backupDir, currentDir); renameErr != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to restore previous version of %q: %v\n", name, renameErr)
			}
		}
		return fmt.Errorf("installing updated plugin %q: %w", name, err)
	}

	// Success: remove the backup.
	_ = os.RemoveAll(backupDir)

	// Drop the images built from the previous source; otherwise the next
	// start reuses them and the update never runs (see image_invalidate.go).
	removed, err := invalidatePluginImages(ctx, cfg, currentDir)
	for _, ref := range removed {
		fmt.Fprintf(os.Stderr, "ℹ Removed cached image %s; the next 'nself start' rebuilds it.\n", ref)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not clear cached images for %q (%v); the next start may keep running the previous build\n", name, err)
	}
	return nil
}

// checkSigBypassAllowed evaluates whether the NSELF_LICENSE_SKIP_VERIFY /
// NSELF_LICENSE_SKIP_VERIFY_FORCE bypass is allowed for the given env and
// plugin name.
//
// Returns (true, nil) when both vars are set and env is not prod/staging.
// Returns (false, err) with a SECURITY prefix when env is prod or staging.
// Returns (false, err) when only one var is set (standalone bypass rejected).
// Returns (false, nil) when neither var is set (normal verification path).
//
// Purpose: Extracted so unit tests can exercise the bypass-gate logic without
//
//	spinning up a full HTTP registry server.
//
// SPORT: security-audit; ticket P2-E2-W2-S3-T10
func checkSigBypassAllowed(env, pluginName string) (bypassed bool, err error) {
	skipVerify := os.Getenv("NSELF_LICENSE_SKIP_VERIFY") == "1"
	forceSkip := os.Getenv("NSELF_LICENSE_SKIP_VERIFY_FORCE") == "1"

	if !skipVerify && !forceSkip {
		// Normal path — no bypass requested.
		return false, nil
	}

	// Any bypass var set in prod/staging is a fatal security violation.
	if env == "prod" || env == "staging" {
		return false, fmt.Errorf(
			"SECURITY: NSELF_LICENSE_SKIP_VERIFY and NSELF_LICENSE_SKIP_VERIFY_FORCE are not permitted in %s — remove these env vars",
			env,
		)
	}

	// In dev, both vars must be set together.
	if !skipVerify || !forceSkip {
		return false, fmt.Errorf(
			"NSELF_LICENSE_SKIP_VERIFY requires NSELF_LICENSE_SKIP_VERIFY_FORCE=1; standalone skip is not permitted",
		)
	}

	return true, nil
}
