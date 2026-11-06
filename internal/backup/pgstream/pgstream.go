// Package pgstream is the three-stage pipe behind `nself backup
// restore-remote`: download | decrypt | load, with no local temp file.
//
// Purpose: split out of internal/backup (package-size ratchet).
// Inputs: the source object, an age identity and the caller's load (pg_restore
// into a scratch database through internal/docker).
// Outputs: nil, or the first stage error wrapped in the caller's sentinels.
// Constraints: stdlib only (no errs or docker import: fan-in and docker funnel
// ratchets). The caller owns the database, the swap and confirmation.
package pgstream

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Source names the backup object and its consumer. From is the rclone remote
// path, used when Native is nil; AgeKey decrypts it when Encrypted. Load
// consumes the plaintext stream. RemoteErr and DecryptErr wrap download and
// decrypt failures (the caller's sentinels).
type Source struct {
	From       string
	Native     io.ReadCloser
	AgeKey     string
	Encrypted  bool
	Load       func(ctx context.Context, r io.Reader) error
	RemoteErr  error
	DecryptErr error
}

// Stream runs rclone cat (or src.Native) | age --decrypt | src.Load and
// returns the first stage error.
func Stream(ctx context.Context, src Source) error {
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
				errc <- fmt.Errorf("%w: %v", src.RemoteErr, err)
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
				errc <- fmt.Errorf("%w: %s", src.RemoteErr, msg)
			} else {
				errc <- fmt.Errorf("%w: %v", src.RemoteErr, err)
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
					errc <- fmt.Errorf("%w: %s", src.DecryptErr, msg)
				} else {
					errc <- fmt.Errorf("%w: %v", src.DecryptErr, err)
				}
			}
		}()
	} else {
		restoreReader = downloadReader
	}

	// ── Stage 3: the caller's load (pg_restore into the scratch database) ──
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { _ = restoreReader.Close() }()
		if err := src.Load(ctx, restoreReader); err != nil {
			cancel()
			errc <- err
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
