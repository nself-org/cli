package backup

// Purpose: per-target backup implementations (full pg_dump, Hasura metadata, MinIO, WAL checkpoint) plus encryption and remote-upload helpers used by create.go's createSingle dispatcher.
// Inputs: a *config.Config, backup directory, timestamp, and CreateOptions.
// Outputs: backup artifact files on disk, optionally encrypted and/or uploaded to a remote target.
// Constraints: split out of create.go as a pure move (CLI-R12); no behavior change.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/security"
)

// createFullBackup runs pg_dump (custom format) inside the postgres container.
//
// The custom (.dump) format is used instead of a pg_basebackup tar because it
// is restorable end-to-end by the restore path (pg_restore) and validated by
// `nself backup verify --restore-test`. A pg_basebackup tar has no working
// restore path in this tool, so producing it would yield write-only backups.
func createFullBackup(ctx context.Context, cfg *config.Config, backupDir, ts, tag string, opts CreateOptions) error {
	container := cfg.ProjectName + "_postgres"
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}

	filename := fmt.Sprintf("%s_full_%s%s.dump", cfg.ProjectName, ts, tag)
	outputPath := filepath.Join(backupDir, filename)

	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}

	args := []string{
		"exec", container,
		"pg_dump",
		"-U", user,
		"-d", db,
		"-Fc", // custom format, restorable via pg_restore
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pg_dump stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("pg_dump stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start pg_dump: %w", err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("create backup file %s: %w", outputPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, stdout); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("stream pg_dump output: %w", err)
	}

	errOutput, _ := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		_ = os.Remove(outputPath)
		if len(errOutput) > 0 {
			return fmt.Errorf("%w: %s", errs.ErrBackupFailed, string(errOutput))
		}
		return fmt.Errorf("%w: %v", errs.ErrBackupFailed, err)
	}

	if err := security.EnforceFilePermissions(outputPath, 0600); err != nil {
		return fmt.Errorf("enforce permissions on %s: %w", outputPath, err)
	}

	// Encrypt if configured.
	encrypt := cfg.Backup.Encryption
	if opts.Encrypt {
		encrypt = true
	}
	if opts.NoEncrypt {
		encrypt = false
	}
	// Asking for encryption and getting plaintext is worse than not asking.
	// This was `encrypt && AgeRecipients != ""`, so a caller who passed
	// --encrypt (or set NSELF_BACKUP_ENCRYPTION) with no recipient configured
	// silently got an unencrypted backup: the condition was false and nothing
	// reported it. Refuse instead.
	if encrypt && cfg.Backup.AgeRecipients == "" {
		return fmt.Errorf(
			"encryption requested but no recipient configured: set NSELF_BACKUP_AGE_RECIPIENTS, or pass --no-encrypt to write the backup in the clear")
	}
	if encrypt {
		if err := encryptFile(outputPath, cfg.Backup.AgeRecipients); err != nil {
			return fmt.Errorf("encrypt backup: %w", err)
		}
	}

	// Upload to remote if configured.
	remote := cfg.Backup.Remote
	if opts.Remote != "" {
		remote = opts.Remote
	}
	if remote != "" {
		if err := requireCompleteS3Credentials(cfg); err != nil {
			// Half-configured S3 creds (one of access/secret set, not both)
			// is worse than none: it looks configured, uploads nothing or
			// fails opaquely inside rclone, and nobody notices until a
			// restore is needed. Refuse loudly instead of attempting the
			// upload with partial credentials.
			slog.Error("remote upload skipped: incomplete S3 credentials", "error", err, "path", outputPath)
		} else if err := uploadToRemote(ctx, outputPath, remote, cfg); err != nil {
			slog.Error("remote upload failed", "error", err, "path", outputPath)
			// Non-fatal: local backup succeeded.
		}
	}

	slog.Info("full backup created", "path", outputPath)
	return nil
}

// createMetadataBackup dumps Hasura metadata and env config.
func createMetadataBackup(ctx context.Context, cfg *config.Config, backupDir, ts, tag string) error {
	filename := fmt.Sprintf("%s_metadata_%s%s.tar.gz", cfg.ProjectName, ts, tag)
	outputPath := filepath.Join(backupDir, filename)

	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}

	// Export Hasura metadata.  The admin secret is passed through the child
	// process environment, not as an argv element, to prevent CWE-214 process
	// table exposure.
	cmd := hasuraMetadataExportCmd(ctx, cfg)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Do NOT persist output on failure: hasura-cli error messages may echo
		// connection details or contain fragments of the admin secret.
		slog.Warn("hasura metadata export failed, skipping metadata component", "error", err)
		return nil
	}

	if err := os.WriteFile(outputPath, output, 0600); err != nil {
		return fmt.Errorf("write metadata backup: %w", err)
	}

	slog.Info("metadata backup created", "path", outputPath)
	return nil
}

// hasuraMetadataExportCmd constructs the docker exec command that runs
// hasura-cli metadata export inside the project's Hasura container.
//
// The admin secret is injected into the child process environment via cmd.Env
// (not visible in the host process table) and forwarded to the container using
// a bare "-e HASURA_GRAPHQL_ADMIN_SECRET" flag (no "=value"; docker reads the
// value from the client process environment).  hasura-cli reads this variable
// natively, so no --admin-secret argv element is needed.
func hasuraMetadataExportCmd(ctx context.Context, cfg *config.Config) *exec.Cmd {
	container := cfg.ProjectName + "_hasura"
	cmd := exec.CommandContext(ctx, "docker",
		"exec",
		"-e", "HASURA_GRAPHQL_ADMIN_SECRET",
		container,
		"hasura-cli", "metadata", "export",
	)
	cmd.Env = append(os.Environ(), "HASURA_GRAPHQL_ADMIN_SECRET="+cfg.Hasura.AdminSecret)
	return cmd
}

// createMinioBackup uses mc mirror to back up MinIO buckets.
func createMinioBackup(ctx context.Context, cfg *config.Config, backupDir, ts, tag string) error {
	if !cfg.Minio.Enabled {
		slog.Info("MinIO not enabled, skipping MinIO backup")
		return nil
	}

	destDir := filepath.Join(backupDir, fmt.Sprintf("minio_%s%s", ts, tag))
	if err := os.MkdirAll(destDir, 0700); err != nil {
		return fmt.Errorf("create minio backup directory: %w", err)
	}

	// Use mc mirror to copy all buckets locally.
	alias := cfg.ProjectName + "-minio"
	args := []string{"mirror", "--overwrite", alias + "/", destDir}
	cmd := exec.CommandContext(ctx, "mc", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mc mirror: %w", err)
	}

	slog.Info("minio backup created", "path", destDir)
	return nil
}

// triggerWALCheckpoint forces a WAL checkpoint to flush pending WAL data.
func triggerWALCheckpoint(ctx context.Context, cfg *config.Config) error {
	container := cfg.ProjectName + "_postgres"
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}

	args := []string{
		"exec", container,
		"psql", "-U", user, "-d", cfg.Postgres.DB,
		"-c", "CHECKPOINT;",
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("WAL checkpoint: %s: %w", string(output), err)
	}

	slog.Info("WAL checkpoint triggered")
	return nil
}
