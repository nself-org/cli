package deploy

// Purpose: retention for the pre-deploy tars (D-0421). Every v1.5 deploy writes
//          <remote_path>/.nself/backups/pre-deploy-<id>.tar.gz and nothing used
//          to remove one, so a host grew a tar per deploy. After a deploy that
//          fully succeeded, the newest PreDeployTarKeep tars stay, plus the one
//          this deploy's record names, and the older ones are removed.
// Inputs:  a Remote, the remote path, how many to keep, paths to protect.
// Outputs: the names (relative to the backups dir) of the tars it removed.
// Constraints: only regular files named pre-deploy-<id>.tar.gz with an id that
//              matches backupIDRe are candidates (lexical order is time order);
//              restore-*, .list-*, .empty-* entries and everything else in the
//              dir are never touched. A symlink is never removed. Nothing is
//              listed or removed before the backups dir is proven (cd -P) to
//              resolve strictly inside the real remote_path; the proof is
//              repeated in the removing script. keep < 1 is an error, so a
//              caller bug can never remove every tar. A prune failure after a
//              good deploy is a warning, never a deploy failure
//              (pruneOldTars).

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
)

// PreDeployTarKeep is how many pre-deploy tars a host keeps after a successful
// deploy (the protected one is extra).
const PreDeployTarKeep = 5

// backupsDirProof is the shell that resolves the backups dir inside the real
// remote_path or ends the script: nothing is missing -> exit 0 silently, a dir
// that resolves outside (or to remote_path itself) -> OUTSIDE.
const backupsDirProof = `rp=%s; cd -P "$rp" 2>/dev/null || exit 0; base=$(pwd -P)
x=$(cd -P "` + BackupDir + `" 2>/dev/null && pwd -P) || exit 0
case "$x/" in "$base/"?*) ;; *) printf 'OUTSIDE\n'; exit 0;; esac
`

// PrunePreDeployTars removes the pre-deploy tars of remotePath beyond the newest
// keep, except the absolute paths in protect, and returns the removed names.
func PrunePreDeployTars(ctx context.Context, r Remote, remotePath string, keep int, protect ...string) ([]string, error) {
	if keep < 1 {
		return nil, fmt.Errorf("refusing to prune pre-deploy tars on %s: keep must be at least 1, got %d", r.Label(), keep)
	}
	list := fmt.Sprintf(backupsDirProof, q(remotePath)) +
		`cd "$x" || exit 0; for f in pre-deploy-*.tar.gz; do if [ -f "$f" ] && [ ! -L "$f" ]; then printf 'TAR %s\n' "$f"; fi; done`
	out, err := r.Run(ctx, shScript(list))
	if err != nil {
		return nil, fmt.Errorf("listing pre-deploy tars on %s: %w", r.Label(), err)
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "OUTSIDE") {
			return nil, fmt.Errorf("refusing to prune pre-deploy tars on %s: %s resolves outside the project directory", r.Label(), path.Join(remotePath, BackupDir))
		}
		if n, ok := strings.CutPrefix(line, "TAR "); ok && isPreDeployTarName(n) {
			names = append(names, n)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names))) // newest first
	protected := map[string]bool{}
	for _, p := range protect {
		protected[p] = true
	}
	var drop []string
	for i, n := range names {
		if i >= keep && !protected[path.Join(remotePath, BackupDir, n)] {
			drop = append(drop, n)
		}
	}
	if len(drop) == 0 {
		return nil, nil
	}
	rm := fmt.Sprintf(backupsDirProof, q(remotePath)) +
		fmt.Sprintf(`for n in %s; do f="$x/$n"; if [ -f "$f" ] && [ ! -L "$f" ]; then rm -f -- "$f" && printf 'RM %%s\n' "$n"; fi; done`, strings.Join(quoteAll(drop), " "))
	out, err = r.Run(ctx, shScript(rm))
	if err != nil {
		return nil, fmt.Errorf("removing old pre-deploy tars on %s: %w", r.Label(), err)
	}
	asked := map[string]bool{}
	for _, n := range drop {
		asked[n] = true
	}
	var removed []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "OUTSIDE") {
			return nil, fmt.Errorf("refusing to prune pre-deploy tars on %s: %s resolves outside the project directory", r.Label(), path.Join(remotePath, BackupDir))
		}
		if n, ok := strings.CutPrefix(line, "RM "); ok && asked[n] {
			removed = append(removed, n)
		}
	}
	return removed, nil
}

// isPreDeployTarName reports whether name is exactly pre-deploy-<id>.tar.gz
// with an id NewBackupID could have made.
func isPreDeployTarName(name string) bool {
	id, ok := strings.CutPrefix(name, "pre-deploy-")
	if !ok {
		return false
	}
	id, ok = strings.CutSuffix(id, ".tar.gz")
	return ok && backupIDRe.MatchString(id)
}

// pruneOldTars is the step ApplyRemote runs after a fully successful apply:
// keep the newest PreDeployTarKeep tars and protect (this deploy's own, which
// the deploy record names). A failure is a warning only.
func pruneOldTars(ctx context.Context, o ApplyOptions, protect string) {
	removed, err := PrunePreDeployTars(ctx, o.Remote, o.RemotePath, PreDeployTarKeep, protect)
	if err != nil {
		emit(o.Out, "  [warn] could not prune old pre-deploy tars on %s: %v", o.Remote.Label(), err)
		return
	}
	for _, n := range removed {
		emit(o.Out, "  [pruned] %s (old pre-deploy tar)", n)
	}
}
