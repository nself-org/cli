package promote

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/deploy"
)

func TestPromoteAppliedFilesUndo(t *testing.T) {
	st := &State{BackupID: "verified-db-backup"}
	RecordApplied(st, deploy.ApplyResult{BackupHost: "untouched"})
	if len(st.PreDeployTar) != 0 {
		t.Fatal("unsynced host was recorded for rollback")
	}
	RecordApplied(st, deploy.ApplyResult{Synced: true, BackupHost: "primary", PreDeployTar: "/srv/app/.nself/backups/pre-deploy-1.tar.gz"})
	r := &recordRemote{label: "primary", compose: true}
	if err := UndoApplied(context.Background(), st, map[string]deploy.Remote{"primary": r}, map[string]string{"primary": "/srv/app"}); err != nil {
		t.Fatal(err)
	}
	log := strings.Join(r.calls, "\n")
	if !strings.Contains(log, "tar ") || !strings.Contains(log, "docker compose") {
		t.Fatalf("files undo did not restore and reapply: %s", log)
	}
	if strings.Contains(log, "backup restore") {
		t.Fatalf("files undo restored database before snapshot undo: %s", log)
	}
	if err := UndoApplied(context.Background(), st, nil, nil); err == nil || !strings.Contains(err.Error(), "primary") {
		t.Fatalf("missing remote = %v", err)
	}
}
