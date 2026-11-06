// Package promote implements staging-to-production promotion with diff preview and rollback.
package promote

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/config"
)

// DiffResult holds the comparison between two environments.
type DiffResult struct {
	Source     string      `json:"source"`
	Target     string      `json:"target"`
	Migrations []DiffEntry `json:"migrations"`
	EnvVars    []DiffEntry `json:"env_vars"`
	Images     []DiffEntry `json:"images"`
	Metadata   []DiffEntry `json:"metadata"`
	Timestamp  time.Time   `json:"timestamp"`
}

// DiffEntry represents a single difference between environments.
type DiffEntry struct {
	Key         string `json:"key"`
	SourceValue string `json:"source_value"`
	TargetValue string `json:"target_value"`
	Status      string `json:"status"` // added, removed, changed, same
}

// PromotionRecord records an executed promotion.
type PromotionRecord struct {
	ID          string    `json:"id"`
	Source      string    `json:"source"`
	Target      string    `json:"target"`
	ApproveID   string    `json:"approve_id"`
	BackupTag   string    `json:"backup_tag"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Status      string    `json:"status"` // success, failed, rolled_back
	Error       string    `json:"error,omitempty"`
}

// DryRun computes the diff between source and target environments.
func DryRun(ctx context.Context, projectDir, source, target string) (*DiffResult, error) {
	result := &DiffResult{
		Source:    source,
		Target:    target,
		Timestamp: time.Now(),
	}

	// Compare env files
	sourceEnv, err := loadEnvFile(filepath.Join(projectDir, fmt.Sprintf(".env.%s", source)))
	if err != nil {
		return nil, fmt.Errorf("load source env: %w", err)
	}
	targetEnv, err := loadEnvFile(filepath.Join(projectDir, fmt.Sprintf(".env.%s", target)))
	if err != nil {
		return nil, fmt.Errorf("load target env: %w", err)
	}

	result.EnvVars = diffMaps(sourceEnv, targetEnv)

	// Compare migration directories
	sourceMigDir := filepath.Join(projectDir, "migrations", source)
	targetMigDir := filepath.Join(projectDir, "migrations", target)
	result.Migrations = diffDirectories(sourceMigDir, targetMigDir)

	// Compare Docker image digests
	result.Images = diffDockerImages(ctx, projectDir, source, target)

	return result, nil
}

// Execute runs the full promotion pipeline.
func Execute(ctx context.Context, projectDir, source, target, approveID string) (*PromotionRecord, error) {
	record := &PromotionRecord{
		ID:        fmt.Sprintf("promo-%d", time.Now().UnixNano()),
		Source:    source,
		Target:    target,
		ApproveID: approveID,
		StartedAt: time.Now(),
	}

	// 1. Pre-flight: run nself doctor --deep
	cmd := exec.CommandContext(ctx, "nself", "doctor", "--full")
	if out, err := cmd.CombinedOutput(); err != nil {
		record.Status = "failed"
		record.Error = fmt.Sprintf("pre-flight doctor failed: %s", string(out))
		record.CompletedAt = time.Now()
		return record, fmt.Errorf("pre-flight check failed: %w", err)
	}

	// 2. Create backup with promotion tag
	backupTag := fmt.Sprintf("pre-promote-%s-%d", target, time.Now().Unix())
	record.BackupTag = backupTag
	cmd = exec.CommandContext(ctx, "nself", "backup", "create", "--tag", backupTag)
	if out, err := cmd.CombinedOutput(); err != nil {
		record.Status = "failed"
		record.Error = fmt.Sprintf("backup failed: %s", string(out))
		record.CompletedAt = time.Now()
		return record, fmt.Errorf("backup failed: %w", err)
	}

	// 3. Apply migrations
	cmd = exec.CommandContext(ctx, "nself", "db", "migrate", "--env", target)
	if out, err := cmd.CombinedOutput(); err != nil {
		record.Status = "failed"
		record.Error = fmt.Sprintf("migration failed: %s", string(out))
		record.CompletedAt = time.Now()
		return record, fmt.Errorf("migration failed: %w", err)
	}

	// 4. Rebuild and restart services
	cmd = exec.CommandContext(ctx, "nself", "build", "--env", target)
	if out, err := cmd.CombinedOutput(); err != nil {
		record.Status = "failed"
		record.Error = fmt.Sprintf("build failed: %s", string(out))
		record.CompletedAt = time.Now()
		return record, fmt.Errorf("build failed: %w", err)
	}

	cmd = exec.CommandContext(ctx, "nself", "restart", "--env", target)
	if out, err := cmd.CombinedOutput(); err != nil {
		record.Status = "failed"
		record.Error = fmt.Sprintf("restart failed: %s", string(out))
		record.CompletedAt = time.Now()
		return record, fmt.Errorf("restart failed: %w", err)
	}

	// 5. Post-flight: run doctor again
	cmd = exec.CommandContext(ctx, "nself", "doctor")
	if out, err := cmd.CombinedOutput(); err != nil {
		record.Status = "failed"
		record.Error = fmt.Sprintf("post-flight doctor failed: %s", string(out))
		record.CompletedAt = time.Now()
		return record, fmt.Errorf("post-flight check failed: %w", err)
	}

	record.Status = "success"
	record.CompletedAt = time.Now()

	// Save promotion record
	if err := saveRecord(projectDir, record); err != nil {
		return record, fmt.Errorf("save record: %w", err)
	}

	return record, nil
}

// runNself runs the nself binary in dir; tests replace it to assert the argv.
var runNself = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "nself", args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// listBackups lists the project's backups, newest first; tests replace it.
var listBackups = func(projectDir string) ([]backup.BackupEntry, error) {
	cfg, err := config.Load(projectDir)
	if err != nil {
		return nil, err
	}
	return backup.List(cfg, backup.ListOptions{})
}

// Rollback keeps the v1.4 signature for callers that cannot name the project.
// It always errors: a restore needs the project name, and it is never read from
// the local config on production (cli#1083). Callers use RollbackProject.
func Rollback(ctx context.Context, projectDir, backupTag string) error {
	return RollbackProject(ctx, projectDir, "", backupTag)
}

// RollbackProject restores the newest backup whose tag equals backupTag (the
// latest promote record's tag when empty) by running
// `nself backup restore <id> --yes --confirm-project <project>`.
// No backup carrying the tag is an error naming the tag; nothing is restored.
func RollbackProject(ctx context.Context, projectDir, project, backupTag string) error {
	if project == "" {
		return fmt.Errorf("rollback needs the project name: pass it explicitly, it is not read from the local config")
	}
	if backupTag == "" {
		// Find latest pre-promote backup
		tag, err := findLatestPromoteBackup(projectDir)
		if err != nil {
			return fmt.Errorf("find backup: %w", err)
		}
		backupTag = tag
	}
	id, err := backupIDForTag(projectDir, backupTag)
	if err != nil {
		return err
	}
	out, err := runNself(ctx, projectDir, "backup", "restore", id, "--yes", "--confirm-project", project)
	if err != nil {
		return fmt.Errorf("restore failed: %s: %w", string(out), err)
	}
	return nil
}

// backupIDForTag returns the ID of the newest backup whose parsed tag is tag.
func backupIDForTag(projectDir, tag string) (string, error) {
	entries, err := listBackups(projectDir)
	if err != nil {
		return "", fmt.Errorf("list backups: %w", err)
	}
	var best *backup.BackupEntry
	for i := range entries {
		if entries[i].Tag == tag && (best == nil || entries[i].Date.After(best.Date)) {
			best = &entries[i]
		}
	}
	if best == nil {
		return "", fmt.Errorf("no backup carries tag %q; nothing was restored", tag)
	}
	return best.ID, nil
}
