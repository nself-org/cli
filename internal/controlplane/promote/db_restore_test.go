package promote

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/deploy"
)

type completionFailureRemote struct {
	recordRemote
	root     string
	restores int
}

func (r *completionFailureRemote) Run(ctx context.Context, cmd string) ([]byte, error) {
	if strings.Contains(cmd, "nself backup restore") {
		r.restores++
		if err := os.Mkdir(filepath.Join(r.root, RecordFile)+".tmp", 0700); err != nil {
			return nil, err
		}
	}
	return r.recordRemote.Run(ctx, cmd)
}

func TestPromoteRestoreCompletionSaveFailure(t *testing.T) {
	root := t.TempDir()
	r := &completionFailureRemote{recordRemote: recordRemote{label: "primary"}, root: root}
	rec := &Record{BackupID: "backup-overlay", BackupHost: "primary", MigrateStarted: true,
		PreDeployTar:          map[string]string{"primary": "/srv/app/.nself/backups/pre-deploy-1.tar.gz"},
		PriorManifestIdentity: map[string]ManifestIdentity{"primary": {Hash: ManifestHash(nil)}}}
	persistRollbackFixture(t, root, rec)
	remotes, paths := map[string]deploy.Remote{"primary": r}, map[string]string{"primary": "/srv/app"}
	if err := RollbackRecord(context.Background(), root, rec, remotes, paths); err == nil {
		t.Fatal("completion save must fail")
	}
	if err := os.Remove(filepath.Join(root, RecordFile) + ".tmp"); err != nil {
		t.Fatal(err)
	}
	err := RollbackRecord(context.Background(), root, rec, remotes, paths)
	if err == nil || !strings.Contains(err.Error(), "E489") || !strings.Contains(err.Error(), "backup-overlay") || !strings.Contains(err.Error(), "may already have run") || !strings.Contains(err.Error(), "--db-restore-done") || r.restores != 1 {
		t.Fatalf("unsafe retry: restores=%d err=%v", r.restores, err)
	}
	if err := MarkDBRestoreDone(root, rec); err != nil {
		t.Fatal(err)
	}
	if err := RollbackRecord(context.Background(), root, rec, remotes, paths); err != nil {
		t.Fatal(err)
	}
	if r.restores != 1 || rec.Status != "rolled_back" {
		t.Fatalf("completion acknowledgement repeated restore: %+v restores=%d", rec, r.restores)
	}
}

func TestSnapshotUndoCompletionSaveFailure(t *testing.T) {
	r := &restoreRemote{}
	st := &State{BackupID: "backup-overlay", RecoveryCheckpoint: func(st *State) error {
		if st.DBRestoredAt != "" {
			return fmt.Errorf("completion save failed")
		}
		return nil
	}}
	undo := SnapshotStep(r, "/srv/app", "pre-promote-1-aaaaaaaa").Undo
	if err := undo(context.Background(), st); err == nil {
		t.Fatal("completion save must fail")
	}
	if st.DBRestoredAt != "" || st.RecoveryPhase != "db_restore_started" {
		t.Fatalf("uncertain completion retained a success marker: %+v", st)
	}
	err := undo(context.Background(), st)
	if err == nil || !strings.Contains(err.Error(), "E489") || len(r.commands) != 1 {
		t.Fatalf("unsafe engine retry commands=%v err=%v", r.commands, err)
	}
}

func TestSnapshotUndoIntentSaveFailure(t *testing.T) {
	r := &restoreRemote{}
	st := &State{BackupID: "backup-overlay", RecoveryCheckpoint: func(*State) error { return fmt.Errorf("intent save failed") }}
	if err := SnapshotStep(r, "/srv/app", "pre-promote-1-aaaaaaaa").Undo(context.Background(), st); err == nil || len(r.commands) != 0 {
		t.Fatalf("restore ran without durable intent: commands=%v err=%v", r.commands, err)
	}
}

func TestSnapshotUndoRequiresCheckpoint(t *testing.T) {
	r := &restoreRemote{}
	if err := SnapshotStep(r, "/srv/app", "pre-promote-1-aaaaaaaa").Undo(context.Background(), &State{BackupID: "backup-overlay"}); err == nil || len(r.commands) != 0 {
		t.Fatalf("restore without checkpoint writer: commands=%v err=%v", r.commands, err)
	}
}
