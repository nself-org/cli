package stack

// Purpose: create and bind an explicit run-owned backup to its recovery journal.
// Inputs: run identity and configured backup directory. Outputs: verified evidence.
// Constraints: never select latest, never prune, and preserve encryption policy.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/reconcile"
)

var runtimeRunID = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// Adapter seams use the same backup implementation in production and isolated tests.
var runtimeBackupCreate = backup.Create
var runtimeBackupVerify = backup.Verify
var runtimeBackupCountRows = backup.CountRows

// Backup snapshots immutable old images and creates a newly named full dump.
func (r *Runtime) Backup(ctx context.Context, j *Journal) error {
	if !runtimeRunID.MatchString(j.RunID) {
		return fmt.Errorf("invalid stack run identity")
	}
	if j.BackupID != "" {
		return r.VerifyBackup(ctx, j)
	}
	if err := r.snapshot(ctx, j); err != nil {
		return err
	}
	if err := r.createRunBackup(ctx, j); err != nil {
		return err
	}
	rows, err := r.countRows(ctx)
	if err != nil {
		return fmt.Errorf("count pre-update rows: %w", err)
	}
	j.BeforeRows = rows
	return nil
}

// createRunBackup binds the uniquely tagged new artifact and its digest.
// Inputs: run journal. Outputs: backup identity. Constraints: retains prior attempts.
func (r *Runtime) createRunBackup(ctx context.Context, j *Journal) error {
	before, err := r.backupNames()
	if err != nil {
		return err
	}
	// An interrupted backup has no committed identity; a retry retains it and
	// creates a distinct artifact, even within the same timestamp second.
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fmt.Errorf("draw backup attempt identity: %w", err)
	}
	tag := "stack-" + j.RunID + "-" + hex.EncodeToString(nonce[:])
	if err := runtimeBackupCreate(ctx, r.cfg, backup.CreateOptions{Type: backup.BackupTypeFull, Tag: tag}); err != nil {
		return fmt.Errorf("create stack backup: %w", err)
	}
	after, err := r.backupNames()
	if err != nil {
		return err
	}
	if err := r.identifyRunBackup(j, before, after, tag); err != nil {
		return err
	}
	digest, err := r.backupDigest(j.BackupID)
	if err != nil {
		return err
	}
	j.BackupSHA256 = digest
	return nil
}

// identifyRunBackup selects only the new artifact belonging to this attempt.
// Inputs: inventories and tag. Outputs: journal proof. Constraints: no latest lookup.
func (r *Runtime) identifyRunBackup(j *Journal, before, after map[string]bool, tag string) error {
	for name := range after {
		if before[name] || !strings.Contains(name, "_"+tag+".dump") {
			continue
		}
		if j.BackupID != "" {
			return fmt.Errorf("multiple backups created for stack run")
		}
		info, err := os.Stat(filepath.Join(r.cfg.Backup.Dir, name))
		if err != nil {
			return fmt.Errorf("stat created backup: %w", err)
		}
		if info.ModTime().Before(j.StartedAt) {
			return fmt.Errorf("created backup predates update run")
		}
		j.BackupID = name
		j.BackupCreatedAt = time.Now().UTC()
	}
	if j.BackupID == "" {
		return fmt.Errorf("new run-owned backup was not found")
	}
	return nil
}

// backupNames lists only existing artifacts, retaining all backups unchanged.
func (r *Runtime) backupNames() (map[string]bool, error) {
	entries, err := os.ReadDir(r.cfg.Backup.Dir)
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list backup artifacts: %w", err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	return names, nil
}

// backupDigest refuses path traversal and symlinks and hashes the exact artifact.
func (r *Runtime) backupDigest(id string) (string, error) {
	if id == "" || id == "latest" || filepath.Base(id) != id {
		return "", fmt.Errorf("invalid explicit stack backup identity")
	}
	p := filepath.Join(r.cfg.Backup.Dir, id)
	info, err := os.Lstat(p)
	if err != nil {
		return "", fmt.Errorf("stat backup: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("stack backup is not a regular file")
	}
	f, err := os.Open(p)
	if err != nil {
		return "", fmt.Errorf("open backup: %w", err)
	}
	if strings.HasSuffix(id, ".dump") {
		if err := validateBackupHeader(f); err != nil {
			closeErr := f.Close()
			if closeErr != nil {
				return "", fmt.Errorf("close invalid backup: %w", closeErr)
			}
			return "", err
		}
	}
	h := sha256.New()
	_, copyErr := io.Copy(h, f)
	closeErr := f.Close()
	if copyErr != nil {
		return "", fmt.Errorf("hash backup: %w", copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close backup: %w", closeErr)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// validateBackupHeader proves custom dump format and rewinds for hashing.
// Inputs: open artifact. Outputs: validation error. Constraints: no file mutation.
func validateBackupHeader(f *os.File) error {
	magic := make([]byte, 5)
	if _, err := io.ReadFull(f, magic); err != nil {
		return fmt.Errorf("read backup archive header: %w", err)
	}
	if string(magic) != "PGDMP" {
		return fmt.Errorf("stack backup is not a PostgreSQL custom-format archive")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind backup archive: %w", err)
	}
	return nil
}

// VerifyBackup proves identity, freshness and unchanged bytes using backup.Verify.
func (r *Runtime) VerifyBackup(ctx context.Context, j *Journal) error {
	if j.PreviousCompose != "" {
		if err := r.verifySnapshot(j); err != nil {
			return err
		}
	}
	if j.BackupCreatedAt.Before(j.StartedAt) || j.BackupSHA256 == "" {
		return fmt.Errorf("backup is not evidenced in this run")
	}
	digest, err := r.backupDigest(j.BackupID)
	if err != nil {
		return err
	}
	if digest != j.BackupSHA256 {
		return fmt.Errorf("stack backup digest changed")
	}
	result, err := runtimeBackupVerify(ctx, r.cfg, backup.VerifyOptions{BackupID: j.BackupID})
	if err != nil {
		return fmt.Errorf("verify stack backup: %w", err)
	}
	if result == nil || !result.Verified {
		return fmt.Errorf("stack backup was not verified")
	}
	j.BackupVerifiedAt = time.Now().UTC()
	return nil
}

// Restore refuses production independently of caller policy and restores only pg.
func (r *Runtime) Restore(ctx context.Context, j *Journal) error {
	if j.Prod || reconcile.EnvClass(r.cfg.Env) == reconcile.ClassProd {
		return fmt.Errorf("automatic restore is forbidden on production-class environments")
	}
	if err := r.VerifyBackup(ctx, j); err != nil {
		return err
	}
	if err := backup.Restore(ctx, r.cfg, backup.RestoreOptions{BackupID: j.BackupID, Only: []string{"pg"}, Yes: true}); err != nil {
		return fmt.Errorf("restore step-one backup: %w", err)
	}
	return nil
}

// countRows reuses the exact backup verification oracle against the live database.
func (r *Runtime) countRows(ctx context.Context) (map[string]int64, error) {
	c := &backup.Container{Name: r.cfg.ProjectName + "_postgres", User: r.cfg.Postgres.User, DB: r.cfg.Postgres.DB}
	rows, err := runtimeBackupCountRows(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("count database rows: %w", err)
	}
	return rows, nil
}
