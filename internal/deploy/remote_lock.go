package deploy

// Purpose: one deploy at a time per host (F13). An atomic `mkdir
//          <remote_path>/.nself/deploy.lock` before the pre-deploy tar; the dir
//          holds a `holder` file (pid, host, user, time, id). A second deploy
//          or rollback finds the dir and refuses, naming the holder.
// Inputs:  a Remote, the remote path, the deploy id.
// Outputs: a release func (safe on a cancelled context), or an error naming the holder.
// Constraints: the lock is removed with rm of its own holder file and rmdir, never
//              a recursive delete. A crashed deploy leaves it behind on purpose;
//              the error says how to remove it by hand once the holder is gone.

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"
)

// LockDir is the lock directory, relative to the remote path.
const LockDir = ".nself/deploy.lock"

// AcquireLock takes the host's deploy lock.
func AcquireLock(ctx context.Context, r Remote, remotePath, id string) (release func(), err error) {
	lock := path.Join(remotePath, LockDir)
	script := fmt.Sprintf(`umask 077; mkdir -p %s && if mkdir %s 2>/dev/null; then printf 'pid=%%s host=%%s user=%%s time=%%s id=%%s\n' "$$" "$(hostname)" "$(id -un)" "$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)" %s > %s/holder; echo OK; else printf 'LOCKED %%s\n' "$(cat %s/holder 2>/dev/null)"; fi`,
		q(path.Join(remotePath, ".nself")), q(lock), q(id), q(lock), q(lock))
	out, err := r.Run(ctx, shScript(script))
	if err != nil {
		return nil, fmt.Errorf("taking the deploy lock on %s: %w", r.Label(), err)
	}
	if got := strings.TrimSpace(string(out)); got != "OK" {
		holder := strings.TrimSpace(strings.TrimPrefix(got, "LOCKED"))
		return nil, fmt.Errorf("another deploy or rollback holds the lock on %s (%s); if that process is gone, remove it with: rm %s/holder && rmdir %s",
			r.Label(), strings.TrimSpace(holder+" "+lock), lock, lock)
	}
	return func() {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		_, _ = r.Run(rctx, shScript(fmt.Sprintf(`rm -f %s/holder; rmdir %s`, q(lock), q(lock))))
	}, nil
}
