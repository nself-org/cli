package promote

// Purpose: Share apply bookkeeping and files-first undo between CLI and sim.
// Inputs: A synced ApplyResult or recorded host remotes and paths.
// Outputs: State rollback evidence and restored pre-deploy artifact sets.
// Constraints: Database restore belongs to snapshot undo after all file undos.

import (
	"context"
	"fmt"
	"sort"

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

// UndoApplied restores files and reapplies the previous stack, without a DB undo.
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
		if err := deploy.RollbackRemote(ctx, deploy.RollbackOptions{
			Remote: r, RemotePath: paths[host], Tar: st.PreDeployTar[host],
			Changed: st.PreDeployChanged[host], Absent: st.PreDeployAbsent[host], StrictHasura: true,
		}); err != nil {
			return err
		}
	}
	return nil
}
