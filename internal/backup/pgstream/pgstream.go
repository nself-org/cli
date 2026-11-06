// Package pgstream is the three-stage restore pipe behind
// `nself backup restore-remote`: download | decrypt | pg_restore.
//
// Purpose: stream a backup object into an existing (scratch) database with no
// local temp file. Split out of internal/backup (package-size ratchet).
// Inputs: the source object, the postgres container, user and target
// database, and the caller's verdict on pg_restore's stderr.
// Outputs: nil, or the first stage error (errs.ErrBackupRemoteFailed,
// ErrBackupDecryptFailed, or whatever verdict returns).
// Constraints: pg_restore runs via docker exec inside the container. The
// caller owns database creation, the swap and confirmation.
package pgstream

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/nself-org/cli/internal/errs"
)

// Source names the backup object: From is the rclone remote path, used when
// Native is nil; AgeKey decrypts it when Encrypted.
type Source struct {
	From      string
	Native    io.ReadCloser
	AgeKey    string
	Encrypted bool
}

// Restore runs rclone cat (or src.Native) | age --decrypt | pg_restore into
// database scratch inside container. verdict judges pg_restore's stderr after
// a non-zero exit; nil from it accepts the restore.
func Restore(ctx context.Context, src Source, container, user, scratch string, verdict func(stderr string) error) error {
	from, native, keyPath, encrypted := src.From, src.Native, src.AgeKey, src.Encrypted
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var downloadReader io.ReadCloser
	var restoreReader io.ReadCloser

	// Stage 1 output.
	dlR, dlW := io.Pipe()

	var wg sync.WaitGroup
	errc := make(chan error, 3)

	// ── Stage 1: rclone cat <remote> (or the native reader) ─────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { _ = dlW.Close() }()
		if native != nil {
			if err := copyNative(dlW, native); err != nil {
				cancel()
				errc <- fmt.Errorf("%w: %v", errs.ErrBackupRemoteFailed, err)
			}
			return
		}
		cmd := exec.CommandContext(ctx, "rclone", "cat", from)
		cmd.Stdout = dlW
		stderr, err := cmd.StderrPipe()
		if err != nil {
			cancel()
			errc <- fmt.Errorf("rclone stderr pipe: %w", err)
			return
		}
		if err := cmd.Start(); err != nil {
			cancel()
			errc <- fmt.Errorf("start rclone: %w", err)
			return
		}
		errOut, _ := io.ReadAll(stderr)
		if err := cmd.Wait(); err != nil {
			cancel()
			msg := strings.TrimSpace(string(errOut))
			if msg != "" {
				errc <- fmt.Errorf("%w: %s", errs.ErrBackupRemoteFailed, msg)
			} else {
				errc <- fmt.Errorf("%w: %v", errs.ErrBackupRemoteFailed, err)
			}
		}
	}()

	downloadReader = dlR

	// ── Stage 2: age -d (optional) ───────────────────────────────────
	var decR *io.PipeReader
	var decW *io.PipeWriter
	if encrypted {
		decR, decW = io.Pipe()
		restoreReader = decR
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { _ = decW.Close() }()
			cmd := exec.CommandContext(ctx, "age", "--decrypt", "-i", keyPath)
			cmd.Stdin = downloadReader
			cmd.Stdout = decW
			stderr, err := cmd.StderrPipe()
			if err != nil {
				cancel()
				dlR.CloseWithError(err)
				errc <- fmt.Errorf("age stderr pipe: %w", err)
				return
			}
			if err := cmd.Start(); err != nil {
				cancel()
				dlR.CloseWithError(err)
				errc <- fmt.Errorf("start age: %w", err)
				return
			}
			errOut, _ := io.ReadAll(stderr)
			if err := cmd.Wait(); err != nil {
				cancel()
				msg := strings.TrimSpace(string(errOut))
				if msg != "" {
					errc <- fmt.Errorf("%w: %s", errs.ErrBackupDecryptFailed, msg)
				} else {
					errc <- fmt.Errorf("%w: %v", errs.ErrBackupDecryptFailed, err)
				}
			}
		}()
	} else {
		restoreReader = downloadReader
	}

	// ── Stage 3: pg_restore into the scratch database ─────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { _ = restoreReader.Close() }()
		args := []string{
			"exec", "-i", container,
			"pg_restore",
			"-U", user,
			"-d", scratch,
		}
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Stdin = restoreReader
		stderr, err := cmd.StderrPipe()
		if err != nil {
			cancel()
			errc <- fmt.Errorf("pg_restore stderr pipe: %w", err)
			return
		}
		if err := cmd.Start(); err != nil {
			cancel()
			errc <- fmt.Errorf("start pg_restore: %w", err)
			return
		}
		errOut, _ := io.ReadAll(stderr)
		if err := cmd.Wait(); err != nil {
			if verr := verdict(string(errOut)); verr != nil {
				cancel()
				errc <- verr
			}
		}
	}()

	wg.Wait()
	close(errc)

	for e := range errc {
		if e != nil {
			return e
		}
	}

	return nil
}

// copyNative copies the native reader into w and closes it, so a failed
// remote read (reported by Close) becomes the stage error.
func copyNative(w io.Writer, rc io.ReadCloser) error {
	_, err := io.Copy(w, rc)
	if cerr := rc.Close(); err == nil {
		err = cerr
	}
	return err
}
