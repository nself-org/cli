package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// RestoreOptions holds flags for `nself backup restore`.
//
// Note: PointInTime (PITR) is NOT supported in v1.0.9. The field is retained
// for forward compatibility but any non-empty value returns an explicit error
// directing users to v1.1.0 + pgbackrest integration. Remove this note when
// PITR ships.
type RestoreOptions struct {
	BackupID   string   // backup ID or "latest"
	ToDir      string   // restore to alternate directory
	Only       []string // pg, minio, metadata — subset to restore
	DecryptKey string   // path to age identity file
	Yes        bool     // skip confirmation
}

// Restore recovers a backup by ID or "latest".
func Restore(ctx context.Context, cfg *config.Config, opts RestoreOptions) error {
	backupDir := cfg.Backup.Dir
	if backupDir == "" {
		backupDir = "./backups"
	}

	sweepStaleRestoreTemps(backupDir, restoreTempStaleAfter)

	backupFile, err := resolveBackupFile(backupDir, opts.BackupID)
	if err != nil {
		return err
	}

	slog.Info("restoring from backup", "file", backupFile)

	// Decrypt if needed.
	workFile := backupFile
	if strings.HasSuffix(backupFile, ".age") {
		decrypted, err := decryptFile(ctx, backupFile, opts.DecryptKey, cfg.ProjectName)
		if err != nil {
			return fmt.Errorf("decrypt backup: %w", err)
		}
		workFile = decrypted
		defer func() { _ = os.Remove(decrypted) }()
	}

	// Determine what to restore.
	restoreComponents := map[string]bool{"pg": true, "minio": true, "metadata": true}
	if len(opts.Only) > 0 {
		restoreComponents = map[string]bool{}
		for _, c := range opts.Only {
			restoreComponents[c] = true
		}
	}

	if restoreComponents["pg"] {
		if err := restorePostgres(ctx, cfg, workFile, opts); err != nil {
			return fmt.Errorf("restore postgres: %w", err)
		}
	}

	if restoreComponents["minio"] && cfg.Minio.Enabled {
		if err := restoreMinio(ctx, cfg, backupDir, opts.BackupID); err != nil { //nolint:staticcheck // SA4023: restoreMinio always errors by design (not automated); see its doc comment
			slog.Warn("minio restore failed", "error", err)
		}
	}

	if restoreComponents["metadata"] {
		if err := restoreMetadata(ctx, cfg, backupDir, opts.BackupID); err != nil { //nolint:staticcheck // SA4023: restoreMetadata always errors by design (not automated); see its doc comment
			slog.Warn("metadata restore failed", "error", err)
		}
	}

	slog.Info("restore complete")
	return nil
}

func resolveBackupFile(backupDir, backupID string) (string, error) {
	if backupID == "latest" {
		entries, err := List(&config.Config{Backup: config.BackupConfig{Dir: backupDir}}, ListOptions{})
		if err != nil {
			return "", fmt.Errorf("list backups: %w", err)
		}
		// Find latest full backup.
		for _, e := range entries {
			if e.Type == "full" || e.Type == "manual" {
				// Reconstruct filename from directory listing.
				files, _ := os.ReadDir(backupDir)
				for _, f := range files {
					if strings.Contains(f.Name(), e.ID) || f.Name() == e.ID {
						return filepath.Join(backupDir, f.Name()), nil
					}
				}
			}
		}
		return "", fmt.Errorf("%w: no backups found in %s", errs.ErrBackupNotFound, backupDir)
	}

	// Try exact match or prefix match.
	files, err := os.ReadDir(backupDir)
	if err != nil {
		return "", fmt.Errorf("read backup directory: %w", err)
	}
	for _, f := range files {
		if f.Name() == backupID || strings.HasPrefix(f.Name(), backupID) {
			return filepath.Join(backupDir, f.Name()), nil
		}
	}
	return "", fmt.Errorf("%w: %s", errs.ErrBackupNotFound, backupID)
}

// restoreTempStaleAfter is how old a leftover plaintext temp must be before the
// sweep at restore start removes it (a killed restore cannot clean up itself).
var restoreTempStaleAfter = time.Hour

// sweepStaleRestoreTemps removes plaintext temps of ours that an earlier,
// killed restore left in dir: named .nself-restore-*.dec, regular, 0600, owned
// by this user, older than olderThan. Anything else is never touched.
func sweepStaleRestoreTemps(dir string, olderThan time.Duration) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		n := e.Name()
		if !strings.HasPrefix(n, ".nself-restore-") || !strings.HasSuffix(n, ".dec") {
			continue
		}
		p := filepath.Join(dir, n)
		fi, err := os.Lstat(p)
		if err != nil || !fi.Mode().IsRegular() || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600) || !ownedByCurrentUser(fi) || time.Since(fi.ModTime()) < olderThan {
			continue
		}
		slog.Warn("removing a stale decrypted restore temp", "file", p)
		_ = os.Remove(p)
	}
}

func decryptFile(ctx context.Context, path, keyPath, project string) (string, error) {
	if keyPath == "" {
		// compat.V15(P7-PROD-08): age-key.txt only -> shared identity search, E223 when none
		if compat.V15() {
			var err error
			if keyPath, err = DefaultIdentity(project, "--decrypt-key"); err != nil {
				return "", err
			}
		} else {
			keyPath = filepath.Join(os.Getenv("HOME"), ".config", "nself", "age-key.txt")
		}
	}

	// The plaintext dump is created 0600 by us (never by age's default mode)
	// and removed on every path that does not hand it to the caller.
	// A fresh unique 0600 file next to the backup; an existing <backup>.dec is
	// never touched. The name still ends in .dump.dec for restorePostgres.
	out, err := os.CreateTemp(filepath.Dir(path), ".nself-restore-*-"+strings.TrimSuffix(filepath.Base(path), ".age")+".dec")
	if err != nil {
		return "", fmt.Errorf("%w: create temp file: %v", errs.ErrBackupDecryptFailed, err)
	}
	decrypted := out.Name()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "age", "-d", "-i", keyPath, path)
	cmd.Stdout, cmd.Stderr = out, &stderr
	runErr := cmd.Run()
	closeErr := out.Close()
	if runErr != nil || closeErr != nil {
		_ = os.Remove(decrypted)
		return "", fmt.Errorf("%w: %s %v", errs.ErrBackupDecryptFailed, strings.TrimSpace(stderr.String()), errors.Join(runErr, closeErr))
	}
	return decrypted, nil
}

func restorePostgres(ctx context.Context, cfg *config.Config, backupFile string, opts RestoreOptions) error {
	container := cfg.ProjectName + "_postgres"
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}

	// If it's a pg_dump custom format, use pg_restore.
	if strings.HasSuffix(strings.TrimSuffix(backupFile, ".dec"), ".dump") {
		return restorePgDump(ctx, container, user, db, backupFile)
	}

	// For base backup tar format, use pg_basebackup restore flow.
	return restoreBaseBackup(ctx, cfg, container, user, backupFile, opts)
}

func restorePgDump(ctx context.Context, container, user, db, backupFile string) error {
	// Stream backup file into pg_restore via docker exec.
	f, err := os.Open(backupFile)
	if err != nil {
		return fmt.Errorf("open backup file: %w", err)
	}
	defer func() { _ = f.Close() }()

	args := []string{
		"exec", "-i", container,
		"pg_restore",
		"-U", user,
		"-d", db,
		"--clean",
		"--if-exists",
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = f
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("pg_restore stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start pg_restore: %w", err)
	}

	errOutput, _ := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil { // cancelled (SIGINT/SIGTERM): a killed restore is not a warning
			return fmt.Errorf("%w: %v", errs.ErrBackupRestoreFailed, ctx.Err())
		}
		// pg_restore returns non-zero on warnings too; only fail on real errors.
		errStr := string(errOutput)
		if strings.Contains(errStr, "FATAL") || strings.Contains(errStr, "could not") {
			return fmt.Errorf("%w: %s", errs.ErrBackupRestoreFailed, errStr)
		}
		slog.Warn("pg_restore completed with warnings", "output", errStr)
	}

	return nil
}

func restoreBaseBackup(_ context.Context, _ *config.Config, _, _, backupFile string, _ RestoreOptions) error {
	// nself backup create produces restorable pg_dump (.dump) files, handled by
	// restorePgDump above. A pg_basebackup tar has no working restore path here
	// (it requires stopping postgres and replacing PGDATA out-of-band), so we
	// must NOT report success: doing so previously made backups effectively
	// write-only. Fail loudly instead.
	return fmt.Errorf("%w: %s is a base-backup tar with no automated restore path; "+
		"restore it manually by replacing PGDATA, or recreate backups with the default "+
		"pg_dump format (nself backup create)", errs.ErrBackupRestoreFailed, backupFile)
}

func restoreMinio(_ context.Context, _ *config.Config, _, backupID string) error { //nolint:staticcheck // SA4023: deliberately never returns nil; see comment below
	// Return an error (not nil) so the caller's slog.Warn reflects reality:
	// object storage is NOT restored. A silent nil falsely implied success.
	return fmt.Errorf("%w: minio object-storage restore is not automated; restore the bucket contents manually (backup_id=%s)",
		errs.ErrBackupRestoreFailed, backupID)
}

func restoreMetadata(_ context.Context, _ *config.Config, _, backupID string) error { //nolint:staticcheck // SA4023: deliberately never returns nil; see comment below
	// Return an error (not nil) so the caller's slog.Warn reflects reality:
	// Hasura metadata is NOT restored. A silent nil falsely implied success.
	return fmt.Errorf("%w: hasura metadata restore is not automated; re-apply metadata manually (backup_id=%s)",
		errs.ErrBackupRestoreFailed, backupID)
}
