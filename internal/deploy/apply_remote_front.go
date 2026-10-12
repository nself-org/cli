package deploy

// Purpose: the fronting-stack half of ApplyRemote (D-0425 apply side). A project
//          fronted by another stack's nginx (NGINX_FRONTED_BY) ships its
//          per-service nginx/sites/*.conf into that stack's directory, the second
//          artifact root ArtifactSet.Front (remote path = path.Dir(remote_path)).
// Inputs:  ApplyOptions (the set's Front, the record of what the previous deploy
//          shipped there, the expected snapshot).
// Outputs: a *frontApply the pipeline steps call (nil when there is no front work,
//          every method is nil-safe), and FrontRecord fields on ApplyResult.
// Constraints: only nginx/sites/<name>.conf files move; the fronting stack's .env*,
//              compose files and ssl/ are never read, tarred, synced or removed.
//              Guards run before the lock (E485, nothing sent): the real front root
//              is the parent of the real remote_path, ResolveDirs, no unmarked file
//              overwritten, the nginx container resolved by labels and mounted.
//              The front tar lives under the project's own .nself/backups and holds
//              the shipped paths plus the earlier-shipped paths a prune may remove.
//              The sync has no --delete; a front file is pruned only when this
//              project's previous deploy shipped it (record) and it carries the
//              marker. A failure before the first `up` restores both roots in place.

import (
	"context"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/nself-org/cli/internal/errs"
)

// FrontRecord is what ApplyResult carries about the front root: its pre-deploy
// tar and the paths shipped or pruned there (Absent: the shipped ones the host
// did not have). Empty when the deploy touched no front root.
type FrontRecord struct {
	FrontTar     string
	FrontChanged []string
	FrontAbsent  []string
}

// frontPathRe is the only shape of path that ships from or is pruned in the front root.
var frontPathRe = regexp.MustCompile(`^nginx/sites/[A-Za-z0-9][A-Za-z0-9._-]*\.conf$`)

func frontPathOK(p string) bool { return frontPathRe.MatchString(p) && pruneable(p) }

// frontApply is the front root of one apply.
type frontApply struct {
	fr      *FrontRoot
	ctr     frontContainer
	prev    []string // earlier-shipped paths from the local record, validated
	tar     string
	changed []string
	absent  []string
	synced  bool
}

// prepareFront runs the front guards. It returns nil, nil when the set has no
// front work (no files and nothing earlier to prune).
func prepareFront(ctx context.Context, o ApplyOptions) (*frontApply, error) {
	fr := o.Set.Front
	if fr == nil {
		return nil, nil
	}
	fa := &frontApply{fr: fr}
	for _, p := range fr.Previous {
		if frontPathOK(p) {
			fa.prev = append(fa.prev, p)
		}
	}
	if len(fr.Files) == 0 && len(fa.prev) == 0 {
		return nil, nil
	}
	for _, f := range fr.Files {
		if !frontPathOK(f) {
			return nil, errs.New("E485", fmt.Sprintf("the front root holds an unsafe path %q; only nginx/sites/<name>.conf may ship there; nothing was sent", safeText(f)))
		}
	}
	r, rp, front := o.Remote, o.RemotePath, fr.RemotePath
	if front != path.Dir(rp) || front == "/" || !path.IsAbs(front) {
		return nil, errs.New("E485", fmt.Sprintf("the front root %q is not the directory of remote_path %q; nothing was sent", safeText(front), safeText(rp)))
	}
	realFront, err := checkFrontParent(ctx, r, front, rp)
	if err != nil {
		return nil, err
	}
	if _, err = ResolveDirs(ctx, r, front, []string{"nginx", "nginx/sites"}); err != nil {
		return nil, err
	}
	if err = CheckNotOverwritingForeign(ctx, r, front, fr.Files, fa.prev); err != nil {
		return nil, err
	}
	if o.Expected != nil {
		got, err := TakeFrontSSL(ctx, r, front)
		if err != nil {
			return nil, err
		}
		if err = DriftError(frontSSLDrift(o.Expected.FrontSSL, got)); err != nil {
			return nil, err
		}
	}
	if fa.ctr, err = resolveFrontContainer(ctx, r, front, realFront); err != nil {
		return nil, err
	}
	if err = fa.ctr.checkMounts(ctx, r, front, realFront); err != nil {
		return nil, err
	}
	return fa, nil
}

// checkFrontParent proves the real front root is the parent of the real
// remote_path (E485 otherwise) and returns the real front root. A remote_path that
// does not exist yet counts as its real parent plus its name.
func checkFrontParent(ctx context.Context, r Remote, front, rp string) (string, error) {
	script := fmt.Sprintf(`f=$(cd -P %s 2>/dev/null && pwd -P) || { echo NOFRONT; exit 0; }
if [ -d %s ]; then p=$(cd -P %s && pwd -P); else d=$(cd -P %s 2>/dev/null && pwd -P) && p="$d/$(basename %s)"; fi
printf 'FRONT\t%%s\nRP\t%%s\n' "$f" "$p"`, q(front), q(rp), q(rp), q(path.Dir(rp)), q(rp))
	out, err := r.Run(ctx, shScript(script))
	if err != nil {
		return "", fmt.Errorf("resolving the front root on %s: %w", r.Label(), err)
	}
	var realFront, realRP string
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "FRONT\t"); ok {
			realFront = v
		} else if v, ok := strings.CutPrefix(line, "RP\t"); ok {
			realRP = v
		}
	}
	bad := realFront == "" || realRP == "" || strings.ContainsFunc(realFront+realRP, unicode.IsControl) || path.Dir(realRP) != realFront
	if bad {
		return "", errs.New("E485", fmt.Sprintf("the fronting stack's directory %s must be the parent of remote_path %s on %s (real paths: %q and %q); nothing was sent",
			safeText(front), safeText(rp), r.Label(), safeText(realFront), safeText(realRP))).
			WithFix("lay the project out as <fronting stack dir>/<project> without symlinks that move it elsewhere, then deploy again")
	}
	return realFront, nil
}

// makeTar takes the front pre-deploy tar right after the main one.
func (fa *frontApply) makeTar(ctx context.Context, o ApplyOptions, id string, res *ApplyResult) error {
	if fa == nil {
		return nil
	}
	paths := append(append([]string{}, fa.fr.Files...), fa.prev...)
	seen := map[string]bool{}
	var uniq []string
	for _, p := range paths {
		if !seen[p] {
			seen[p] = true
			uniq = append(uniq, p)
		}
	}
	if !backupIDRe.MatchString(id) {
		return fmt.Errorf("invalid pre-deploy backup id %q", id)
	}
	if err := checkPaths("the front path list", uniq); err != nil {
		return err
	}
	tar := path.Join(o.RemotePath, BackupDir, "pre-deploy-"+id+"-front.tar.gz")
	script := fmt.Sprintf(`set -e; umask 077; out=%s; mkdir -p "$(dirname "$out")"; cd %s; set --
for p in %s; do if [ -e "$p" ]; then set -- "$@" "$p"; fi; done
if [ "$#" -eq 0 ]; then e="$out.empty"; mkdir -p "$e"; tar -czf "$out" -C "$e" .; rmdir "$e"; else tar -hczf "$out" "$@"; fi
tar -tzf "$out" >/dev/null
for p in %s; do if [ ! -e "$p" ] && [ ! -L "$p" ]; then printf 'ABSENT %%s\n' "$p"; fi; done`,
		q(tar), q(fa.fr.RemotePath), strings.Join(quoteAll(uniq), " "), strings.Join(quoteAll(fa.fr.Files), " "))
	out, err := o.Remote.Run(ctx, shScript(script))
	if err != nil {
		return fmt.Errorf("front pre-deploy backup on %s failed, nothing was sent: %w", o.Remote.Label(), err)
	}
	inFiles := map[string]bool{}
	for _, f := range fa.fr.Files {
		inFiles[f] = true
	}
	for _, l := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(l, "ABSENT "); ok && inFiles[p] {
			fa.absent = append(fa.absent, p)
		}
	}
	fa.tar, fa.changed = tar, append([]string{}, fa.fr.Files...)
	res.FrontTar, res.FrontChanged, res.FrontAbsent = fa.tar, fa.changed, fa.absent
	return nil
}

// sync prunes the front files this project shipped before and no longer does,
// then copies the new ones in place (no --delete).
func (fa *frontApply) sync(ctx context.Context, o ApplyOptions, res *ApplyResult) error {
	if fa == nil {
		return nil
	}
	r, front := o.Remote, fa.fr.RemotePath
	emit(o.Out, "  [running] sync the fronting stack's nginx/sites to %s:%s", r.Label(), front)
	fa.synced = true
	keep := map[string]bool{}
	for _, f := range fa.fr.Files {
		keep[f] = true
	}
	var cand []string
	for _, p := range fa.prev {
		if !keep[p] {
			cand = append(cand, p)
		}
	}
	pruned, err := PruneMarked(ctx, r, front, cand)
	for _, f := range pruned {
		emit(o.Out, "  [pruned] %s/%s (generated file the new set dropped)", front, f)
		fa.changed = append(fa.changed, f)
	}
	res.FrontChanged = fa.changed
	if err != nil {
		return err
	}
	if len(fa.fr.Files) == 0 {
		return nil
	}
	return rsyncList(ctx, r, ArtifactSet{StageDir: fa.fr.StageDir, RemotePath: front}, fa.fr.Files, nil)
}

// restore puts the front root back in place (changed restored, absent removed).
// rerr is the main root's restore result; the returned error combines both.
func (fa *frontApply) restore(ctx context.Context, o ApplyOptions, rerr error) error {
	if fa == nil || !fa.synced {
		return rerr
	}
	err := restoreIn(ctx, o.Remote, fa.fr.RemotePath, o.RemotePath, fa.tar, RestoreSpec{Changed: fa.changed, Absent: fa.absent})
	if err == nil {
		fa.synced = false
		return rerr
	}
	err = fmt.Errorf("restoring the fronting stack's files from %s: %w", fa.tar, err)
	if rerr != nil {
		return fmt.Errorf("%v; %w", rerr, err)
	}
	return err
}

// checkNginx is step 8: nginx -t in the live fronting container for a fronted
// project, the project model's throwaway-container check otherwise.
func (fa *frontApply) checkNginx(ctx context.Context, o ApplyOptions, model ComposeModel) error {
	if fa == nil {
		return CheckNginx(ctx, o.Remote, o.RemotePath, model, o.Out)
	}
	emit(o.Out, "  [running] nginx -t in the fronting stack's nginx container")
	return fa.ctr.nginxTest(ctx, o.Remote)
}

// reload is step 12 for the front: nginx -s reload in the fronting container.
func (fa *frontApply) reload(ctx context.Context, w io.Writer, r Remote) error {
	if fa == nil {
		return nil
	}
	emit(w, "  [running] nginx -s reload in the fronting stack's nginx container")
	return fa.ctr.nginxReload(ctx, r)
}
