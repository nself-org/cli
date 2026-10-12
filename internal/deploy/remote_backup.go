package deploy

// Purpose: the pre-deploy tar (D24 step 4) and its in-place restore. On every
//          tier, before any sync, the host tars the compose files, env files,
//          plugin fragments, nginx/ and hasura/ it has into
//          <remote_path>/.nself/backups/pre-deploy-<id>.tar.gz. The restore is an
//          exact revert of this deploy's own changes, driven by lists this process
//          recorded locally (RestoreSpec), never by files read back from the host:
//          it extracts the tar to a temp dir, rsyncs back only the changed paths
//          that the tar holds, and removes only the changed paths that were absent
//          at tar time. The host's .env, other projects' files and anything this
//          deploy did not ship are never restored or removed.
// Inputs:  a Remote, the remote project path, the artifact paths, RestoreSpec.
// Outputs: the backup id, tar path and absent list; an error from the restore.
// Constraints: every path is validated (shippable) before use; removal refuses a
//              path with a symlink component; rsync follows a symlinked dir
//              (--keep-dirlinks) and never replaces it. The restore runs on a
//              context that survives cancellation, bounded by restoreTimeout.
//              Nothing is moved or replaced as a directory, so a bind-mounted
//              nginx/ keeps its inode. The tar follows symlinks (-h) so a
//              symlinked nginx/ contributes its contents, not the link.
//              rsync -ac: tar keeps whole-second mtimes.
//              conf.d*/ is excluded from the tree the tar takes. The one exception
//              is a shipped nginx/conf.d/<name>.conf (D-0424, see shippable): it
//              is in the lists, and when the host has it a second step appends it
//              to the tar (tar -r on the unpacked tar, then gzip), so a rollback
//              restores it; when the host lacks it, it is reported ABSENT and a
//              rollback removes the created file. The tar is mode 0600 (it holds
//              env files).

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

// BackupDir is where pre-deploy tars live, relative to the remote path.
const BackupDir = ".nself/backups"

var backupIDRe = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}$`)

// PreDeployBackup is one saved tar.
type PreDeployBackup struct {
	ID     string
	Tar    string   // absolute path on the host
	Absent []string // artifact paths the host did not have at tar time
}

// RestoreSpec says what a restore may touch: Changed are the paths this deploy
// shipped or pruned, Absent the subset that did not exist before it. Both come
// from this process (or the local deploy.json), not from the host.
type RestoreSpec struct {
	Changed []string
	Absent  []string
}

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

// CreatePreDeployTar tars the host's current deployable files, reports which of
// paths (the artifact set, relative to remotePath) the host does not have yet,
// and checks the archive reads back.
func CreatePreDeployTar(ctx context.Context, r Remote, remotePath, id string, paths []string) (PreDeployBackup, error) {
	if !backupIDRe.MatchString(id) {
		return PreDeployBackup{}, fmt.Errorf("invalid pre-deploy backup id %q", id)
	}
	if err := checkPaths("the artifact set", paths); err != nil {
		return PreDeployBackup{}, err
	}
	tar := path.Join(remotePath, BackupDir, "pre-deploy-"+id+".tar.gz")
	// Generated conf.d files are taken in a second step (see the header): the
	// first tar excludes conf.d*, so they are kept out of the first loop.
	var tree, confD []string
	for _, p := range paths {
		if isGeneratedConfD(p) {
			confD = append(confD, p)
		} else {
			tree = append(tree, p)
		}
	}
	appendConfD := ""
	if len(confD) > 0 {
		appendConfD = fmt.Sprintf(`set --
for c in %s; do if [ -f "$c" ]; then set -- "$@" "$c"; fi; done
if [ "$#" -gt 0 ]; then t="$out.tmp"; trap 'rm -f "$t" "$t.gz"' EXIT; gzip -dc "$out" > "$t"; tar -hrf "$t" "$@"; gzip -n -c "$t" > "$t.gz"; mv -f "$t.gz" "$out"; rm -f "$t"; fi`, strings.Join(quoteAll(confD), " "))
	}
	script := fmt.Sprintf(`set -e; umask 077; rp=%s; out=%s; mkdir -p "$rp/%s"; cd "$rp"; set --
for p in %s %s; do if [ -e "$p" ]; then set -- "$@" "$p"; fi; done
for f in .env.*; do if [ -f "$f" ]; then set -- "$@" "$f"; fi; done
l="$rp/%s/.list-%s"; : > "$l"
if [ -d .nself/plugins ]; then find -H .nself/plugins -type f -name '*.yml' > "$l"; fi
while IFS= read -r f; do set -- "$@" "$f"; done < "$l"; rm -f "$l"
if [ "$#" -eq 0 ]; then e="$rp/%s/.empty-%s"; mkdir -p "$e"; tar -czf "$out" -C "$e" .; rmdir "$e"; else tar -hczf "$out" --exclude='conf.d*' "$@"; fi
%s
tar -tzf "$out" >/dev/null
for p in %s; do if [ ! -e "$p" ] && [ ! -L "$p" ]; then printf 'ABSENT %%s\n' "$p"; fi; done`,
		q(remotePath), q(tar), BackupDir, strings.Join(quoteAll(tarSources), " "), strings.Join(quoteAll(tree), " "), BackupDir, id, BackupDir, id,
		appendConfD, strings.Join(quoteAll(paths), " "))
	out, err := r.Run(ctx, shScript(script))
	if err != nil {
		return PreDeployBackup{}, fmt.Errorf("pre-deploy backup on %s failed, nothing was sent: %w", r.Label(), err)
	}
	bk := PreDeployBackup{ID: id, Tar: tar}
	for _, l := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(l, "ABSENT "); ok {
			bk.Absent = append(bk.Absent, p)
		}
	}
	return bk, nil
}

// RestorePreDeployTar reverts this deploy's own changes: it rsyncs back the
// Changed paths the tar holds, then removes the Absent paths. A tampered or
// inconsistent spec (an unsafe path, an Absent path the tar holds) is refused
// before anything is touched. It runs on a context that survives the caller's
// cancellation, bounded by restoreTimeout.
func RestorePreDeployTar(ctx context.Context, r Remote, remotePath, tar string, spec RestoreSpec) error {
	return restoreIn(ctx, r, remotePath, remotePath, tar, spec)
}

// restoreIn is RestorePreDeployTar for a tar kept under home (the project's own
// remote path) that restores paths relative to root: the project itself, or the
// fronting stack's directory for the front tar (apply_remote_front.go).
func restoreIn(ctx context.Context, r Remote, root, home, tar string, spec RestoreSpec) error {
	remotePath := root
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), restoreTimeout)
	defer cancel()
	if !strings.HasPrefix(tar, path.Join(home, BackupDir)+"/pre-deploy-") || strings.Contains(tar, "..") || !strings.HasSuffix(tar, ".tar.gz") {
		return fmt.Errorf("refusing to restore %q: not a pre-deploy tar under %s", tar, path.Join(home, BackupDir))
	}
	if err := checkPaths("the change list", spec.Changed); err != nil {
		return err
	}
	if err := checkPaths("the absent list", spec.Absent); err != nil {
		return err
	}
	isChanged := map[string]bool{}
	for _, c := range spec.Changed {
		isChanged[c] = true
	}
	for _, a := range spec.Absent {
		if !isChanged[a] {
			return fmt.Errorf("the absent list names %q, which this deploy did not ship; refusing to remove it", a)
		}
	}
	listing, err := r.Run(ctx, shScript(fmt.Sprintf(`tar -tzf %s`, q(tar))))
	if err != nil {
		return fmt.Errorf("reading %s on %s: %w", tar, r.Label(), err)
	}
	inTar := map[string]bool{}
	for _, line := range strings.Split(string(listing), "\n") {
		if f := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(line), "./"), "/"); f != "" {
			inTar[f] = true
		}
	}
	for _, a := range spec.Absent {
		if inTar[a] {
			return fmt.Errorf("the absent list names %q but the tar holds it; refusing to remove it", a)
		}
	}
	wasAbsent := map[string]bool{}
	for _, a := range spec.Absent {
		wasAbsent[a] = true
	}
	var restore []string
	for _, c := range spec.Changed {
		switch {
		case inTar[c]:
			restore = append(restore, c)
		case !wasAbsent[c]:
			// Fail closed: the deploy changed this path, the tar does not hold it and it
			// was not created by the deploy, so it cannot be put back.
			return fmt.Errorf("cannot revert %q on %s: it is neither in %s nor recorded as created by the deploy; nothing was restored", c, r.Label(), path.Base(tar))
		}
	}
	tmp := path.Join(home, BackupDir, "restore-"+strings.TrimSuffix(path.Base(tar), ".tar.gz"))
	script := fmt.Sprintf(`set -e; umask 077; tmp=%s; rm -rf "$tmp"; mkdir -p "$tmp"; tar -xzf %s -C "$tmp"`, q(tmp), q(tar))
	script += fmt.Sprintf(`; rp=%s`, q(remotePath))
	// One rsync per directory, over bare file names: no directory is ever
	// transferred, so existing dirs (remote_path included) keep their mode and
	// owner, and the destination is the directory's real path (cd -P), so a
	// symlinked dir on the host is written through and never replaced. rsync >= 3.4
	// refuses to traverse a symlink in the destination path, even with -K.
	byDir := map[string][]string{}
	for _, f := range restore {
		byDir[path.Dir(f)] = append(byDir[path.Dir(f)], path.Base(f))
	}
	dirs := make([]string, 0, len(byDir))
	for d := range byDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	// Nothing is created before the deepest existing ancestor of the destination is
	// proven to resolve inside the real remote_path.
	script += `; base=$(cd -P "$rp" && pwd -P); inrp() { case "$1/" in "$base/"*) return 0;; esac; return 1; }; safe() { case "$1" in *[[:cntrl:]]*) return 1;; esac; return 0; }; refuse() { echo "refusing: a restore directory is outside remote_path or has an unsafe name" >&2; exit 1; }`
	for _, d := range dirs {
		script += fmt.Sprintf(`; d=%s; src="$tmp/$d"; dst="$rp/$d"; a="$dst"; while [ ! -d "$a" ]; do a=$(dirname "$a"); done; x=$(cd -P "$a" && pwd -P) || refuse; safe "$x" && inrp "$x" || refuse; (umask 022; mkdir -p "$dst"); dst=$(cd -P "$dst" && pwd -P && echo X); dst=${dst%%X}; dst=${dst%%?}; safe "$dst" && inrp "$dst" || refuse; printf '%%s\n' %s > "$tmp.list"; rsync -ac --files-from="$tmp.list" "$src"/ "$dst"/; rm -f "$tmp.list"`,
			q(d), strings.Join(quoteAll(byDir[d]), " "))
	}
	script += `; rm -rf "$tmp"`
	if _, err := r.Run(ctx, shScript(script)); err != nil {
		return fmt.Errorf("restoring %s on %s: %w", tar, r.Label(), err)
	}
	return removeCreated(ctx, r, remotePath, spec.Absent)
}

// removeCreated removes the regular files this deploy created, named by the local
// record. A path with a symlink component refuses the whole batch.
func removeCreated(ctx context.Context, r Remote, remotePath string, absent []string) error {
	for len(absent) > 0 {
		n := min(len(absent), 50)
		body := `if [ -f "$2" ]; then rm -f -- "$2"; fi`
		out, err := r.Run(ctx, shScript(guardedScript(remotePath, absent[:n], body)))
		if err != nil {
			return fmt.Errorf("removing files the deploy created on %s: %w", r.Label(), err)
		}
		if err := refusedPaths(out); err != nil {
			return err
		}
		absent = absent[n:]
	}
	return nil
}
