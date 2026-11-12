package promote

// Purpose: Share apply bookkeeping and files-first undo between CLI and sim.
// Inputs: A synced ApplyResult or recorded host remotes and paths.
// Outputs: State rollback evidence and restored pre-deploy artifact sets.
// Constraints: Database restore belongs to snapshot undo after all file undos.

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
)

// RecordApplied remembers only hosts whose artifact tree was changed.
func RecordApplied(st *State, res deploy.ApplyResult) {
	if !res.Synced {
		return
	}
	if st.PreDeployTar == nil {
		st.PreDeployTar = map[string]string{}
		st.PreDeployChanged = map[string][]string{}
		st.PreDeployAbsent = map[string][]string{}
	}
	st.PreDeployTar[res.BackupHost] = res.PreDeployTar
	st.PreDeployChanged[res.BackupHost] = res.Changed
	st.PreDeployAbsent[res.BackupHost] = res.Absent
}

// RestartApplied runs the service up and health gate on hosts whose files were
// already restored (files first, then DB restore, then up+health with StrictHasura).
func RestartApplied(ctx context.Context, st *State, remotes map[string]deploy.Remote, paths map[string]string) error {
	hosts := make([]string, 0, len(st.PreDeployTar))
	for host := range st.PreDeployTar {
		if st.FilesRestored != nil && st.FilesRestored[host] {
			hosts = append(hosts, host)
		}
	}
	sort.Strings(hosts)
	for _, host := range hosts {
		r := remotes[host]
		if r == nil {
			return fmt.Errorf("promotion rollback has no remote for %s", host)
		}
		if err := deploy.RollbackRemote(ctx, deploy.RollbackOptions{
			Remote: r, RemotePath: paths[host], Tar: st.PreDeployTar[host],
			Changed: st.PreDeployChanged[host], Absent: st.PreDeployAbsent[host],
			StrictHasura: true, ReapplyOnly: true,
		}); err != nil {
			return err
		}
	}
	return nil
}

// UndoApplied restores files only (no service restart) so the DB can be restored next.
func UndoApplied(ctx context.Context, st *State, remotes map[string]deploy.Remote, paths map[string]string) error {
	hosts := make([]string, 0, len(st.PreDeployTar))
	for host := range st.PreDeployTar {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	for _, host := range hosts {
		r := remotes[host]
		if r == nil {
			return fmt.Errorf("promotion rollback has no remote for %s", host)
		}
		if st.FilesRestored[host] {
			continue
		}
		if err := deploy.RollbackRemote(ctx, deploy.RollbackOptions{
			Remote: r, RemotePath: paths[host], Tar: st.PreDeployTar[host],
			Changed: st.PreDeployChanged[host], Absent: st.PreDeployAbsent[host], FilesOnly: true,
		}); err != nil {
			return err
		}
		if st.FilesRestored == nil {
			st.FilesRestored = map[string]bool{}
		}
		st.FilesRestored[host] = true
		st.RecoveryPhase = "files_restored"
		if st.RecoveryCheckpoint != nil {
			if err := st.RecoveryCheckpoint(st); err != nil {
				return fmt.Errorf("files restored but promotion checkpoint failed: %w", err)
			}
		}
	}
	return nil
}

// AppliedHosts returns the remotes and remote paths, keyed by host label, that
// the CLI apply step recorded in st.Values.
func AppliedHosts(st *State) (map[string]deploy.Remote, map[string]string) {
	remotes, _ := st.Values["apply_remotes"].(map[string]deploy.Remote)
	paths, _ := st.Values["apply_paths"].(map[string]string)
	return remotes, paths
}

// GateApplied is the health half of the deferred apply: after the migration,
// every applied host's services are gated, nginx reloaded and the hasura
// metadata applied (D-0698). Hosts are gated in label order. base, the CLI's
// RemoteApply (may be nil), supplies StrictHasura, Clock and Out.
func GateApplied(ctx context.Context, st *State, base *controlplane.RemoteApply) error {
	remotes, paths := AppliedHosts(st)
	hosts := make([]string, 0, len(remotes))
	for host := range remotes {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	for _, host := range hosts {
		o := deploy.ApplyOptions{Remote: remotes[host], RemotePath: paths[host], Out: io.Discard}
		if base != nil {
			o.StrictHasura, o.Clock = base.StrictHasura, base.Clock
			if base.Out != nil {
				o.Out = base.Out
			}
		}
		set, found, err := deploy.HostComposeSet(ctx, o.Remote, o.RemotePath)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("health check on %s: the host names no compose set under %s", o.Remote.Label(), o.RemotePath)
		}
		o.Set = set
		model, err := deploy.ReadComposeModel(ctx, o.Remote, o.RemotePath, set.Compose, set.EnvManifest)
		if err != nil {
			return err
		}
		if err = deploy.GateServices(ctx, o, model); err != nil {
			return err
		}
	}
	return nil
}

// DeferredApplyStep names run "apply" and gives it the promotion undo: files
// first (UndoApplied), then, after the database restore, the restored stack is
// brought up (RestartApplied, D-0699).
func DeferredApplyStep(run func(context.Context, *State) error) Step {
	return Step{Name: "apply", Run: run,
		Undo: func(ctx context.Context, st *State) error {
			remotes, paths := AppliedHosts(st)
			return UndoApplied(ctx, st, remotes, paths)
		},
		Restart: func(ctx context.Context, st *State) error {
			remotes, paths := AppliedHosts(st)
			return RestartApplied(ctx, st, remotes, paths)
		}}
}

// GatedHealthStep is HealthStep behind GateApplied: the deferred apply's
// service gate runs after the migration and before the doctor (D-0698). base
// returns the CLI's RemoteApply once the build step has made it.
func GatedHealthStep(r deploy.Remote, remotePath string, base func() *controlplane.RemoteApply) Step {
	health := HealthStep(r, remotePath)
	doctor := health.Run
	health.Run = func(ctx context.Context, st *State) error {
		if err := GateApplied(ctx, st, base()); err != nil {
			return err
		}
		return doctor(ctx, st)
	}
	return health
}
