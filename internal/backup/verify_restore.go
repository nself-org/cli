package backup

// Purpose: restore-test verification of local plain or age-encrypted dumps.
// Inputs: the backup path, host config and verification options.
// Outputs: an error unless restore and smoke/sentinel checks pass.
// Constraints: uses the same default identity as rollback; plaintext is 0600
// and removed on every exit. Container cleanup survives context cancellation.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// runRestoreTest restores a dump into a test container and checks its data.
func runRestoreTest(ctx context.Context, cfg *config.Config, backupFile string, opts VerifyOptions) error {
	// Check the original name: decryptFile returns a unique .dec plaintext file.
	// Legacy base-backup tars have no automated restore path here.
	if !strings.HasSuffix(strings.TrimSuffix(backupFile, ".age"), ".dump") {
		return fmt.Errorf("%w: %s is not a restorable pg_dump (.dump) backup; recreate it with the default format before running --restore-test",
			errs.ErrBackupVerifyFailed, backupFile)
	}
	if strings.HasSuffix(backupFile, ".age") {
		decrypted, err := decryptFile(ctx, backupFile, "", cfg.ProjectName)
		if err != nil {
			return fmt.Errorf("restore test decrypt backup: %w", err)
		}
		defer func() { _ = os.Remove(decrypted) }()
		backupFile = decrypted
	}
	c, err := StartContainer(ctx, ContainerSpec{
		Name:     cfg.ProjectName + verifySuffix,
		Volume:   cfg.ProjectName + "_restore_test_data",
		User:     cfg.Postgres.User,
		DB:       cfg.Postgres.DB,
		Password: cfg.Postgres.Password,
		Image:    compose.ImageRef("postgres", cfg.Postgres.Version),
		Keep:     opts.Keep,
		Ready:    30 * time.Second,
	})
	if err != nil {
		return err
	}
	defer c.Remove()
	if err := RestoreIntoContainer(ctx, c, backupFile); err != nil {
		return fmt.Errorf("restore test failed: %w", err)
	}
	return smokeAndSentinel(ctx, c)
}
