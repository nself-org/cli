package database

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// CrossRegionBackupConfig holds the configuration for cross-region backup.
type CrossRegionBackupConfig struct {
	Enabled         bool
	RemotePath      string // rclone remote path (e.g. "s3:mybucket/backups")
	SecondaryRegion string
	Retention       int // days to keep remote backups
	Encrypted       bool
	AgeRecipients   string
}

// CrossRegionBackupStatus describes the last backup sync result.
type CrossRegionBackupStatus struct {
	LastSync         time.Time
	FilesTransferred int
	BytesTransferred int64
	Errors           []string
}

// rcloneFile represents a single entry in rclone's lsjson output.
type rcloneFile struct {
	Path    string `json:"Path"`
	Name    string `json:"Name"`
	Size    int64  `json:"Size"`
	ModTime string `json:"ModTime"`
	IsDir   bool   `json:"IsDir"`
}

// SyncBackupToRemote syncs local backups to the remote location using rclone.
// Creates a local backup first if no recent backup exists (within 24h).
func SyncBackupToRemote(ctx context.Context, cfg *config.Config, xrCfg CrossRegionBackupConfig) error {
	if xrCfg.RemotePath == "" {
		return fmt.Errorf("remote path is required for cross-region backup")
	}

	if _, err := exec.LookPath("rclone"); err != nil {
		return fmt.Errorf("rclone not found in PATH: install rclone to enable cross-region backup: %w", err)
	}

	backupDir := cfg.Backup.Dir
	if backupDir == "" {
		backupDir = "backups"
	}

	// Check whether a recent backup exists (within 24h).
	recentExists, err := hasRecentBackup(backupDir, 24*time.Hour)
	if err != nil {
		return fmt.Errorf("check recent backup: %w", err)
	}
	if !recentExists {
		if err := Backup(ctx, cfg, ""); err != nil {
			return fmt.Errorf("create local backup before sync: %w", err)
		}
	}

	// Optionally encrypt files with age before sync.
	if xrCfg.Encrypted && xrCfg.AgeRecipients != "" {
		if err := encryptBackupDir(ctx, backupDir, xrCfg.AgeRecipients); err != nil {
			return fmt.Errorf("encrypt backups: %w", err)
		}
	}

	// Run rclone copy.
	args := []string{"copy", backupDir, xrCfg.RemotePath, "--progress"}
	cmd := exec.CommandContext(ctx, "rclone", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = os.Stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rclone copy to %s: %s: %w", xrCfg.RemotePath, strings.TrimSpace(stderr.String()), err)
	}

	return nil
}

// GetCrossRegionStatus checks the rclone remote for recent backups.
func GetCrossRegionStatus(ctx context.Context, cfg *config.Config, xrCfg CrossRegionBackupConfig) (CrossRegionBackupStatus, error) {
	if xrCfg.RemotePath == "" {
		return CrossRegionBackupStatus{}, fmt.Errorf("remote path is required")
	}

	if _, err := exec.LookPath("rclone"); err != nil {
		return CrossRegionBackupStatus{}, fmt.Errorf("rclone not found in PATH: %w", err)
	}

	files, err := rcloneLsJSON(ctx, xrCfg.RemotePath)
	if err != nil {
		return CrossRegionBackupStatus{}, fmt.Errorf("list remote backups: %w", err)
	}

	status := CrossRegionBackupStatus{}
	for _, f := range files {
		if f.IsDir {
			continue
		}
		status.FilesTransferred++
		status.BytesTransferred += f.Size

		mod, err := time.Parse(time.RFC3339Nano, f.ModTime)
		if err != nil {
			mod, err = time.Parse("2006-01-02T15:04:05.000000000Z07:00", f.ModTime)
			if err != nil {
				continue
			}
		}
		if mod.After(status.LastSync) {
			status.LastSync = mod
		}
	}

	return status, nil
}

// PruneRemoteBackups removes remote backups older than RetentionDays.
func PruneRemoteBackups(ctx context.Context, cfg *config.Config, xrCfg CrossRegionBackupConfig) (pruned int, err error) {
	if xrCfg.RemotePath == "" {
		return 0, fmt.Errorf("remote path is required")
	}

	if _, err := exec.LookPath("rclone"); err != nil {
		return 0, fmt.Errorf("rclone not found in PATH: %w", err)
	}

	retention := xrCfg.Retention
	if retention <= 0 {
		retention = 7
	}
	cutoff := time.Now().AddDate(0, 0, -retention)

	files, err := rcloneLsJSON(ctx, xrCfg.RemotePath)
	if err != nil {
		return 0, fmt.Errorf("list remote backups for pruning: %w", err)
	}

	for _, f := range files {
		if f.IsDir {
			continue
		}
		mod, parseErr := time.Parse(time.RFC3339Nano, f.ModTime)
		if parseErr != nil {
			mod, parseErr = time.Parse("2006-01-02T15:04:05.000000000Z07:00", f.ModTime)
			if parseErr != nil {
				continue
			}
		}
		if mod.Before(cutoff) {
			remotePath := xrCfg.RemotePath + "/" + f.Path
			delCmd := exec.CommandContext(ctx, "rclone", "deletefile", remotePath)
			var delStderr bytes.Buffer
			delCmd.Stderr = &delStderr
			if delErr := delCmd.Run(); delErr != nil {
				return pruned, fmt.Errorf("delete remote file %s: %s: %w", remotePath, strings.TrimSpace(delStderr.String()), delErr)
			}
			pruned++
		}
	}

	return pruned, nil
}

// GenerateCrossRegionCronScript generates a shell script suitable for use in
// a Docker cron container for daily cross-region backups.
func GenerateCrossRegionCronScript(cfg *config.Config, xrCfg CrossRegionBackupConfig) string {
	backupDir := cfg.Backup.Dir
	if backupDir == "" {
		backupDir = "backups"
	}
	remotePath := xrCfg.RemotePath
	retention := xrCfg.Retention
	if retention <= 0 {
		retention = 7
	}

	encryptBlock := ""
	if xrCfg.Encrypted && xrCfg.AgeRecipients != "" {
		encryptBlock = fmt.Sprintf(`
# Encrypt new backup files with age
for f in "$BACKUP_DIR"/*.dump; do
  if [ -f "$f" ] && [ ! -f "${f}.age" ]; then
    age --recipient %s -o "${f}.age" "$f"
  fi
done
`, xrCfg.AgeRecipients)
	}

	return fmt.Sprintf(`#!/bin/sh
# Cross-region backup — generated by nself
set -e
BACKUP_DIR=%s
REMOTE_PATH=%s
RETENTION_DAYS=%d
%s
# Sync to remote
rclone copy "$BACKUP_DIR" "$REMOTE_PATH" --progress

# Prune old remote backups
rclone delete "$REMOTE_PATH" --min-age "${RETENTION_DAYS}d"
`, backupDir, remotePath, retention, encryptBlock)
}

// hasRecentBackup reports whether any file in backupDir is newer than maxAge.
func hasRecentBackup(backupDir string, maxAge time.Duration) (bool, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP: an absent backup dir holds no recent backup — that is the
			// fact, not a stand-in. ENOENT only, so "could not check" never
			// reaches an operator as "your backups are stale".
			return false, nil
		}
		return false, fmt.Errorf("read backup directory %s: %w", backupDir, err)
	}
	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			return true, nil
		}
	}
	return false, nil
}

// encryptBackupDir encrypts all unencrypted .dump files in backupDir using age.
func encryptBackupDir(ctx context.Context, backupDir string, recipients string) error {
	if _, err := exec.LookPath("age"); err != nil {
		return fmt.Errorf("age not found in PATH: install age to enable encryption: %w", err)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("read backup directory %s: %w", backupDir, err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".dump") {
			continue
		}
		src := filepath.Join(backupDir, name)
		dst := src + ".age"

		// Skip if already encrypted.
		if _, err := os.Stat(dst); err == nil {
			continue
		}

		args := []string{"--recipient", recipients, "-o", dst, src}
		cmd := exec.CommandContext(ctx, "age", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("age encrypt %s: %s: %w", src, strings.TrimSpace(stderr.String()), err)
		}
	}

	return nil
}

// rcloneLsJSON runs rclone lsjson and returns the parsed file list.
func rcloneLsJSON(ctx context.Context, remotePath string) ([]rcloneFile, error) {
	cmd := exec.CommandContext(ctx, "rclone", "lsjson", remotePath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("rclone lsjson %s: %s: %w", remotePath, strings.TrimSpace(stderr.String()), err)
	}

	var files []rcloneFile
	if err := json.Unmarshal(stdout.Bytes(), &files); err != nil {
		return nil, fmt.Errorf("parse rclone lsjson output: %w", err)
	}
	return files, nil
}
