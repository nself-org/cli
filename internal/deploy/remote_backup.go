package deploy

// Purpose: the pre-deploy tar (D24 step 4) and its in-place restore. On every
//          tier, before any sync, the host tars the compose files, env files,
//          plugin fragments, nginx/ and hasura/ it has into
//          <remote_path>/.nself/backups/pre-deploy-<id>.tar.gz; a rollback
//          extracts it to a temp dir and rsyncs it back over the live tree.
// Inputs:  a Remote and the remote project path.
// Outputs: the backup id and tar path; the files the restore pruned.
// Constraints: restore uses rsync -ac: the checksum compare matters because tar
//              keeps whole-second mtimes, so a same-size file rewritten in the
//              same second would otherwise be skipped. Nothing is moved or
//              replaced as a directory, so a bind-mounted
//              nginx/ keeps its inode. Restore rsyncs without --delete, then
//              removes only generated-marker files the tar does not hold
//              (the same rule as the artifact prune). conf.d*/ is excluded
//              from the tar, so a restore never writes it. A first deploy
//              with nothing to save still gets a valid (empty) tar.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
)

// BackupDir is where pre-deploy tars live, relative to the remote path.
const BackupDir = ".nself/backups"

var backupIDRe = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}$`)

// PreDeployBackup is one saved tar.
type PreDeployBackup struct {
	ID  string
	Tar string // absolute path on the host
}

// NewBackupID returns a UTC timestamp plus six random hex characters.
func NewBackupID(now time.Time) string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return now.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:])
}

// tarSources are the paths (relative to the remote path) the tar takes when
// they exist; .env.* and plugin fragment files are added by the script.
var tarSources = []string{
	"docker-compose.yml", "docker-compose.override.yml", "nself-compose.yml", ".env",
	".nself/compose-files.txt", ".nself/compose-env-files.txt", ".nself/compose.env", "nginx", "hasura",
}

// CreatePreDeployTar tars the host's current deployable files and checks the
// archive reads back.
func CreatePreDeployTar(ctx context.Context, r Remote, remotePath, id string) (PreDeployBackup, error) {
	if !backupIDRe.MatchString(id) {
		return PreDeployBackup{}, fmt.Errorf("invalid pre-deploy backup id %q", id)
	}
	tar := path.Join(remotePath, BackupDir, "pre-deploy-"+id+".tar.gz")
	script := fmt.Sprintf(`set -e; umask 077; rp=%s; out=%s; mkdir -p "$rp/%s"; cd "$rp"; set --
for p in %s; do if [ -e "$p" ]; then set -- "$@" "$p"; fi; done
for f in .env.*; do if [ -f "$f" ]; then set -- "$@" "$f"; fi; done
if [ -d .nself/plugins ]; then for f in $(find .nself/plugins -type f -name '*.yml'); do set -- "$@" "$f"; done; fi
if [ "$#" -eq 0 ]; then e="$rp/%s/.empty-%s"; mkdir -p "$e"; tar -czf "$out" -C "$e" .; rmdir "$e"; else tar -czf "$out" --exclude='conf.d*' "$@"; fi
tar -tzf "$out" >/dev/null`,
		q(remotePath), q(tar), BackupDir, strings.Join(quoteAll(tarSources), " "), BackupDir, id)
	if _, err := r.Run(ctx, shScript(script)); err != nil {
		return PreDeployBackup{}, fmt.Errorf("pre-deploy backup on %s failed, nothing was sent: %w", r.Label(), err)
	}
	return PreDeployBackup{ID: id, Tar: tar}, nil
}

// RestorePreDeployTar puts the tar's files back over the live tree without
// moving any directory, then removes generated files the tar does not hold.
// It returns the pruned files.
func RestorePreDeployTar(ctx context.Context, r Remote, remotePath, tar string) ([]string, error) {
	if !strings.HasPrefix(tar, path.Join(remotePath, BackupDir)+"/pre-deploy-") || strings.Contains(tar, "..") {
		return nil, fmt.Errorf("refusing to restore %q: not a pre-deploy tar under %s", tar, path.Join(remotePath, BackupDir))
	}
	tmp := path.Join(remotePath, BackupDir, "restore-"+strings.TrimSuffix(path.Base(tar), ".tar.gz"))
	listing, err := r.Run(ctx, shScript(fmt.Sprintf(`tar -tzf %s`, q(tar))))
	if err != nil {
		return nil, fmt.Errorf("reading %s on %s: %w", tar, r.Label(), err)
	}
	script := fmt.Sprintf(`set -e; umask 077; tmp=%s; rm -rf "$tmp"; mkdir -p "$tmp"; tar -xzf %s -C "$tmp"; rsync -ac "$tmp"/ %s/; rm -rf "$tmp"`, q(tmp), q(tar), q(remotePath))
	if _, err := r.Run(ctx, shScript(script)); err != nil {
		return nil, fmt.Errorf("restoring %s on %s: %w", tar, r.Label(), err)
	}
	keep := map[string]bool{}
	for _, line := range strings.Split(string(listing), "\n") {
		if f := strings.TrimPrefix(strings.TrimSpace(line), "./"); f != "" {
			keep[f] = true
		}
	}
	return PruneMarked(ctx, r, remotePath, keep)
}
