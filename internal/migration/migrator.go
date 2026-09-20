package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/ui"
)

// BackupManifest records what was backed up during a migration.
type BackupManifest struct {
	Timestamp  string            `json:"timestamp"`
	ProjectDir string            `json:"project_dir"`
	Files      []BackupFileEntry `json:"files"`
}

// BackupFileEntry describes a single backed-up file or directory.
type BackupFileEntry struct {
	// Source is the path relative to the project directory.
	Source string `json:"source"`
	// Dest is the path relative to the backup directory.
	Dest string `json:"dest"`
}

// BackupInfo describes an available backup for listing.
type BackupInfo struct {
	Timestamp string
	Dir       string
	// Size is the total size of all files in the backup directory, in bytes.
	Size int64
}

// Run performs a full v1 → v2 migration.
// It is idempotent: running on an already-migrated project prints a message and exits cleanly.
func Run(ctx context.Context, projectDir string) error {
	// 1. Detect v1 artifacts — idempotency check
	artifacts := Detect(projectDir)
	if len(artifacts) == 0 {
		ui.Success("Already on v2 — nothing to migrate")
		return nil
	}

	_, _ = fmt.Fprintln(os.Stdout, "\n"+ui.C(ui.Yellow, ui.IconWarning)+" "+ui.C(ui.Bold, fmt.Sprintf("Migrating %d v1 artifact(s) to v2...", len(artifacts)))+"\n")

	// 2. Stop all running containers gracefully
	ui.Section("Stopping containers")
	if err := composeDown(ctx, projectDir); err != nil {
		ui.Warn(fmt.Sprintf("docker compose down: %v (containers may not be running)", err))
	} else {
		ui.Success("Containers stopped")
	}

	// 3. Backup current state
	timestamp := time.Now().Format("20060102-150405")
	backupDir := filepath.Join(projectDir, ".nself", "backup", timestamp)
	ui.Section(fmt.Sprintf("Creating backup at .nself/backup/%s", timestamp))
	manifest, err := createBackup(projectDir, backupDir)
	if err != nil {
		return fmt.Errorf("creating backup: %w", err)
	}
	ui.Success(fmt.Sprintf("Backed up %d file(s)", len(manifest.Files)))

	// 4. Move v1 nginx configs to v2 layout (nginx/ → nginx/sites/)
	ui.Section("Migrating nginx layout")
	moved, err := migrateNginxLayout(projectDir)
	if err != nil {
		return fmt.Errorf("migrating nginx layout (run `nself migrate rollback` to restore): %w", err)
	}
	if moved > 0 {
		ui.Success(fmt.Sprintf("Moved %d nginx config(s) to nginx/sites/", moved))
	} else {
		ui.Info("Nginx layout already compatible")
	}

	// 5. Regenerate compose + nginx + SSL via build
	ui.Section("Regenerating v2 configuration")
	if _, err = build.Build(projectDir, build.BuildOptions{Force: true}); err != nil {
		return fmt.Errorf("regenerating v2 config (run `nself migrate rollback` to restore): %w", err)
	}
	ui.Success("v2 configuration generated")

	// 6. Print plugin re-install warning (S60-T05)
	// v0.9 plugin code is incompatible with v1 signed bundles — hard break.
	// Parse the backed-up .env for PLUGIN_* entries and surface the re-install chain.
	pluginWarning(projectDir, backupDir)

	// 7. Print summary
	printRunSummary(manifest, timestamp)
	return nil
}

// Rollback restores a project from a backup created by Run.
// If backupTimestamp is empty, the most recent backup is used.
func Rollback(ctx context.Context, projectDir, backupTimestamp string) error {
	backupsBase := filepath.Join(projectDir, ".nself", "backup")

	var backupDir string
	if backupTimestamp != "" {
		backupDir = filepath.Join(backupsBase, backupTimestamp)
		if _, err := os.Stat(backupDir); err != nil {
			return fmt.Errorf("backup %q not found", backupTimestamp)
		}
	} else {
		latest, err := latestBackupDir(backupsBase)
		if err != nil {
			return err
		}
		backupDir = latest
	}

	// Load and validate manifest
	manifestPath := filepath.Join(backupDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading backup manifest: %w", err)
	}
	var manifest BackupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parsing backup manifest (file may be corrupt): %w", err)
	}

	ui.Section(fmt.Sprintf("Restoring from backup %s", filepath.Base(backupDir)))

	// Stop containers before restoring
	ui.Info("Stopping containers...")
	if err := composeDown(ctx, projectDir); err != nil {
		ui.Warn(fmt.Sprintf("docker compose down: %v (continuing)", err))
	}

	// Restore each file recorded in the manifest
	for _, entry := range manifest.Files {
		src := filepath.Join(backupDir, entry.Dest)
		dst := filepath.Join(projectDir, entry.Source)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("preparing restore path for %s: %w", entry.Source, err)
		}
		if err := copyPath(src, dst); err != nil {
			return fmt.Errorf("restoring %s: %w", entry.Source, err)
		}
	}

	ui.Success(fmt.Sprintf("Restored %d file(s) from backup %s", len(manifest.Files), filepath.Base(backupDir)))
	_, _ = fmt.Fprintln(os.Stdout)
	ui.Info("Rollback complete. Run `docker compose up -d` to start your v1 stack.")
	return nil
}

// ListBackups returns all available backups sorted newest-first.
// Returns an empty slice (not an error) when no backups exist.
func ListBackups(projectDir string) ([]BackupInfo, error) {
	backupsBase := filepath.Join(projectDir, ".nself", "backup")
	entries, err := os.ReadDir(backupsBase)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP. Nothing has been backed up here yet, so there is nothing
			// to list or roll back to. ENOENT only: this list is what a
			// rollback picks from, so an unreadable backup dir must say so
			// rather than report "no backups available" to someone trying to
			// undo a migration.
			return nil, nil
		}
		return nil, fmt.Errorf("reading backup directory: %w", err)
	}

	var backups []BackupInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(backupsBase, e.Name())
		backups = append(backups, BackupInfo{
			Timestamp: e.Name(),
			Dir:       dir,
			Size:      dirSize(dir),
		})
	}

	// Timestamp format 20060102-150405 sorts lexicographically; newest-first = descending.
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp > backups[j].Timestamp
	})
	return backups, nil
}
