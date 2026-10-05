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

	"github.com/nself-org/cli/internal/backup/destinations"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/controlplane"
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
