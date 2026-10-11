package commands

// Purpose: the cmd side of the v1.5 remote apply (D24; P7-DEPL-24). One flow for
//          the control-plane pipeline and the legacy single-host deploy: read the
//          target snapshot of the primary, refuse plugin drift, build in a
//          scratch copy, map paths, stage the artifact set, then apply it per
//          server through controlplane (deploy.ApplyRemote). Rollback restores
//          the pre-deploy tar recorded in .nself/state/deploy.json.
// Inputs:  workdir, target env, the primary server, --yes (from the context).
// Outputs: a controlplane.RemoteApply and the scratch compose path; deploy.json
//          records backup_id, backup_host and pre_deploy_tar.
// Constraints: the operator's project tree is read only; every write goes to the
//              scratch dir or the host. v1.4 mode never reaches this file's
//              apply code (each entry point checks compat.V15()).

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/remote"

	"github.com/spf13/cobra"
)

// deployRemoteOpen opens a server's remote. nil means the strict-host-key ssh
// remote; tests set it to a recorder.
var deployRemoteOpen func(ctx context.Context, srv controlplane.Server, keyPath, env string) (deploy.Remote, error)

// deployGateClock is the health gate's clock; nil means the system clock.
var deployGateClock docker.Clock

type deployYesKey struct{}

// withDeployYes stores cmd's --yes in ctx for the legacy push,
// which has no cmd.
func withDeployYes(cmd *cobra.Command, ctx context.Context) context.Context {
	yes, _ := cmd.Flags().GetBool("yes")
	return context.WithValue(ctx, deployYesKey{}, yes)
}

func deployYes(ctx context.Context) bool { y, _ := ctx.Value(deployYesKey{}).(bool); return y }

func openDeployRemote(ctx context.Context, srv controlplane.Server, keyPath, env string) (deploy.Remote, error) {
	if deployRemoteOpen != nil {
		return deployRemoteOpen(ctx, srv, keyPath, env)
	}
	return deploy.NewSSHRemote(ctx, srv.Host, keyPath, env, srv.Name)
}

// v15Params are the inputs of prepareRemoteApply.
type v15Params struct {
	workdir, target, keyPath string
	srv                      controlplane.Server
	prodClass, yes, jsonOut  bool
}

func deployOut(jsonOut bool) io.Writer {
	if jsonOut {
		return io.Discard
	}
	return os.Stdout
}

// prepareRemoteApply runs steps 1-3 of D24 and returns the apply payload and the
// scratch compose path: target snapshot and drift refusal, scratch build, path
// map and staging.
func prepareRemoteApply(ctx context.Context, p v15Params) (*controlplane.RemoteApply, string, error) {
	rp := p.srv.RemotePath
	if rp == "" {
		return nil, "", errs.New("E485", fmt.Sprintf("server %s has no remote_path; a v1.5 remote deploy needs the project directory on the host", p.srv.Name))
	}
	if err := deploy.ValidateRemotePath(rp); err != nil {
		return nil, "", err
	}
	out := deployOut(p.jsonOut)
	r, err := openDeployRemote(ctx, p.srv, p.keyPath, p.target)
	if err != nil {
		return nil, "", err
	}
	_, _ = fmt.Fprintf(out, "  [running] target snapshot of %s (read-only)\n", p.srv.Name)
	snap, err := deploy.TakeTargetSnapshot(ctx, r, rp)
	if err != nil {
		return nil, "", fmt.Errorf("deploy: %w; nothing was sent", err)
	}
	local, err := localPluginRefs()
	if err != nil {
		return nil, "", err
	}
	if err := deploy.DriftError(deploy.PluginDrift(local, snap)); err != nil {
		return nil, "", err
	}
	_, _ = fmt.Fprintf(out, "  [running] scratch build in %s\n", scratchDirFor(p.workdir, p.target))
	scratch, err := runScratchBuild(ctx, p.workdir, p.target, snap)
	if err != nil {
		return nil, "", err
	}
	set, err := stageScratch(p, scratch)
	if err != nil {
		return nil, "", err
	}
	backupID, backupHost := "", ""
	if p.prodClass {
		// One verified database backup per env, on its primary, before any server is touched.
		_, _ = fmt.Fprintf(out, "  [running] verified database backup on %s\n", p.srv.Name)
		if backupID, err = controlplane.VerifiedSnapshot(ctx, r, rp, "pre-deploy-"+deploy.NewBackupID(time.Now())); err != nil {
			return nil, "", err
		}
		backupHost = r.Label()
	}
	ra := &controlplane.RemoteApply{
		BackupID: backupID, BackupHost: backupHost, Previous: previousShipped(p.workdir, p.target),
		Set: set, Expected: snap, LocalPlugins: local, ProdClass: p.prodClass, Yes: p.yes, Confirm: func(msg string) bool { return deployConfirmPrompt(ctx, msg) },
		StrictHasura: hasuraStrictFor(p.workdir, p.target), Out: out, Clock: deployGateClock,
		Open: func(c context.Context, s controlplane.Server, k, e string) (deploy.Remote, error) {
			return openDeployRemote(c, s, k, e)
		},
		Record: recordRemoteApply(p.workdir, p.target, time.Now(), backupID, backupHost),
	}
	return ra, filepath.Join(scratch, "docker-compose.yml"), nil
}

// recordRemoteApply writes deploy.json as servers finish, but only once some
// server kept changes (refused or restored servers alone leave the last good
// record). Success and per-host outcomes cover every server seen, so a later
// refused server makes the env success=false. Servers that kept changes add their
// tar and change and absent lists per host; the primary supplies pre_deploy_tar.
func recordRemoteApply(workdir, target string, start time.Time, backupID, backupHost string) func(controlplane.Server, deploy.ApplyResult, error) {
	st := deploy.StrategyState{Strategy: deploy.StrategyRolling, Target: target, StartedAt: start, Success: true, HostOutcomes: map[string]string{}}
	var failures []string
	return func(srv controlplane.Server, res deploy.ApplyResult, err error) {
		st.HostOutcomes[srv.Name] = "ok"
		if err != nil {
			st.HostOutcomes[srv.Name] = map[bool]string{true: "failed: ", false: "not changed: "}[res.Synced] + err.Error()
			failures = append(failures, srv.Name+": "+err.Error())
			st.Success, st.Error = false, strings.Join(failures, "; ")
		}
		if res.Touched && backupID != "" {
			st.BackupID, st.BackupHost = backupID, backupHost // only a deploy that ran `up` may offer the database restore
		}
		if res.Synced {
			if st.PreDeployTars == nil {
				st.PreDeployTars, st.PreDeployChanged, st.PreDeployAbsent = map[string]string{}, map[string][]string{}, map[string][]string{}
			}
			st.PreDeployTars[res.BackupHost] = res.PreDeployTar
			st.PreDeployChanged[res.BackupHost], st.PreDeployAbsent[res.BackupHost] = res.Changed, res.Absent
			if srv.Primary || st.PreDeployTar == "" {
				st.PreDeployTar = res.PreDeployTar
				if backupHost == "" {
					st.BackupHost = res.BackupHost
				}
			}
		}
		if len(st.PreDeployTars) == 0 {
			return
		}
		st.CompletedAt = time.Now()
		if serr := deploy.SaveDeployState(workdir, st); serr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: could not save the deploy record: %v\n", serr)
		}
	}
}

// previousShipped is the union of the paths the target's last recorded deploy
// shipped or pruned, on every host: what a prune may remove.
func previousShipped(workdir, target string) []string {
	st, _ := deploy.LoadDeployState(workdir)
	var out []string
	if st.Target == target {
		for _, paths := range st.PreDeployChanged {
			out = append(out, paths...)
		}
	}
	return out
}

// resolveDeployEnvSnapshot is writeResolvedDeployEnv for the pipeline: v1.5
// writes the snapshot inside the scratch dir later, so nothing lands in the
// operator's tree here.
func resolveDeployEnvSnapshot(workdir, envName string) (string, func(), error) {
	// compat.V15(P7-DEPL-24): env snapshot written into the operator's project dir -> written in the scratch dir at apply time
	if compat.V15() {
		return "", func() {}, nil
	}
	return writeResolvedDeployEnv(workdir, envName)
}

// primaryServer picks the server the build is snapshotted against.
func primaryServer(inv *controlplane.Inventory, env string) (controlplane.Server, bool) {
	e, ok := inv.Environments[env]
	if !ok || len(e.Servers) == 0 {
		return controlplane.Server{}, false
	}
	for _, s := range e.Servers {
		if s.Primary {
			return s, true
		}
	}
	for _, s := range e.Servers {
		if s.Role == controlplane.RoleApp {
			return s, true
		}
	}
	return e.Servers[0], true
}

// runDeployPipeline runs the control-plane pipeline: v1.4 pushes the generated
// compose to every server, v1.5 applies the artifact set built in a scratch copy.
func runDeployPipeline(cmd *cobra.Command, workdir string, inv *controlplane.Inventory, target string, prober controlplane.Prober, composePath, envFile, envName string, jsonOut bool) (*controlplane.DeployResult, error) {
	ctx := cmd.Context()
	// compat.V15(P7-DEPL-24): RunWithEnv single-compose push -> target snapshot, scratch build, artifact-set apply
	if !compat.V15() || target == "local" {
		return controlplane.RunWithEnv(ctx, inv, target, prober, composePath, envFile, envName)
	}
	ctx, stop := deploy.WithInterrupt(ctx)
	defer stop()
	srv, ok := primaryServer(inv, target)
	if !ok {
		return nil, errs.New("E483", fmt.Sprintf("invalid target %q: no servers to deploy to", target))
	}
	ra, scratchCompose, err := prepareRemoteApply(ctx, v15Params{
		workdir: workdir, target: target, srv: srv, keyPath: os.Getenv(srv.SSHKeyRef),
		prodClass: prodClassFn(inv, target), yes: deployYes(ctx), jsonOut: jsonOut,
	})
	if err != nil {
		return nil, err
	}
	return controlplane.RunWithApply(ctx, inv, target, prober, scratchCompose, ra)
}

// remoteDeployPushDispatch is the legacy single-host push: v1.4 keeps
// remoteDeployPush, v1.5 goes through the same snapshot, scratch build and apply.
func remoteDeployPushDispatch(ctx context.Context, workdir, host, target string, jsonOut bool) error {
	// compat.V15(P7-DEPL-24): remoteDeployPush single-compose rsync + pull + up -> artifact-set apply
	if compat.V15() {
		return remoteDeployV15(ctx, workdir, host, target, jsonOut)
	}
	return remoteDeployPush(ctx, workdir, host, target, jsonOut)
}

func remoteDeployV15(ctx context.Context, workdir, host, target string, jsonOut bool) error {
	ctx, stop := deploy.WithInterrupt(ctx)
	defer stop()
	key := sshKeyPath()
	if key != "" && !sshKeyRe.MatchString(key) {
		return fmt.Errorf("NSELF_DEPLOY_SSH_KEY contains unsafe characters (got %q): only [a-zA-Z0-9/_.~-] allowed", key)
	}
	spec, err := remote.ParseHostSpec(host)
	if err != nil {
		return fmt.Errorf("NSELF_DEPLOY_HOST_%s: %w", strings.ToUpper(target), err)
	}
	inv, err := controlplane.Load(workdir)
	if err != nil {
		return err
	}
	rp := spec.LegacyPath
	if rp == "" {
		rp = os.Getenv("NSELF_REMOTE_PATH_" + strings.ToUpper(target))
	}
	if rp == "" {
		rp = "/opt/nself"
	}
	if !remotePathRe.MatchString(rp) {
		return fmt.Errorf("NSELF_DEPLOY_HOST_%s remote path contains unsafe characters (got %q): only [a-zA-Z0-9/_.-] allowed", strings.ToUpper(target), rp)
	}
	srv := controlplane.Server{Name: target + "-app", Role: controlplane.RoleApp, Host: spec.String(), RemotePath: rp, Primary: true}
	for _, e := range inv.Environments {
		for _, c := range e.Servers {
			if c.Host == spec.String() {
				srv.Name = c.Name
			}
		}
	}
	ra, scratchCompose, err := prepareRemoteApply(ctx, v15Params{
		workdir: workdir, target: target, srv: srv, keyPath: key, prodClass: prodClassFn(inv, target), yes: deployYes(ctx), jsonOut: jsonOut,
	})
	if err != nil {
		return err
	}
	files, err := controlplane.ComposeFilesForDeploy(scratchCompose)
	if err != nil {
		return err
	}
	one := &controlplane.Inventory{Environments: map[string]controlplane.Environment{target: {Name: target, Kind: "remote", Servers: []controlplane.Server{srv}}}}
	if err := controlplane.Preflight(ctx, one, target, files); err != nil {
		return err
	}
	return controlplane.ApplyServer(ctx, target, srv, key, ra)
}
