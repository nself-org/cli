package commands

// Purpose: the command side of shipping a fronting stack's nginx/sites (D-0425):
//          the front root's ssl layout in the target snapshot, the record of what
//          the previous deploy shipped there (the only files a prune may remove),
//          and the per-host front fields of deploy.json.
// Inputs:  the v15 params, the target snapshot, the staged set, an ApplyResult.
// Outputs: snap.FrontSSL, set.Front.Previous, StrategyState front fields.
// Constraints: only for a project with NGINX_FRONTED_BY whose host layout follows
//              the convention (dir(remote_path) is named FrontedBy; deploy.Stage
//              refuses the others, E485). Reads file names only, never a PEM.

import (
	"context"
	"path"

	"github.com/nself-org/cli/internal/deploy"
)

// snapshotFrontSSL adds the fronting stack's <front>/ssl/certificates layout to
// snap for a fronted project; any other project is left as it is.
func snapshotFrontSSL(ctx context.Context, r deploy.Remote, p v15Params, snap *deploy.TargetSnapshot) error {
	frontedBy, err := scratchFrontedBy(p.workdir, p.target)
	if err != nil || frontedBy == "" {
		return err
	}
	front := path.Dir(p.srv.RemotePath)
	if path.Base(front) != frontedBy {
		return nil // deploy.Stage refuses this layout with E485 before anything is sent
	}
	snap.FrontSSL, err = deploy.TakeFrontSSL(ctx, r, front)
	return err
}

// markFrontPrevious sets, per host, the front paths the target's last recorded
// deploy shipped or pruned there on set.Front. Without a readable record
// nothing is set and nothing in the front root is pruned.
func markFrontPrevious(set *deploy.ArtifactSet, workdir, target string) {
	st, err := deploy.LoadDeployState(workdir)
	if set.Front == nil || err != nil || st.Target != target {
		return
	}
	set.Front.Previous = map[string][]string{} // per host: a host prunes only what its own record says
	for host, paths := range st.FrontShipped { // shipped, not pruned: an old record has none, so nothing is pruned
		set.Front.Previous[host] = append([]string{}, paths...)
	}
	set.Front.PreviousNames = st.FrontNames // the server_name set of each: a path without one is never pruned
}

// recordFront adds the host's front tar and lists to the deploy record. A deploy
// that changed nothing in the front root (no file shipped, nothing pruned) records
// no front part: there is nothing to restore (the same when the front sync never
// started, FrontSynced false: the front root was not touched), and rollback then treats the host as
// it did before the front root existed.
func recordFront(st *deploy.StrategyState, res deploy.ApplyResult) {
	if res.FrontTar == "" || len(res.FrontChanged) == 0 || !res.FrontSynced {
		return
	}
	if st.FrontTars == nil {
		st.FrontTars, st.FrontChanged, st.FrontAbsent = map[string]string{}, map[string][]string{}, map[string][]string{}
	}
	if st.FrontShipped == nil {
		st.FrontShipped = map[string][]string{}
	}
	st.FrontShipped[res.BackupHost] = res.FrontShipped
	if st.FrontNames == nil {
		st.FrontNames = map[string]map[string][]string{}
	}
	st.FrontNames[res.BackupHost] = res.FrontNames
	if st.FrontPrevNames == nil {
		st.FrontPrevNames = map[string]map[string][]string{}
	}
	st.FrontPrevNames[res.BackupHost] = res.FrontPrevNames
	st.FrontTars[res.BackupHost] = res.FrontTar
	st.FrontChanged[res.BackupHost], st.FrontAbsent[res.BackupHost] = res.FrontChanged, res.FrontAbsent
}
