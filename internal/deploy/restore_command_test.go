package deploy

// Purpose: pin C2-01 on the deploy side: the rollback restore carries
// --confirm-project (a remote run has no stdin for the production prompt), and
// a database rollback without a project name is refused before any host change.

import (
	"context"
	"strings"
	"testing"
)

func TestBackupRestoreCommandCarriesProof(t *testing.T) {
	got, err := BackupRestoreCommand("demo_full_1", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if want := "nself backup restore 'demo_full_1' --yes --confirm-project 'demo'"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if _, err := BackupRestoreCommand("demo_full_1", " "); err == nil {
		t.Fatal("an empty project name was accepted")
	}
	if h := BackupRestoreHint("demo_full_1", ""); !strings.Contains(h, "--confirm-project <project>") || strings.Contains(h, "--yes") {
		t.Fatalf("hint %q", h)
	}
}

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
