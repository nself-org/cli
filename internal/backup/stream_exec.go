package backup

// stream_exec.go — subprocess helpers for the streaming backup pipeline.
//
// Purpose: run the pg_dump and age-encrypt legs (the upload leg is the destination's PutStream) of the streaming pipeline that Stream (stream.go) wires together, split out for file size.
// Inputs: a context, the resolved StreamConfig fields and the pipe endpoints connecting the three legs.
// Outputs: the started *exec.Cmd for each leg, or an error if the binary is missing or fails to start.
// Constraints: the database password never goes on an argv, it travels in PGPASSWORD (D-0352). Keep in sync with runStreamPipeline in stream.go, which calls these in order.

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/backup/destinations"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// runPgDump executes pg_dump and writes its output to w.
// Uses pg_dump custom format for efficient streaming and restore.
func runPgDump(ctx context.Context, cfg *config.Config, pgURL string, w io.Writer) error {
	cmd, password, err := pgDumpCmd(ctx, pgURL)
	if err != nil {
		return err
	}
	cmd.Stdout = w
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start pg_dump: %w", err)
	}

	errOut, _ := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		if len(errOut) > 0 {
			msg := strings.TrimSpace(string(errOut))
			if password != "" {
				msg = strings.ReplaceAll(msg, password, "***")
			}
			return fmt.Errorf("%w: %s", errs.ErrBackupFailed, msg)
		}
		return fmt.Errorf("%w: %v", errs.ErrBackupFailed, err)
	}

	return nil
}

// pgDumpCmd builds the pg_dump command for pgURL and returns it with the
// password it carries. The password travels in PGPASSWORD in the child's
// environment, never in argv: argv is readable by every local user through
// `ps` for the length of the dump (D-0352). A DSN whose password cannot be
// split off is refused, so it never reaches argv either.
func pgDumpCmd(ctx context.Context, pgURL string) (*exec.Cmd, string, error) {
	connURL, password, err := splitPgPassword(pgURL)
	if err != nil {
		return nil, "", fmt.Errorf("%w: refusing to start pg_dump: %v", errs.ErrBackupFailed, err)
	}
	cmd := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--no-password", connURL)
	if password != "" {
		cmd.Env = append(os.Environ(), "PGPASSWORD="+password)
	}
	return cmd, password, nil
}

// ageEncryptStream pipes r through the age binary and writes ciphertext to w.
// Supports multiple recipients (age public keys, SSH public keys).
func ageEncryptStream(ctx context.Context, r io.Reader, w io.Writer, recipients []string) error {
	args := []string{}
	for _, rec := range recipients {
		args = append(args, "-r", rec)
	}

	cmd := exec.CommandContext(ctx, "age", args...)
	cmd.Stdin = r
	cmd.Stdout = w

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("age stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start age: %w", err)
	}

	errOut, _ := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		if len(errOut) > 0 {
			return fmt.Errorf("%w: %s", errs.ErrBackupEncryptFailed, strings.TrimSpace(string(errOut)))
		}
		return fmt.Errorf("%w: %v", errs.ErrBackupEncryptFailed, err)
	}

	return nil
}

// discardFailedObject runs after a stream backup failed once an upload may
// have started (a truncated dump reaches rclone as a clean EOF) or produced an
// empty object. It deletes the remote object so that nothing picking the newest
// object can select it, and returns cause. When the delete fails the returned
// error says the object is still on the remote; the caller still exits non-zero.
// The delete ignores a cancelled ctx: an interrupted backup must still clean up.
func discardFailedObject(ctx context.Context, cause error, rm destinations.Remover, key string) error {
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
	defer cancel()
	if err := rm.Remove(dctx, key); err != nil {
		return fmt.Errorf("%w; the remote object %s could NOT be removed (%v): delete it by hand, it is not a usable backup", cause, key, err)
	}
	return fmt.Errorf("%w (the partial remote object %s was removed)", cause, key)
}
