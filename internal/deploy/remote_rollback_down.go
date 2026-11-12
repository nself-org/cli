package deploy

import (
	"context"
	"fmt"
	"strings"
)

// downFirstDeploy stops the stack of a failed first deploy (the pre-deploy tar holds no compose file)
// before the restore deletes its compose files; no volume is removed.
func downFirstDeploy(ctx context.Context, o RollbackOptions) error {
	r, rp := o.Remote, o.RemotePath
	out, err := r.Run(ctx, shScript(fmt.Sprintf(`tar -tzf %s | grep -Eq '^(\./)?(docker-compose\.yml|\.nself/compose-files\.txt)$' && echo HAS || echo FIRST`, q(o.Tar))))
	if err != nil || !strings.Contains(string(out), "FIRST") {
		return err
	}
	set, found, err := HostComposeSet(ctx, r, rp)
	if err != nil || !found {
		return err
	}
	emit(o.Out, "  [running] stop the stack the failed first deploy started on %s", r.Label())
	if _, err = r.Run(ctx, shScript(fmt.Sprintf("cd %s && %s down --remove-orphans", q(rp), composeBase(set.Compose, set.EnvManifest)))); err != nil {
		return fmt.Errorf("stopping the failed first deploy's stack on %s: %w", r.Label(), err)
	}
	return nil
}
