// Package backup — create_remote.go: post-create encryption and remote
// (rclone) upload helpers used by createFullBackup. Split out of
// create_targets.go (file-size ratchet, internal/repoqa) as a pure move —
// no behavior change beyond what is documented on requireCompleteS3Credentials
// and uploadToRemote below.
package backup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// encryptFile encrypts a file in-place using age with the given recipient public key.
func encryptFile(path, recipient string) error {
	encPath := path + ".age"
	args := []string{"-r", recipient, "-o", encPath, path}
	cmd := exec.Command("age", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", errs.ErrBackupEncryptFailed, string(output))
	}

	// Replace original with encrypted version.
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove unencrypted file: %w", err)
	}
	if err := os.Rename(encPath, path+".age"); err != nil {
		return fmt.Errorf("rename encrypted file: %w", err)
	}

	return nil
}

// requireCompleteS3Credentials refuses a half-configured S3 credential pair.
// Exactly one of BACKUP_S3_ACCESS_KEY_ID/BACKUP_ACCESS_KEY and
// BACKUP_S3_SECRET_ACCESS_KEY/BACKUP_SECRET_KEY being set is never valid: it
// is not "use rclone.conf instead" (that path is both empty) and it is not
// "use these creds" (that path is both set) — it is a typo or partial
// migration between the two accepted name pairs, and rclone/S3 would either
// silently fail or use an empty-string secret. Both-empty is fine: the
// remote may be fully configured via rclone.conf or RCLONE_CONFIG_* env vars
// with no nSelf-side credentials at all.
func requireCompleteS3Credentials(cfg *config.Config) error {
	hasAccess := cfg.Backup.S3AccessKeyID != ""
	hasSecret := cfg.Backup.S3SecretAccessKey != ""
	if hasAccess == hasSecret {
		return nil
	}
	missing := "BACKUP_S3_SECRET_ACCESS_KEY (or BACKUP_SECRET_KEY)"
	if hasSecret {
		missing = "BACKUP_S3_ACCESS_KEY_ID (or BACKUP_ACCESS_KEY)"
	}
	return fmt.Errorf("S3 backup credentials are half-configured: %s is set but its counterpart is not", missing)
}

// uploadToRemote uploads a local file to the configured rclone remote. When
// cfg carries an S3 access/secret key pair (BACKUP_S3_ACCESS_KEY_ID /
// BACKUP_S3_SECRET_ACCESS_KEY, or the BACKUP_ACCESS_KEY / BACKUP_SECRET_KEY
// aliases), it is exported as AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY for
// this rclone invocation — the env-var form rclone's :s3 / :r2 remotes read
// when no matching entry exists in rclone.conf. Callers must have already
// checked requireCompleteS3Credentials; this function does not re-check.
func uploadToRemote(ctx context.Context, localPath, remote string, cfg *config.Config) error {
	args := []string{"copyto", localPath, remote + "/" + filepath.Base(localPath)}
	cmd := exec.CommandContext(ctx, "rclone", args...)
	cmd.Env = os.Environ()
	if cfg.Backup.S3AccessKeyID != "" && cfg.Backup.S3SecretAccessKey != "" {
		cmd.Env = append(cmd.Env,
			"AWS_ACCESS_KEY_ID="+cfg.Backup.S3AccessKeyID,
			"AWS_SECRET_ACCESS_KEY="+cfg.Backup.S3SecretAccessKey,
		)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", errs.ErrBackupRemoteFailed, string(output))
	}
	return nil
}
