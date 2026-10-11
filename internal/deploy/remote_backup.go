package deploy

// Purpose: the pre-deploy tar (D24 step 4) and its in-place restore. On every
//          tier, before any sync, the host tars the compose files, env files,
//          plugin fragments, nginx/ and hasura/ it has into
//          <remote_path>/.nself/backups/pre-deploy-<id>.tar.gz; a rollback
//          extracts it to a temp dir and rsyncs it back over the live tree.
// Inputs:  a Remote and the remote project path.
// Outputs: the backup id and tar path; the files the restore pruned.
//          Beside the tar, <tar>.absent lists the artifact-set paths that did not
//          exist at tar time; a restore removes exactly those files (never a
//          dir, never under conf.d*/), so a deploy onto a host without manifests
//          does not leave them behind. The tar is mode 0600 (it holds env files).
// Constraints: restore runs on a context that survives cancellation, and uses rsync -ac: the checksum compare matters because tar
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
	ID     string
	Tar    string // absolute path on the host
	Absent string // <tar>.absent: artifact paths the host did not have
}

// absentPath is the absent list beside tar.
func absentPath(tar string) string { return strings.TrimSuffix(tar, ".tar.gz") + ".absent" }

// restoreTimeout bounds a restore that runs after the caller gave up.
const restoreTimeout = 5 * time.Minute

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
	".nself/compose-files.txt", ".nself/compose-env-files.txt", ".nself/compose.env", ShippedFile, "nginx", "hasura",
}

// CreatePreDeployTar tars the host's current deployable files, records which of
// paths (the artifact set, relative to remotePath) the host does not have yet,
// and checks the archive reads back.
func CreatePreDeployTar(ctx context.Context, r Remote, remotePath, id string, paths []string) (PreDeployBackup, error) {
	if !backupIDRe.MatchString(id) {
		return PreDeployBackup{}, fmt.Errorf("invalid pre-deploy backup id %q", id)
	}
	tar := path.Join(remotePath, BackupDir, "pre-deploy-"+id+".tar.gz")
	script := fmt.Sprintf(`set -e; umask 077; rp=%s; out=%s; abs=%s; mkdir -p "$rp/%s"; cd "$rp"; set --
for p in %s; do if [ -e "$p" ]; then set -- "$@" "$p"; fi; done
for f in .env.*; do if [ -f "$f" ]; then set -- "$@" "$f"; fi; done
l="$rp/%s/.list-%s"; : > "$l"
if [ -d .nself/plugins ]; then find .nself/plugins -type f -name '*.yml' > "$l"; fi
while IFS= read -r f; do set -- "$@" "$f"; done < "$l"; rm -f "$l"
if [ "$#" -eq 0 ]; then e="$rp/%s/.empty-%s"; mkdir -p "$e"; tar -czf "$out" -C "$e" .; rmdir "$e"; else tar -czf "$out" --exclude='conf.d*' "$@"; fi
tar -tzf "$out" >/dev/null
: > "$abs"
for p in %s; do if [ ! -e "$p" ]; then printf '%%s\n' "$p" >> "$abs"; fi; done`,
		q(remotePath), q(tar), q(absentPath(tar)), BackupDir, strings.Join(quoteAll(tarSources), " "), BackupDir, id, BackupDir, id,
		strings.Join(quoteAll(append(append([]string{}, paths...), ShippedFile)), " "))
	if _, err := r.Run(ctx, shScript(script)); err != nil {
		return PreDeployBackup{}, fmt.Errorf("pre-deploy backup on %s failed, nothing was sent: %w", r.Label(), err)
	}
	return PreDeployBackup{ID: id, Tar: tar, Absent: absentPath(tar)}, nil
}

// RestorePreDeployTar puts the tar's files back over the live tree without
// moving any directory, removes the files the deploy created (the absent list),
// then removes generated files the previous deploy shipped that the tar does not
// hold. It returns the removed generated files. It runs on a context that
// survives the caller's cancellation, bounded by restoreTimeout.
func RestorePreDeployTar(ctx context.Context, r Remote, remotePath, tar string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), restoreTimeout)
	defer cancel()
	if !strings.HasPrefix(tar, path.Join(remotePath, BackupDir)+"/pre-deploy-") || strings.Contains(tar, "..") || !strings.HasSuffix(tar, ".tar.gz") {
		return nil, fmt.Errorf("refusing to restore %q: not a pre-deploy tar under %s", tar, path.Join(remotePath, BackupDir))
	}
	tmp := path.Join(remotePath, BackupDir, "restore-"+strings.TrimSuffix(path.Base(tar), ".tar.gz"))
	listing, err := r.Run(ctx, shScript(fmt.Sprintf(`tar -tzf %s`, q(tar))))
	if err != nil {
		return nil, fmt.Errorf("reading %s on %s: %w", tar, r.Label(), err)
	}
	shippedNow, err := remoteLines(ctx, r, path.Join(remotePath, ShippedFile))
	if err != nil {
		return nil, err
	}
	absent, err := remoteLines(ctx, r, absentPath(tar))
	if err != nil {
		return nil, err
	}
	script := fmt.Sprintf(`set -e; umask 077; tmp=%s; rm -rf "$tmp"; mkdir -p "$tmp"; tar -xzf %s -C "$tmp"; rsync -ac "$tmp"/ %s/; rm -rf "$tmp"`, q(tmp), q(tar), q(remotePath))
	if _, err := r.Run(ctx, shScript(script)); err != nil {
		return nil, fmt.Errorf("restoring %s on %s: %w", tar, r.Label(), err)
	}
	if err := removeCreated(ctx, r, remotePath, absent); err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, line := range strings.Split(string(listing), "\n") {
		if f := strings.TrimPrefix(strings.TrimSpace(line), "./"); f != "" {
			keep[f] = true
		}
	}
	for _, f := range absent {
		keep[f] = true // already handled above
	}
	var cand []string
	for _, f := range shippedNow {
		if !keep[f] {
			cand = append(cand, f)
		}
	}
	return PruneMarked(ctx, r, remotePath, cand)
}

// removeCreated removes the regular files the deploy created: the absent list.
// Symlinks, directories and anything under conf.d*/ are left; .env only when empty.
func removeCreated(ctx context.Context, r Remote, remotePath string, absent []string) error {
	var paths []string
	for _, p := range absent {
		if pruneable(p) {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	script := fmt.Sprintf(`cd %s && for f in %s; do if [ -f "$f" ] && [ ! -L "$f" ]; then if [ "$f" = .env ] && [ -s "$f" ]; then :; else rm -f -- "$f"; fi; fi; done`, q(remotePath), strings.Join(quoteAll(paths), " "))
	if _, err := r.Run(ctx, shScript(script)); err != nil {
		return fmt.Errorf("removing files the deploy created on %s: %w", r.Label(), err)
	}
	return nil
}
