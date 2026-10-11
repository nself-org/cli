package commands

// Purpose: Persist recovery authority before migration and after remote changes.
// Inputs: Wired steps and their shared state. Outputs: Atomic provisional records.
// Constraints: A final record failure leaves the last provisional record intact.

import (
	"context"
	"time"

	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/deploy"
)

func checkpointPromoteStep(w *promoteWiring, step cpromote.Step, before bool) cpromote.Step {
	run := step.Run
	step.Run = func(ctx context.Context, st *cpromote.State) error {
		if before {
			if err := savePromoteRecord(w, st, "in_progress"); err != nil {
				return err
			}
		}
		if err := run(ctx, st); err != nil {
			return err
		}
		if !before {
			st.RemotePaths = map[string]string{st.BackupHost: promotePath(w.targetServer, w.workdir)}
			return savePromoteRecord(w, st, "in_progress")
		}
		return nil
	}
	return step
}

func runPromoteRecord(w *promoteWiring) func(context.Context, *cpromote.State) error {
	return func(_ context.Context, st *cpromote.State) error { return savePromoteRecord(w, st, "ok") }
}

func savePromoteRecord(w *promoteWiring, st *cpromote.State, status string) error {
	saved, _ := st.Values["saved_manifest"].(cpromote.SavedManifest)
	return cpromote.SaveRecord(w.workdir, cpromote.Record{
		ID: deploy.NewBackupID(w.started), Source: st.Source, Target: st.Target,
		StartedAt: w.started, CompletedAt: time.Now().UTC(), Status: status, SnapshotTag: st.SnapshotTag,
		BackupID: st.BackupID, BackupHost: st.BackupHost, PreDeployTar: st.PreDeployTar,
		Changed: st.PreDeployChanged, Absent: st.PreDeployAbsent, Digests: st.Digests, SavedManifest: saved,
		DBRestoredAt: st.DBRestoredAt, MigrateStarted: st.MigrateStarted,
		RemotePaths: st.RemotePaths, AppliedManifestHash: st.AppliedManifestHash,
	})
}
