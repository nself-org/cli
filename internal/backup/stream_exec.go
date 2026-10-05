package backup

// stream_exec.go — subprocess helpers for the streaming backup pipeline.
//
// Purpose: run the pg_dump, age-encrypt and rclone-rcat legs of the streaming pipeline that Stream (stream.go) wires together, split out for file size.
// Inputs: a context, the resolved StreamConfig fields and the pipe endpoints connecting the three legs.
// Outputs: the started *exec.Cmd for each leg, or an error if the binary is missing or fails to start.
// Constraints: pure move from stream.go (CLI-R12 Batch E); no behaviour change. Keep in sync with runStreamPipeline in stream.go, which calls these in order.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// runPgDump executes pg_dump and writes its output to w.
// Uses pg_dump custom format for efficient streaming and restore.
func runPgDump(ctx context.Context, cfg *config.Config, pgURL string, w io.Writer) error {
	args := []string{
		"--format=custom",
		"--no-password",
		pgURL,
	}

	cmd := exec.CommandContext(ctx, "pg_dump", args...)
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
			return fmt.Errorf("%w: %s", errs.ErrBackupFailed, strings.TrimSpace(string(errOut)))
		}
		return fmt.Errorf("%w: %v", errs.ErrBackupFailed, err)
	}

	return nil
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

// rcloneRcat uploads from r to destination/key using rclone rcat.
// rclone handles multipart uploads internally for all supported backends.
func rcloneRcat(ctx context.Context, r io.Reader, destination, key string) error {
	remote := destination
	if !strings.HasSuffix(remote, "/") {
		remote = remote + "/"
	}
	remote = remote + key

	cmd := exec.CommandContext(ctx, "rclone", "rcat", remote)
	cmd.Stdin = r

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("rclone stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start rclone: %w", err)
	}

	errOut, _ := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		if len(errOut) > 0 {
			return fmt.Errorf("%w: %s", errs.ErrBackupRemoteFailed, strings.TrimSpace(string(errOut)))
		}
		return fmt.Errorf("%w: %v", errs.ErrBackupRemoteFailed, err)
	}

	return nil
}

// rcloneDeleteFile removes destination/key with rclone deletefile. An object
// that is not there (rclone exit 3 or 4) counts as removed.
func rcloneDeleteFile(ctx context.Context, destination, key string) error {
	remote := destination
	if !strings.HasSuffix(remote, "/") {
		remote += "/"
	}
	out, err := exec.CommandContext(ctx, "rclone", "deletefile", remote+key).CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) && (ee.ExitCode() == 3 || ee.ExitCode() == 4) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: %v: %s", errs.ErrBackupRemoteFailed, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// discardFailedObject runs after a stream backup failed once an upload may
// have started (a truncated dump reaches rclone as a clean EOF) or produced an
// empty object. It deletes the remote object so that nothing picking the newest
// object can select it, and returns cause. When the delete fails the returned
// error says the object is still on the remote; the caller still exits non-zero.
// The delete ignores a cancelled ctx: an interrupted backup must still clean up.
func discardFailedObject(ctx context.Context, cause error, destination, key string) error {
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
	defer cancel()
	if err := rcloneDeleteFile(dctx, destination, key); err != nil {
		return fmt.Errorf("%w; the remote object %s could NOT be removed (%v): delete it by hand, it is not a usable backup", cause, key, err)
	}
	return fmt.Errorf("%w (the partial remote object %s was removed)", cause, key)
}
