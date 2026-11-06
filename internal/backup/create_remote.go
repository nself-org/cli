// Package backup — create_remote.go: post-create encryption and remote
// upload for createFullBackup (finalizeFullBackup), plus the rclone/S3
// credential checks. Split out of create_targets.go (file-size ratchet,
// internal/repoqa). A configured upload that cannot run or fails is an error
// that names the kept local file (C2-02): a backup that only exists on the
// box it protects is not the backup the operator asked for.
package backup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nself-org/cli/internal/backup/destinations"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/errs"
)

// encryptFile encrypts path with age for recipient, removes the plaintext
// and returns the path of the encrypted file (path + ".age"). Callers must use
// the returned path: the original no longer exists.
func encryptFile(path, recipient string) (string, error) {
	encPath := path + ".age"
	cmd := exec.Command("age", "-r", recipient, "-o", encPath, path)
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(encPath)
		return "", fmt.Errorf("%w: %s", errs.ErrBackupEncryptFailed, string(output))
	}
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("remove unencrypted file: %w", err)
	}
	return encPath, nil
}

// finalizeFullBackup encrypts the finished dump when asked and uploads the
// resulting file to the configured remote. It returns the final local path.
// An upload that is configured but skipped (half-set S3 keys) or fails is an
// error naming the local file, so `nself backup create` exits non-zero and the
// backup metric records a failure instead of a success.
func finalizeFullBackup(ctx context.Context, cfg *config.Config, outputPath string, opts CreateOptions) (string, error) {
	encrypt := (cfg.Backup.Encryption || opts.Encrypt) && !opts.NoEncrypt
	// Asking for encryption and getting plaintext is worse than not asking:
	// refuse when no recipient is configured.
	if encrypt && cfg.Backup.AgeRecipients == "" {
		return outputPath, fmt.Errorf(
			"encryption requested but no recipient configured: set NSELF_BACKUP_AGE_RECIPIENTS, or pass --no-encrypt to write the backup in the clear")
	}
	if encrypt {
		encPath, err := encryptFile(outputPath, cfg.Backup.AgeRecipients)
		if err != nil {
			return outputPath, fmt.Errorf("encrypt backup: %w", err)
		}
		outputPath = encPath
	}
	remote := cfg.Backup.Remote
	if opts.Remote != "" {
		remote = opts.Remote
	}
	if remote == "" {
		return outputPath, nil
	}
	// Half-configured S3 keys look configured but upload nothing or fail
	// opaquely inside rclone: refuse loudly before attempting the upload.
	if err := requireCompleteS3CredentialsFor(remote, cfg); err != nil {
		return outputPath, fmt.Errorf("%w: backup kept locally at %s but not uploaded to %s: %v", errs.ErrBackupRemoteFailed, outputPath, remote, err)
	}
	if err := uploadToRemote(ctx, outputPath, remote, cfg); err != nil {
		return outputPath, fmt.Errorf("backup kept locally at %s but the upload to %s failed: %w", outputPath, remote, err)
	}
	return outputPath, nil
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

// requireCompleteS3CredentialsFor applies requireCompleteS3Credentials to the
// rclone/S3 kind only: path:// and host:// uploads never use S3 keys, so a
// half-set key pair must not block them.
func requireCompleteS3CredentialsFor(remote string, cfg *config.Config) error {
	if destinations.KindOf(remote) != destinations.KindRclone {
		return nil
	}
	return requireCompleteS3Credentials(cfg)
}

// destinationFor parses a --remote / --from destination. host:// loads the
// controlplane inventory from the current directory (the project root);
// rcloneEnv reaches every rclone process the destination starts.
func destinationFor(uri string, rcloneEnv ...string) (destinations.Destination, error) {
	var inv *destinations.Inventory
	if destinations.KindOf(uri) == destinations.KindHost {
		var err error
		if inv, err = controlplane.Load("."); err != nil {
			return nil, err
		}
	}
	return destinations.Parse(uri, inv, rcloneEnv...)
}

// rcloneEnvFor returns the AWS_* variables for an rclone process when cfg
// holds an S3 key pair, else nil.
func rcloneEnvFor(cfg *config.Config) []string {
	if cfg.Backup.S3AccessKeyID != "" && cfg.Backup.S3SecretAccessKey != "" {
		return []string{
			"AWS_ACCESS_KEY_ID=" + cfg.Backup.S3AccessKeyID,
			"AWS_SECRET_ACCESS_KEY=" + cfg.Backup.S3SecretAccessKey,
		}
	}
	return nil
}

// uploadToRemote uploads a local file to the configured destination: an
// rclone remote, path://<dir> or host://<server>/<dir>. For rclone, when cfg
// carries an S3 access/secret key pair (BACKUP_S3_ACCESS_KEY_ID /
// BACKUP_S3_SECRET_ACCESS_KEY, or the BACKUP_ACCESS_KEY / BACKUP_SECRET_KEY
// aliases), it is exported as AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY for
// this rclone invocation: the env-var form rclone's :s3 / :r2 remotes read
// when no matching entry exists in rclone.conf. Callers must have already
// checked requireCompleteS3Credentials; this function does not re-check.
func uploadToRemote(ctx context.Context, localPath, remote string, cfg *config.Config) error {
	env := rcloneEnvFor(cfg)
	dest, err := destinationFor(remote, env...)
	if err != nil {
		return fmt.Errorf("%w: %v", errs.ErrBackupRemoteFailed, err)
	}
	if err := dest.Put(ctx, localPath, filepath.Base(localPath)); err != nil {
		return fmt.Errorf("%w: %s", errs.ErrBackupRemoteFailed, err)
	}
	return nil
}
