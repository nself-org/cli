package promote

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/deploy"
)

type retryRecordRemote struct {
	recordRemote
	absent, restored             bool
	files, dbAttempts, dbSuccess int
}

func (r *retryRecordRemote) Run(ctx context.Context, cmd string) ([]byte, error) {
	if strings.HasPrefix(cmd, "cat ") && strings.Contains(cmd, "compose-files.txt") {
		if r.restored {
			if r.absent {
				return nil, fmt.Errorf("manifest absent")
			}
			return []byte("prior\n"), nil
		}
		return []byte("applied\n"), nil
	}
	if strings.Contains(cmd, "nself-promote-manifest-absent") && r.restored {
		if r.absent {
			return []byte("nself-promote-manifest-absent\n"), nil
		}
		return []byte("prior\n"), nil
	}
	if strings.Contains(cmd, "tar -xzf") {
		r.files++
		r.restored = true
	}
	if strings.Contains(cmd, "nself backup restore") {
		r.dbAttempts++
		if r.dbAttempts == 1 {
			return nil, fmt.Errorf("transient restore failure")
		}
		r.dbSuccess++
	}
	return r.recordRemote.Run(ctx, cmd)
}

func TestPromoteRecordRetryAfterFilesRestored(t *testing.T) {
	for _, absent := range []bool{false, true} {
		t.Run(fmt.Sprint(absent), func(t *testing.T) {
			root := t.TempDir()
			r := &retryRecordRemote{recordRemote: recordRemote{label: "primary"}, absent: absent}
			rec := &Record{BackupID: "backup-1", BackupHost: "primary", MigrateStarted: true,
				PreDeployTar:        map[string]string{"primary": "/srv/app/.nself/backups/pre-deploy-1.tar.gz"},
				AppliedManifestHash: map[string]string{"primary": ManifestHash([]byte("applied\n"))}}
			hash := ManifestHash([]byte("prior\n"))
			if absent {
				hash = ""
			}
			prior, _ := json.Marshal(map[string]any{"prior_manifest_identity": map[string]any{"primary": map[string]any{"hash": hash, "absent": absent}}})
			if err := json.Unmarshal(prior, rec); err != nil {
				t.Fatal(err)
			}
			persistRollbackFixture(t, root, rec)
			remotes, paths := map[string]deploy.Remote{"primary": r}, map[string]string{"primary": "/srv/app"}
			if err := RollbackRecord(context.Background(), root, rec, remotes, paths); err == nil || !strings.Contains(err.Error(), "transient restore failure") {
				t.Fatalf("first rollback: %v files=%d", err, r.files)
			}
			saved, err := LoadRecord(root)
			if err != nil || saved == nil || saved.RecoveryPhase != "db_restore_started" || !saved.FilesRestored["primary"] {
				t.Fatalf("files phase not durable: %#v %v", saved, err)
			}
			if err := RollbackRecord(context.Background(), root, rec, remotes, paths); err == nil || !strings.Contains(err.Error(), "E489") {
				t.Fatalf("uncertain retry must refuse: %v", err)
			}
			// The operator restored by hand and verified the database on the host.
			if err := MarkDBRestoreDone(root, rec); err != nil {
				t.Fatal(err)
			}
			if err := RollbackRecord(context.Background(), root, rec, remotes, paths); err != nil {
				t.Fatalf("verified completion must recover: %v", err)
			}
			if rec.RecoveryPhase != "db_restored" {
				t.Fatalf("DB phase missing: %#v", rec)
			}
			if r.files != 1 || r.dbSuccess != 0 || r.dbAttempts != 1 {
				t.Fatalf("files=%d DB successes=%d attempts=%d", r.files, r.dbSuccess, r.dbAttempts)
			}
		})
	}
}

func TestPromoteUndoAppliedCheckpointsFiles(t *testing.T) {
	r := &retryRecordRemote{recordRemote: recordRemote{label: "primary"}}
	checkpoints := 0
	st := &State{PreDeployTar: map[string]string{"primary": "/srv/app/.nself/backups/pre-deploy-1.tar.gz"},
		PreDeployChanged: map[string][]string{"primary": nil},
		RecoveryCheckpoint: func(st *State) error {
			if !st.FilesRestored["primary"] || st.RecoveryPhase != "files_restored" {
				return fmt.Errorf("missing phase")
			}
			checkpoints++
			return nil
		}}
	for i := 0; i < 2; i++ {
		if err := UndoApplied(context.Background(), st, map[string]deploy.Remote{"primary": r}, map[string]string{"primary": "/srv/app"}); err != nil {
			t.Fatal(err)
		}
	}
	if checkpoints != 1 || r.files != 1 {
		t.Fatalf("checkpoints=%d files restores=%d", checkpoints, r.files)
	}
}
