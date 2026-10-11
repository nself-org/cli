package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/spf13/cobra"
)

type dbCompletionSaveFailureRemote struct {
	*v15Remote
	root     string
	restores int
}

func (r *dbCompletionSaveFailureRemote) Run(ctx context.Context, command string) ([]byte, error) {
	if strings.Contains(command, "nself backup restore") {
		r.restores++
		if err := os.Mkdir(filepath.Join(r.root, cpromote.RecordFile)+".tmp", 0700); err != nil {
			return nil, err
		}
	}
	return r.v15Remote.Run(ctx, command)
}

func TestPromoteRollbackDBRestoreDoneFlag(t *testing.T) {
	compattest.Set(t, true)
	root := promoteFixture(t)
	rec := cpromote.Record{ID: "uncertain", Target: "prodsim", Status: "rollback_failed", CompletedAt: time.Now(),
		MigrateStarted: true, BackupID: "fixture_backup", BackupHost: "prod-app",
		PreDeployTar: map[string]string{"prod-app": "/srv/fixture/.nself/backups/pre-deploy-fixture.tar.gz"},
		Changed:      map[string][]string{"prod-app": {}}, RemotePaths: map[string]string{"prod-app": "/srv/fixture"},
		AppliedManifestHash:   map[string]string{"prod-app": cpromote.ManifestHash(nil)},
		PriorManifestIdentity: map[string]cpromote.ManifestIdentity{"prod-app": {Hash: cpromote.ManifestHash([]byte("/opt/nself/docker-compose.yml\n"))}}}
	if err := cpromote.SaveRecord(root, rec); err != nil {
		t.Fatal(err)
	}
	// The same promotion may leave its pre-apply attempt beside the live record.
	if err := cpromote.SaveAttempt(root, rec); err != nil {
		t.Fatal(err)
	}
	r := &dbCompletionSaveFailureRemote{v15Remote: &v15Remote{label: "prod-app"}, root: root}
	old := deployRemoteOpen
	t.Cleanup(func() { deployRemoteOpen = old })
	opens := 0
	deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) {
		opens++
		return r, nil
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("yes", true, "")
	cmd.Flags().Bool("db-restore-done", false, "")
	if err := runDeployRollback(cmd, []string{"prodsim"}); err == nil {
		t.Fatal("completion save must fail")
	}
	if err := os.Remove(filepath.Join(root, cpromote.RecordFile) + ".tmp"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("yes", "false"); err != nil {
		t.Fatal(err)
	}
	opens = 0
	if err := runDeployRollback(cmd, []string{"prodsim"}); err == nil || !strings.Contains(err.Error(), "E489") || !strings.Contains(err.Error(), "--db-restore-done") || opens != 0 {
		t.Fatalf("uncertain rollback=%v opens=%d", err, opens)
	}
	if deployRollbackCmd.Flags().Lookup("db-restore-done") == nil {
		t.Fatal("actual rollback command lacks flag")
	}
	if err := cmd.Flags().Set("db-restore-done", "true"); err != nil {
		t.Fatal(err)
	}
	if err := runDeployRollback(cmd, []string{"prodsim"}); err != nil {
		t.Fatal(err)
	}
	if r.restores != 1 {
		t.Fatalf("acknowledging completion repeated restore: %d", r.restores)
	}
	saved, err := cpromote.LoadRecord(root)
	if err != nil || saved.DBRestoredAt == "" || saved.Status != "rolled_back" {
		t.Fatalf("acknowledged record=%+v err=%v", saved, err)
	}
}
