package backup

// stream_restore_remote.go — RestoreFromRemote.
//
// Purpose: restore a backup streamed from a remote destination through a scratch database swapped in only after pg_restore exits 0 under the strict scratchPgRestore rule (restoreViaScratch, D-0663), never in place (C2-03).
// Inputs: the remote backup location, an optional age identity and cfg.
// Outputs: a restored database, or an error naming the failed stage; the live database is untouched on failure.
// Constraints: pg_restore runs in the postgres container (internal/docker): the host needs docker, rclone for rclone remotes, age for .age objects, no pg_restore.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/backup/destinations"
	"github.com/nself-org/cli/internal/backup/pgstream"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/errs"
)

// RestoreFromRemote decrypts and restores a backup directly from a remote URL.
// It streams: rclone cat (or the native reader) | age -d | pg_restore into a
// scratch database, then swaps the scratch database in. No local temp file.
func RestoreFromRemote(ctx context.Context, cfg *config.Config, from, keyPath string) error {
	if from == "" {
		return fmt.Errorf("--from destination required")
	}

	encrypted := strings.HasSuffix(from, ".age")
	if encrypted && keyPath == "" {
		// compat.V15(P7-PROD-08): age-key.txt only -> shared identity search, E223 when none
		if compat.V15() {
			var err error
			if keyPath, err = DefaultIdentity(cfg.ProjectName, "--key"); err != nil {
				return err
			}
		} else {
			keyPath = filepath.Join(os.Getenv("HOME"), ".config", "nself", "age-key.txt")
		}
	}

	// Native destinations (path://, host://) stream through the Destination
	// interface; only rclone remotes need the rclone binary.
	var native io.ReadCloser
	if destinations.IsNativeKey(from) {
		var err error
		if native, err = openNative(ctx, from); err != nil {
			return err
		}
	}

	// Check binaries. pg_restore runs inside the postgres container.
	binaries := []string{"docker", "rclone"}
	if native != nil {
		binaries = []string{"docker"}
	}
	if encrypted {
		binaries = append(binaries, "age")
	}
	for _, bin := range binaries {
		if _, err := exec.LookPath(bin); err != nil {
			if native != nil {
				_ = native.Close()
			}
			return fmt.Errorf("required binary %q not found: %w", bin, err)
		}
	}

	container := cfg.ProjectName + "_postgres"
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}
	consumed := false
	err := restoreViaScratch(ctx, container, user, db, func(ctx context.Context, scratch string) error {
		consumed = true
		load := func(ctx context.Context, r io.Reader) error {
			// Create the roles the archive's policies name (NOLOGIN) before the
			// load, from a buffered prefix: one pass, no temp file (P7-PROD-88).
			r, err := ensurePolicyRolesStream(ctx, &Container{Name: container, User: user, DB: scratch}, r)
			if err != nil {
				return err
			}
			return scratchPgRestore(ctx, container, user, scratch, r) // strict: any non-zero exit fails before the swap
		}
		return pgstream.Stream(ctx, pgstream.Source{From: from, Native: native, AgeKey: keyPath, Encrypted: encrypted,
			Load: load, RemoteErr: errs.ErrBackupRemoteFailed, DecryptErr: errs.ErrBackupDecryptFailed})
	})
	if native != nil && !consumed {
		_ = native.Close()
	}
	if err != nil {
		return err
	}
	slog.Info("remote restore complete", "from", from)
	return nil
}

// openNative opens the object a path:// or host:// URI names.
func openNative(ctx context.Context, from string) (io.ReadCloser, error) {
	var inv *destinations.Inventory
	if destinations.KindOf(from) == destinations.KindHost {
		var err error
		if inv, err = controlplane.Load("."); err != nil {
			return nil, err
		}
	}
	dest, key, err := destinations.ParseObject(from, inv)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrBackupNotFound, err)
	}
	op, ok := dest.(destinations.Opener)
	if !ok {
		return nil, fmt.Errorf("%w: %s destinations cannot be streamed", errs.ErrBackupRemoteFailed, dest.Kind())
	}
	rc, err := op.Open(ctx, key)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %v", errs.ErrBackupNotFound, err)
		}
		return nil, fmt.Errorf("%w: %v", errs.ErrBackupRemoteFailed, err)
	}
	return rc, nil
}
