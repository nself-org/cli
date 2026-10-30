package migration

// migrator_backup.go — pre-migration backup and nginx layout migration.
//
// Purpose: bring the compose stack down, create a backup of the project directory and migrate the legacy nginx layout, used by Run in migrator.go, split out for file size.
// Inputs: the project directory and the backup destination.
// Outputs: a BackupManifest describing the created backup, or a migrated nginx layout on disk.
// Constraints: pure move from migrator.go (CLI-R12 Batch E); no behaviour change.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/docker"
)

// composeDown runs `docker compose down` in the given directory.
func composeDown(ctx context.Context, dir string) error {
	return docker.NewProjectCompose(dir).ComposeDown(ctx, dir, docker.DownOptions{})
}

// createBackup copies key project files to backupDir and writes a manifest.json.
func createBackup(projectDir, backupDir string) (*BackupManifest, error) {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating backup directory: %w", err)
	}

	manifest := &BackupManifest{
		Timestamp:  filepath.Base(backupDir),
		ProjectDir: projectDir,
	}

	// Candidates to back up, relative to projectDir.
	candidates := []string{
		"docker-compose.yml",
		".env",
		"nginx",
		".nself/config",
	}
	// Include SSL cert directories if present.
	for _, sslRel := range []string{"ssl", ".nself/ssl"} {
		if _, err := os.Stat(filepath.Join(projectDir, sslRel)); err == nil {
			candidates = append(candidates, sslRel)
		}
	}

	for _, rel := range candidates {
		src := filepath.Join(projectDir, rel)
		info, err := os.Stat(src)
		if err != nil {
			continue // skip absent files/dirs
		}

		destPath := filepath.Join(backupDir, rel)
		if info.IsDir() {
			if err := copyDir(src, destPath); err != nil {
				return nil, fmt.Errorf("backing up %s: %w", rel, err)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return nil, fmt.Errorf("creating backup subdir for %s: %w", rel, err)
			}
			if err := copyFile(src, destPath); err != nil {
				return nil, fmt.Errorf("backing up %s: %w", rel, err)
			}
		}
		manifest.Files = append(manifest.Files, BackupFileEntry{Source: rel, Dest: rel})
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "manifest.json"), data, 0o644); err != nil {
		return nil, fmt.Errorf("writing manifest: %w", err)
	}

	return manifest, nil
}

// migrateNginxLayout moves .conf files from nginx/ to nginx/sites/ (v1 → v2 layout).
// Returns the number of files moved. Safe to call on a v2 project (no-op).
func migrateNginxLayout(projectDir string) (int, error) {
	nginxDir := filepath.Join(projectDir, "nginx")
	info, err := os.Stat(nginxDir)
	if err != nil || !info.IsDir() {
		return 0, nil // no nginx directory — nothing to migrate
	}

	sitesDir := filepath.Join(nginxDir, "sites")
	if _, err := os.Stat(sitesDir); err == nil {
		return 0, nil // already has sites/ — v2 layout, nothing to do
	}

	if err := os.MkdirAll(sitesDir, 0o755); err != nil {
		return 0, fmt.Errorf("creating nginx/sites: %w", err)
	}

	entries, err := os.ReadDir(nginxDir)
	if err != nil {
		return 0, fmt.Errorf("reading nginx directory: %w", err)
	}

	moved := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".conf") || strings.HasSuffix(name, ".conf.template") {
			src := filepath.Join(nginxDir, name)
			dst := filepath.Join(sitesDir, name)
			if err := os.Rename(src, dst); err != nil {
				return moved, fmt.Errorf("moving %s to nginx/sites/: %w", name, err)
			}
			moved++
		}
	}
	return moved, nil
}

// latestBackupDir returns the path to the most recent backup directory.
func latestBackupDir(backupsBase string) (string, error) {
	entries, err := os.ReadDir(backupsBase)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no backups found: .nself/backup/ does not exist")
		}
		return "", fmt.Errorf("reading backup directory: %w", err)
	}

	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) == 0 {
		return "", fmt.Errorf("no backups found in .nself/backup/")
	}

	sort.Strings(dirs)
	return filepath.Join(backupsBase, dirs[len(dirs)-1]), nil
}
