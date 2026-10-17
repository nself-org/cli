package backup

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
)

const restoreCleanupTimeout = 30 * time.Second

// restorePgDump restores into an isolated database and swaps it into place
// only after pg_restore succeeds. strict remains for API compatibility; local
// restores are always strict because accepting an archive error is unsafe.
func restorePgDump(ctx context.Context, container, user, db, backupFile string, _ bool) (retErr error) {
	maintenance, err := restoreMaintenanceDatabase(db, user)
	if err != nil {
		return err
	}
	scratch, previous, err := restoreDatabaseNames()
	if err != nil {
		return fmt.Errorf("%w: create scratch database name: %v", errs.ErrBackupRestoreFailed, err)
	}
	created := false
	defer func() {
		if !created {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), restoreCleanupTimeout)
		defer cancel()
		if cleanupErr := dropRestoreDatabase(cleanupCtx, container, user, maintenance, scratch); cleanupErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("cleanup scratch database %s: %w", scratch, cleanupErr))
		}
	}()

	createSQL := fmt.Sprintf("CREATE DATABASE %s WITH TEMPLATE template0 OWNER %s", quoteSQLIdentifier(scratch), quoteSQLIdentifier(user))
	if err := runRestoreSQL(ctx, container, user, maintenance, createSQL); err != nil {
		return fmt.Errorf("%w: create scratch database: %v", errs.ErrBackupRestoreFailed, err)
	}
	created = true

	if err := runScratchPgRestore(ctx, container, user, scratch, backupFile); err != nil {
		return err
	}
	if err := swapRestoredDatabase(ctx, container, user, maintenance, db, scratch, previous); err != nil {
		return err
	}
	created = false
	if err := dropRestoreDatabase(ctx, container, user, maintenance, previous); err != nil {
		return fmt.Errorf("%w: restored database is active; previous database retained as %s: %v", errs.ErrBackupRestoreFailed, previous, err)
	}
	return nil
}

func restoreMaintenanceDatabase(db, user string) (string, error) {
	if db == "template0" || db == "template1" {
		return "", fmt.Errorf("%w: refusing to replace PostgreSQL template database %q", errs.ErrBackupRestoreFailed, db)
	}
	if strings.ContainsRune(db, 0) || strings.ContainsRune(user, 0) {
		return "", fmt.Errorf("%w: database and user names cannot contain NUL", errs.ErrBackupRestoreFailed)
	}
	if db == "postgres" {
		return "template1", nil
	}
	return "postgres", nil
}

func restoreDatabaseNames() (scratch, previous string, err error) {
	suffix, err := restoreDatabaseSuffix()
	if err != nil {
		return "", "", err
	}
	return "nself_restore_" + suffix, "nself_previous_" + suffix, nil
}

func swapRestoredDatabase(ctx context.Context, container, user, maintenance, db, scratch, previous string) (retErr error) {
	blockSQL := fmt.Sprintf("ALTER DATABASE %s WITH ALLOW_CONNECTIONS false; ALTER DATABASE %s WITH ALLOW_CONNECTIONS false",
		quoteSQLIdentifier(scratch), quoteSQLIdentifier(db))
	if err := runRestoreSQL(ctx, container, user, maintenance, blockSQL); err != nil {
		return errors.Join(fmt.Errorf("%w: block database connections before swap: %v", errs.ErrBackupRestoreFailed, err),
			unblockRestoreTarget(container, user, maintenance, db))
	}
	blocked := true
	defer func() {
		if blocked {
			retErr = errors.Join(retErr, unblockRestoreTarget(container, user, maintenance, db))
		}
	}()
	terminateSQL := fmt.Sprintf("SELECT pg_terminate_backend(pid, 5000) FROM pg_stat_activity WHERE datname = %s AND pid <> pg_backend_pid()", quoteSQLLiteral(db))
	if err := runRestoreSQL(ctx, container, user, maintenance, terminateSQL); err != nil {
		return fmt.Errorf("%w: terminate database connections before swap: %v", errs.ErrBackupRestoreFailed, err)
	}
	swapSQL := fmt.Sprintf("BEGIN; ALTER DATABASE %s RENAME TO %s; ALTER DATABASE %s RENAME TO %s; ALTER DATABASE %s WITH ALLOW_CONNECTIONS true; COMMIT",
		quoteSQLIdentifier(db), quoteSQLIdentifier(previous), quoteSQLIdentifier(scratch), quoteSQLIdentifier(db), quoteSQLIdentifier(db))
	if err := runRestoreSQL(ctx, container, user, maintenance, swapSQL); err != nil {
		return fmt.Errorf("%w: swap restored database: %v", errs.ErrBackupRestoreFailed, err)
	}
	blocked = false
	return nil
}

func runScratchPgRestore(ctx context.Context, container, user, scratch, backupFile string) error {
	f, err := os.Open(backupFile)
	if err != nil {
		return fmt.Errorf("open backup file: %w", err)
	}
	defer func() { _ = f.Close() }()
	_, stderr, err := docker.ExecStdin(ctx, container,
		[]string{"pg_restore", "-U", user, "-d", scratch, "--exit-on-error"}, f)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%w: %v", errs.ErrBackupRestoreFailed, ctx.Err())
		}
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("%w: pg_restore: %s", errs.ErrBackupRestoreFailed, detail)
	}
	return nil
}

func runRestoreSQL(ctx context.Context, container, user, maintenance, sql string) error {
	_, stderr, err := docker.ExecCapture(ctx, container, []string{"psql", "-X", "-v", "ON_ERROR_STOP=1",
		"-U", user, "-d", maintenance, "-c", sql})
	if err != nil {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = err.Error()
		}
		return errors.New(detail)
	}
	return nil
}

func unblockRestoreTarget(container, user, maintenance, db string) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), restoreCleanupTimeout)
	defer cancel()
	if err := runRestoreSQL(cleanupCtx, container, user, maintenance,
		fmt.Sprintf("ALTER DATABASE %s WITH ALLOW_CONNECTIONS true", quoteSQLIdentifier(db))); err != nil {
		return fmt.Errorf("re-enable original database connections: %w", err)
	}
	return nil
}

func dropRestoreDatabase(ctx context.Context, container, user, maintenance, db string) error {
	return runRestoreSQL(ctx, container, user, maintenance, fmt.Sprintf("DROP DATABASE %s", quoteSQLIdentifier(db)))
}

func restoreDatabaseSuffix() (string, error) {
	var raw [8]byte
	if _, err := randRead(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func quoteSQLIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func quoteSQLLiteral(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return `E'` + strings.ReplaceAll(value, `'`, `''`) + `'`
}
