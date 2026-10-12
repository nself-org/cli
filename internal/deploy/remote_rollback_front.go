package deploy

// Purpose: the fronting stack's half of `nself deploy rollback` (D-0425). The
//          deploy record keeps the front tar and the front changed/absent lists
//          per host; the rollback restores those files in place (changed paths
//          back from the tar, created paths removed), then runs nginx -t and
//          nginx -s reload in the fronting nginx container.
// Inputs:  RollbackOptions (FrontTar, FrontChanged, FrontAbsent).
// Outputs: a *frontRollback the steps call (nil when the record has no front
//          part, every method is nil-safe), so an older record rolls back as before.
// Constraints: the same guards as the deploy run first, before the lock and any
//              change (real front root is the parent of the real remote_path,
//              ResolveDirs, one mounted running nginx container by labels, E485
//              otherwise). Every recorded path must be nginx/sites/<name>.conf;
//              the recorded lists are local state and are checked again. The
//              container is re-checked running before nginx -t and the reload.
//              The restore is in place: a bind-mounted nginx/ keeps its inode.

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

type frontRollback struct {
	root string
	ctr  frontContainer
}

func prepareFrontRollback(ctx context.Context, o RollbackOptions) (*frontRollback, error) {
	if o.FrontTar == "" || len(o.FrontChanged) == 0 {
		return nil, nil // no front part: a deploy that shipped and pruned nothing there has nothing to restore
	}
	if !strings.HasPrefix(o.FrontTar, path.Join(o.RemotePath, BackupDir)+"/pre-deploy-") || !strings.HasSuffix(o.FrontTar, "-front.tar.gz") || strings.Contains(o.FrontTar, "..") {
		return nil, fmt.Errorf("deploy rollback: the recorded front tar %q is not a front pre-deploy tar under %s; nothing was changed", o.FrontTar, path.Join(o.RemotePath, BackupDir))
	}
	for _, list := range [][]string{o.FrontChanged, o.FrontAbsent} {
		for _, p := range list {
			if !frontPathOK(p) {
				return nil, fmt.Errorf("deploy rollback: the front record holds an unsafe path %q; nothing was changed", safeText(p))
			}
		}
	}
	r, front := o.Remote, path.Dir(o.RemotePath)
	if front == "/" {
		return nil, errs.New("E485", "the front record has no front root; it cannot be rolled back safely (nothing was changed)")
	}
	realFront, err := checkFrontParent(ctx, r, front, o.RemotePath)
	if err != nil {
		return nil, err
	}
	linked, err := ResolveFrontDirs(ctx, r, front, o.FrontChanged)
	if err != nil {
		return nil, err
	}
	ctr, err := resolveFrontContainer(ctx, r, front, realFront)
	if err == nil {
		err = ctr.checkMounts(ctx, r, front, realFront, linked)
	}
	if err != nil {
		return nil, rollbackFix(err, front)
	}
	return &frontRollback{root: front, ctr: ctr}, nil
}

// rollbackFix gives a container refusal the rollback's own fix text: the main root
// is not rolled back while the fronting nginx cannot be tested and reloaded.
func rollbackFix(err error, front string) error {
	var ce *errs.CLIError
	if !errors.As(err, &ce) {
		return err
	}
	return ce.WithFix(fmt.Sprintf("start the fronting stack's nginx (docker compose up -d nginx in %s), then run nself deploy rollback <env> again; nothing was changed", safeText(front)))
}

// restore puts the front files back in place from the recorded tar.
func (fb *frontRollback) restore(ctx context.Context, o RollbackOptions) error {
	if fb == nil {
		return nil
	}
	emit(o.Out, "  [running] restore %s in place in the fronting stack's %s", path.Base(o.FrontTar), fb.root)
	return restoreIn(ctx, o.Remote, fb.root, o.RemotePath, o.FrontTar, RestoreSpec{Changed: o.FrontChanged, Absent: o.FrontAbsent})
}

// reload tests the restored config in the live fronting container, then reloads it.
func (fb *frontRollback) reload(ctx context.Context, o RollbackOptions) error {
	if fb == nil {
		return nil
	}
	emit(o.Out, "  [running] nginx -t in the fronting stack's nginx container")
	if err := fb.ctr.nginxTest(ctx, o.Remote); err != nil {
		return fmt.Errorf("the restored fronting nginx config does not pass nginx -t, nginx was not reloaded: %w", err)
	}
	emit(o.Out, "  [running] nginx -s reload in the fronting stack's nginx container")
	return fb.ctr.nginxReload(ctx, o.Remote)
}
