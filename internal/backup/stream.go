// Package backup — streaming encrypted backup to remote destinations.
//
// The pipeline runs three concurrent goroutines:
//
//	pg_dump (streaming) | age (encrypt) | rclone (multipart upload)
//
// No temp files are written. Encryption recipients may be age public keys,
// SSH public keys, or GitHub user keys (fetched via github.com/users/<user>/keys).
// The upload destination is any rclone-supported remote: s3://, r2://, b2://, gcs://, az://.
package backup

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// StreamConfig holds parameters for a streaming encrypted backup.
type StreamConfig struct {
	// PgURL is the postgres DSN used by pg_dump. If empty, derived from cfg.
	PgURL string

	// Destination is the rclone remote path, e.g. "s3:bucket/prefix/".
	Destination string

	// Recipients holds age/SSH public keys (one per entry). May also be
	// "github:<username>" to fetch the user's SSH public keys from GitHub.
	Recipients []string

	// Key is the object name appended to Destination. Auto-generated if empty.
	Key string

	// ChunkMB is the multipart chunk size. Defaults to 64.
	ChunkMB int
}

// StreamOptions holds CLI flag values for `nself backup stream`.
type StreamOptions struct {
	To         string   // destination URL (rclone remote path)
	Recipients []string // --recipient flags (may be specified multiple times)
	DryRun     bool
	// AllowUnencrypted permits streaming in the clear when no recipient
	// resolves. Off by default: without it, a missing recipient is an error
	// rather than a silent plaintext upload. See the check in Stream.
	AllowUnencrypted bool

	// HeartbeatTo is the rclone remote that receives <project>/backup.json
	// after a successful upload (--heartbeat-to). Empty falls back to
	// NSELF_BACKUP_HEARTBEAT_REMOTE; with neither, no heartbeat is written.
	HeartbeatTo string

	// HeartbeatRequired makes Stream return an error when the heartbeat cannot
	// be written (--heartbeat-required). Off by default: a heartbeat outage
	// must never lose or fail the backup itself.
	HeartbeatRequired bool
}

// StreamResult is returned by Stream on success.
type StreamResult struct {
	BackupID    string    `json:"backup_id"`
	Destination string    `json:"destination"`
	StartedAt   time.Time `json:"started_at"`
	Duration    string    `json:"duration"`
	Encrypted   bool      `json:"encrypted"`
	// Bytes is the size of the uploaded (encrypted) object.
	Bytes int64 `json:"bytes,omitempty"`
}

// Stream runs a three-stage concurrent pipeline:
//
//  1. pg_dump stdout -> pgWriter
//  2. age encrypts pgReader -> encWriter (skipped if no recipients)
//  3. rclone rcat uploads encReader to Destination/Key
//
// All three stages run in parallel goroutines. The first error from any stage
// cancels the others via context cancellation.
//
// When a heartbeat remote is set (opts.HeartbeatTo or
// NSELF_BACKUP_HEARTBEAT_REMOTE) the row estimates are read before the dump
// starts and, only after the upload succeeded, <project>/backup.json is
// written there. A heartbeat failure is logged; it is returned (together with
// the successful result) only when opts.HeartbeatRequired is set.
func Stream(ctx context.Context, cfg *config.Config, opts StreamOptions) (*StreamResult, error) {
	if opts.To == "" {
		if cfg.Backup.Remote != "" {
			opts.To = cfg.Backup.Remote
		} else {
			return nil, fmt.Errorf("destination required: use --to <url> or set NSELF_BACKUP_DESTINATION")
		}
	}

	recipients := opts.Recipients
	if len(recipients) == 0 && cfg.Backup.AgeRecipients != "" {
		recipients = strings.Fields(cfg.Backup.AgeRecipients)
	}

	// Resolve GitHub keys.
	var err error
	recipients, err = resolveRecipients(ctx, recipients)
	if err != nil {
		return nil, fmt.Errorf("resolve recipients: %w", err)
	}

	// Fail closed. Encryption was previously skipped whenever no recipient
	// resolved, with no error and no warning, so `nself backup stream --to
	// s3:bucket/path` uploaded a plaintext database dump to object storage
	// while the operator had every reason to believe it was encrypted. Under
	// the Security-Always-Free doctrine a silent-plaintext default is a defect,
	// and the failure is invisible precisely when it matters: offsite backups.
	if len(recipients) == 0 && !opts.AllowUnencrypted {
		return nil, fmt.Errorf(
			"refusing to stream an unencrypted backup: no recipient configured "+
				"(pass --recipient <age1...|ssh-...|github:username>, or set %s, "+
				"or pass --no-encrypt to stream in the clear)",
			"NSELF_BACKUP_AGE_RECIPIENTS")
	}
	if len(recipients) == 0 {
		slog.Warn("streaming an UNENCRYPTED backup: --no-encrypt was passed and no recipient is configured",
			"destination", opts.To)
	}

	// Build the object key.
	ts := time.Now().UTC().Format("20060102_150405")
	key := fmt.Sprintf("%s_stream_%s.sql", cfg.ProjectName, ts)
	if len(recipients) > 0 {
		key += ".age"
	}

	pgURL := buildPgURL(cfg)

	if opts.DryRun {
		slog.Info("dry-run: streaming backup",
			"destination", opts.To+"/"+key,
			"encrypted", len(recipients) > 0,
			"pg_url", redactURL(pgURL),
		)
		return &StreamResult{
			BackupID:    key,
			Destination: opts.To + "/" + key,
			StartedAt:   time.Now(),
			Duration:    "0s",
			Encrypted:   len(recipients) > 0,
		}, nil
	}

	start := time.Now()

	if err := checkBinaries(recipients); err != nil {
		return nil, err
	}

	// Row estimates are read BEFORE the dump so they describe the database the
	// backup contains; an unreadable estimate is recorded as null, never as {}.
	hbRemote := opts.HeartbeatTo
	if hbRemote == "" {
		hbRemote = cfg.Backup.HeartbeatRemote()
	}
	var approx map[string]int64
	if hbRemote != "" {
		var rowsErr error
		if approx, rowsErr = ReadApproxRows(ctx, pgURL); rowsErr != nil {
			slog.Warn("could not read row estimates; heartbeat approx_rows will be null", "err", rowsErr)
			approx = nil
		}
	}

	result, err := runStreamPipeline(ctx, cfg, pgURL, opts.To, key, recipients)
	if err != nil {
		return nil, err // a failed backup never writes a heartbeat
	}
	result.StartedAt = start
	result.Duration = time.Since(start).String()

	// An empty object is not a backup (an encrypted one always has the age
	// header, so this only happens with --no-encrypt and an empty dump). Fail
	// the job instead of reporting green; no heartbeat is written either.
	if result.Bytes == 0 {
		return nil, fmt.Errorf("%w: the upload was empty (0 bytes); the object %s on the remote holds no data and must not be trusted", errs.ErrBackupFailed, key)
	}

	if hbRemote != "" {
		if hbErr := publishHeartbeat(ctx, cfg.ProjectName, hbRemote, result, approx); hbErr != nil {
			slog.Warn("backup heartbeat not written", "err", hbErr)
			if opts.HeartbeatRequired {
				return result, fmt.Errorf("heartbeat: %w", hbErr)
			}
		}
	}
	return result, nil
}

// runStreamPipeline wires the three concurrent goroutines and waits for all.
func runStreamPipeline(ctx context.Context, cfg *config.Config, pgURL, destination, key string, recipients []string) (*StreamResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Stage plumbing: two io.Pipe connections.
	//   pg_dump -> pgW | pgR -> (age) -> encW | encR -> rclone
	pgR, pgW := io.Pipe()
	var uploadReader io.ReadCloser

	encrypt := len(recipients) > 0
	var encW *io.PipeWriter
	var encR *io.PipeReader
	if encrypt {
		encR, encW = io.Pipe()
		uploadReader = encR
	} else {
		uploadReader = pgR
	}

	// The upload leg reads through a counter so the heartbeat can report the
	// size of the object that reached the remote.
	counter := &countingReader{r: uploadReader}

	var wg sync.WaitGroup
	errc := make(chan error, 3)

	// ── Stage 1: pg_dump ─────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { _ = pgW.Close() }()
		if err := runPgDump(ctx, cfg, pgURL, pgW); err != nil {
			cancel()
			errc <- fmt.Errorf("pg_dump: %w", err)
		}
	}()

	// ── Stage 2: age encrypt (optional) ──────────────────────────────
	if encrypt {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { _ = encW.Close() }()
			if err := ageEncryptStream(ctx, pgR, encW, recipients); err != nil {
				cancel()
				pgR.CloseWithError(err)
				errc <- fmt.Errorf("age encrypt: %w", err)
			}
		}()
	}

	// ── Stage 3: rclone rcat upload ───────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { _ = uploadReader.Close() }()
		if err := rcloneRcat(ctx, counter, destination, key); err != nil {
			cancel()
			errc <- fmt.Errorf("rclone upload: %w", err)
		}
	}()

	wg.Wait()
	close(errc)

	for e := range errc {
		if e != nil {
			return nil, e
		}
	}

	var full string
	if !strings.HasSuffix(destination, "/") {
		full = destination + "/" + key
	} else {
		full = destination + key
	}

	slog.Info("streaming backup complete", "destination", full, "encrypted", encrypt)

	return &StreamResult{
		BackupID:    key,
		Destination: full,
		Encrypted:   encrypt,
		Bytes:       counter.n.Load(),
	}, nil
}
