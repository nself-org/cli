// Package backup provides backup creation, listing, restoration, verification,
// pruning, and WAL archiving for nSelf projects.
package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/metrics"
)

// requireBackupContainerConfig fails loudly, naming exactly what is missing,
// before any backup type derives a docker container name from cfg.
//
// PROJECT_NAME and POSTGRES_DB both get silent placeholder defaults inside
// config.ApplyDefaults ("myproject" / "nself") so cfg.ProjectName and
// cfg.Postgres.DB are never actually empty by the time Create() runs — that
// is precisely the trap: createFullBackup happily builds
// "myproject_postgres" and hands it to `docker exec`, which fails with a
// generic "No such container" error that names the wrong thing (a container
// that was never expected to exist) instead of the actual problem (the two
// vars that decide the real container name were never set). Checking
// os.Getenv directly here — instead of the already-defaulted cfg fields —
// is what lets this tell "the user configured myproject/nself on purpose"
// apart from "nothing was configured and the default silently took over."
//
// This only guards `nself backup create`, which is the surface the reported
// failure came from; `Backup()` in internal/database (used by other DB
// tooling) is a separate lower-level primitive with its own callers.
func requireBackupContainerConfig() error {
	var missing []string
	if strings.TrimSpace(os.Getenv("PROJECT_NAME")) == "" {
		missing = append(missing, "PROJECT_NAME")
	}
	if strings.TrimSpace(os.Getenv("POSTGRES_DB")) == "" {
		missing = append(missing, "POSTGRES_DB")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"backup create: required config not set: %s — set these in .env, or run from inside the project directory (without them the backup would target a placeholder container name that cannot exist)",
		strings.Join(missing, ", "),
	)
}

// BackupType identifies the kind of backup to create.
type BackupType string

const (
	BackupTypeFull     BackupType = "full"
	BackupTypeWAL      BackupType = "wal"
	BackupTypeMetadata BackupType = "metadata"
	BackupTypeMinio    BackupType = "minio"
	BackupTypeAll      BackupType = "all"
)

// CreateOptions holds flags for `nself backup create`.
type CreateOptions struct {
	Type      BackupType // full, wal, metadata, minio, all
	Remote    string     // remote name override
	Encrypt   bool       // force encryption on
	NoEncrypt bool       // force encryption off
	Tag       string     // human label for this backup
	DryRun    bool       // preview only
}

// Create performs a backup of the specified type. It writes the backup to the
// local backup directory and optionally uploads to the configured remote.
func Create(ctx context.Context, cfg *config.Config, opts CreateOptions) error {
	backupDir := cfg.Backup.Dir
	if backupDir == "" {
		backupDir = "./backups"
	}

	if opts.DryRun {
		slog.Info("dry-run: would create backup", "type", opts.Type, "dir", backupDir)
		return nil
	}

	if err := requireBackupContainerConfig(); err != nil {
		return err
	}

	types := []BackupType{opts.Type}
	if opts.Type == BackupTypeAll {
		types = []BackupType{BackupTypeFull, BackupTypeMetadata}
		if cfg.Minio.Enabled {
			types = append(types, BackupTypeMinio)
		}
	}

	for _, bt := range types {
		start := time.Now()
		err := createSingle(ctx, cfg, bt, backupDir, opts)
		emitMetric(cfg, bt, start, backupDir, opts, err == nil)
		if err != nil {
			return fmt.Errorf("backup %s: %w", bt, err)
		}
	}

	return nil
}

// emitMetric writes a prometheus textfile record for the just-completed
// backup run. Metric failures are logged but never fail the backup.
func emitMetric(cfg *config.Config, bt BackupType, start time.Time, backupDir string, opts CreateOptions, success bool) {
	// Best-effort: find the newest file for this type to report size.
	var size int64
	if entries, err := os.ReadDir(backupDir); err == nil {
		var newest os.FileInfo
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.Contains(name, "_"+string(bt)+"_") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if newest == nil || info.ModTime().After(newest.ModTime()) {
				newest = info
			}
		}
		if newest != nil {
			size = newest.Size()
		}
	}

	encrypt := cfg.Backup.Encryption
	if opts.Encrypt {
		encrypt = true
	}
	if opts.NoEncrypt {
		encrypt = false
	}

	rec := metrics.BackupRecord{
		Env:         cfg.Env,
		Type:        string(bt),
		Success:     success,
		DurationSec: time.Since(start).Seconds(),
		Bytes:       size,
		Encrypted:   encrypt && cfg.Backup.AgeRecipients != "",
		Timestamp:   time.Now(),
	}
	if err := metrics.EmitBackup(rec); err != nil {
		slog.Warn("emit backup metric", "error", err)
	}
}

func createSingle(ctx context.Context, cfg *config.Config, bt BackupType, backupDir string, opts CreateOptions) error {
	ts := time.Now().Format("20060102_150405")
	tag := ""
	if opts.Tag != "" {
		tag = "_" + opts.Tag
	}

	switch bt {
	case BackupTypeFull:
		return createFullBackup(ctx, cfg, backupDir, ts, tag, opts)
	case BackupTypeMetadata:
		return createMetadataBackup(ctx, cfg, backupDir, ts, tag)
	case BackupTypeMinio:
		return createMinioBackup(ctx, cfg, backupDir, ts, tag)
	case BackupTypeWAL:
		slog.Info("WAL archiving is continuous via archive_command; triggering checkpoint")
		return triggerWALCheckpoint(ctx, cfg)
	default:
		return fmt.Errorf("unknown backup type: %s", bt)
	}
}
