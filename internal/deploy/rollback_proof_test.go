package deploy

// Purpose: pin C2-01 on the deploy side: a database rollback without a
// project name is refused before any host change (the proof flag itself is
// tested in internal/backup/consent).

import (
	"context"
	"strings"
	"testing"
)

func TestRollbackWithoutProjectNameChangesNothing(t *testing.T) {
	f := newFakeRemote(t)
	err := RollbackRemote(context.Background(), RollbackOptions{
		Remote: f, RemotePath: "/srv/app", Tar: "/srv/app/.nself/backups/pre-deploy-1.tar.gz",
		BackupID: "backup-1", Changed: []string{"docker-compose.yml"},
	})
	if err == nil || !strings.Contains(err.Error(), "--confirm-project") {
		t.Fatalf("want a refusal naming --confirm-project, got %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.log) != 0 {
		t.Fatalf("the host was touched: %v", f.log)
	}
}
