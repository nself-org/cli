package controlplane

// remote_apply.go: the v1.5 per-server deploy of the pipeline (D24).
//
// Purpose: in v1.5 mode each server of the env receives the env's artifact set
//          through deploy.ApplyRemote (target snapshot, pre-deploy tar, in-place
//          sync, nginx -t, recreate preview, verified backup on tier prod,
//          gated service-by-service apply) instead of the single-compose
//          DeployViaSsh push. Resolve, Preflight and role order are unchanged.
// Inputs:  a RemoteApply built by the caller after the scratch build.
// Outputs: per-server errors (E485 drift, E403, E206, E250/E251) and, through
//          Record, what each server saved so `deploy rollback` can undo it.
// Constraints: one artifact set per env, mapped to one remote path, so every
//              server of the env must use that path. Promote (P7-DEPL-15)
//              calls deploy.ApplyRemote and VerifiedSnapshot the same way.

import (
	"context"
	"fmt"
	"io"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
)

// RemoteApply is the v1.5 payload for one env.
type RemoteApply struct {
	Set          deploy.ArtifactSet
	Expected     deploy.TargetSnapshot // the primary's snapshot the build used
	LocalPlugins []deploy.PluginRef
	ProdClass    bool // tier prod: verified backup, stateful recreate needs confirmation
	Yes          bool
	Confirm      func(msg string) bool
	StrictHasura bool
	Clock        docker.Clock
	Out          io.Writer
	// BackupID is the one verified database backup of the env, taken on its
	// primary (BackupHost) before any server was touched; other servers never run
	// `nself backup`. Empty off tier prod.
	BackupID, BackupHost string
	// Previous are the paths the env's previous deploy shipped (local record).
	Previous []string
	// Record is called after each server's apply, whether it succeeded or not.
	Record func(srv Server, res deploy.ApplyResult, err error)
	// Open opens the remote of a server; nil means the strict-host-key ssh
	// remote. Tests substitute a recorder.
	Open func(ctx context.Context, srv Server, keyPath, env string) (deploy.Remote, error)
}

// OpenRemote opens the remote of srv through ra.Open or the ssh default.
func (ra *RemoteApply) OpenRemote(ctx context.Context, srv Server, keyPath, env string) (deploy.Remote, error) {
	if ra.Open != nil {
		return ra.Open(ctx, srv, keyPath, env)
	}
	return deploy.NewSSHRemote(ctx, srv.Host, keyPath, env, srv.Name)
}

// RunWithApply is RunWithEnv for the v1.5 artifact-set apply: composePath is the
// scratch build's compose (Preflight reads its manifest), apply the staged set.
func RunWithApply(ctx context.Context, inv *Inventory, env string, prober Prober, composePath string, apply *RemoteApply) (*DeployResult, error) {
	return runPayload(ctx, inv, env, prober, composePath, payload{compose: composePath, apply: apply})
}

// deployTo is the per-server deploy: ApplyRemote in v1.5 when the payload
// carries an artifact set, DeployViaSsh otherwise.
func deployTo(ctx context.Context, envName string, srv Server, cfg deploy.SSHConfig, pl payload) error {
	// compat.V15(P7-DEPL-24): DeployViaSsh single-compose push -> ApplyRemote artifact-set apply
	if compat.V15() && pl.apply != nil {
		return ApplyServer(ctx, envName, srv, cfg.KeyPath, pl.apply)
	}
	return deployServerFn(ctx, cfg, pl.compose)
}

// ApplyServer applies the env's artifact set to one server: the pipeline's
// per-server step, and the legacy single-host deploy's.
func ApplyServer(ctx context.Context, envName string, srv Server, keyPath string, ra *RemoteApply) error {
	if srv.RemotePath == "" || srv.RemotePath != ra.Set.RemotePath {
		err := errs.New("E485", fmt.Sprintf("environments.%s.servers.%s.remote_path: %q must equal %q, the path the artifact set was built for; every server of an env shares one remote_path in v1.5", envName, srv.Name, srv.RemotePath, ra.Set.RemotePath))
		ra.record(srv, deploy.ApplyResult{}, err)
		return err
	}
	r, err := ra.OpenRemote(ctx, srv, keyPath, envName)
	if err != nil {
		ra.record(srv, deploy.ApplyResult{}, err)
		return err
	}
	expected := ra.Expected
	res, err := deploy.ApplyRemote(ctx, deploy.ApplyOptions{
		Remote: r, RemotePath: srv.RemotePath, Set: ra.Set, Expected: &expected, LocalPlugins: ra.LocalPlugins,
		ProdClass: ra.ProdClass, Yes: ra.Yes, Confirm: ra.Confirm, StrictHasura: ra.StrictHasura, Clock: ra.Clock, Out: ra.Out,
		BackupID: ra.BackupID, Env: envName, Previous: ra.Previous,
	})
	ra.record(srv, res, err)
	return err
}

// record reports a server's outcome to ra.Record, early refusals included, so
// deploy.json holds an outcome for every server the pipeline reached.
func (ra *RemoteApply) record(srv Server, res deploy.ApplyResult, err error) {
	if ra.Record != nil {
		ra.Record(srv, res, err)
	}
}
