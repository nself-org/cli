package commands

// Purpose: pin the C2-01 review decision: a manual deploy rollback restores a
// production database only with an operator-typed --confirm-project; --yes
// alone is refused before any host is opened, and the name is never taken
// from the local config there. Non-production keeps the config fallback.
// Inputs: v15Project/rollbackRecord fixtures and the promote record fixture.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

func rollbackCmd(project string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.WithValue(context.Background(), deployYesKey{}, true))
	cmd.Flags().Bool("yes", true, "")
	cmd.Flags().String("confirm-project", project, "")
	return cmd
}

func TestRollbackProdNeedsTypedProject(t *testing.T) {
	proj, remote := v15Project(t)
	rollbackRecord(t, proj, "deploy@app.example.test", "deploy@app.example.test")
	opens := 0
	deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) {
		opens++
		return remote, nil
	}
	handled, err := rollbackRemoteV15(rollbackCmd(""), proj, "prod")
	if !handled || err == nil || !strings.Contains(err.Error(), "E403") || !strings.Contains(err.Error(), "--confirm-project") || errs.ExitCodeFor(err) != 4 {
		t.Fatalf("--yes alone on prod: handled=%v err=%v exit=%d", handled, err, errs.ExitCodeFor(err))
	}
	if opens != 0 || countCmds(remote, "nself backup restore") != 0 {
		t.Fatalf("a refused rollback opened %d host(s) or restored the database", opens)
	}
	if st, _ := deploy.LoadDeployState(proj); st.RolledBack || st.DBRestoredAt != "" {
		t.Fatalf("a refused rollback changed the record: %+v", st)
	}
	if handled, err := rollbackRemoteV15(rollbackCmd("typed-name"), proj, "prod"); !handled || err != nil {
		t.Fatalf("typed project: handled=%v err=%v", handled, err)
	}
	if countCmds(remote, "--confirm-project 'typed-name'") != 1 || countCmds(remote, "--confirm-project 'demo'") != 0 {
		t.Fatalf("the restore must carry the typed name, not the config's:\n%s", strings.Join(remote.commands(), "\n"))
	}
}

func TestRollbackNonProdKeepsConfigProject(t *testing.T) {
	proj, remote := v15Project(t)
	st := deploy.StrategyState{Strategy: deploy.StrategyRolling, Target: "staging", Success: true, BackupID: "demo_full_9", BackupHost: "deploy@app.example.test",
		PreDeployTars:    map[string]string{"deploy@app.example.test": "/opt/nself/.nself/backups/pre-deploy-20261011T010101Z-aaaaaa.tar.gz"},
		PreDeployChanged: map[string][]string{"deploy@app.example.test": {"docker-compose.yml"}}, PreDeployAbsent: map[string][]string{"deploy@app.example.test": {}}}
	if err := deploy.SaveDeployState(proj, st); err != nil {
		t.Fatal(err)
	}
	if handled, err := rollbackRemoteV15(rollbackCmd(""), proj, "staging"); !handled || err != nil {
		t.Fatalf("non-prod rollback with --yes: handled=%v err=%v", handled, err)
	}
	if countCmds(remote, "--confirm-project 'demo'") != 1 {
		t.Fatalf("non-prod must fall back to the config PROJECT_NAME:\n%s", strings.Join(remote.commands(), "\n"))
	}
}

func TestPromoteRecordRollbackProdNeedsTypedProject(t *testing.T) {
	root := promoteFixture(t)
	rec := cpromote.Record{ID: "fixture", Target: "prodsim", Status: "ok", CompletedAt: time.Now(), MigrateStarted: true, BackupID: "fixture_backup", BackupHost: "prod-app", PreDeployTar: map[string]string{"prod-app": "/srv/fixture/.nself/backups/pre-deploy-fixture.tar.gz"}, Changed: map[string][]string{"prod-app": {"docker-compose.yml"}}, RemotePaths: map[string]string{"prod-app": "/srv/fixture"}}
	if err := cpromote.SaveRecord(root, rec); err != nil {
		t.Fatal(err)
	}
	oldOpen := deployRemoteOpen
	t.Cleanup(func() { deployRemoteOpen = oldOpen })
	opens := 0
	deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) {
		opens++
		return &v15Remote{label: "prod-app"}, nil
	}
	cmd := rollbackCmd("")
	err := rollbackPromoteRecord(withPromoteRollbackConfirmation(cmd.Context(), cmd), root, &rec)
	if err == nil || !strings.Contains(err.Error(), "E403") || !strings.Contains(err.Error(), "--confirm-project") || opens != 0 {
		t.Fatalf("--yes alone on a prod promote record: err=%v opens=%d", err, opens)
	}
	for _, c := range []*cobra.Command{deployRollbackCmd, promoteRollbackCmd} {
		if f := c.Flags().Lookup("confirm-project"); f == nil || f.Value.Type() != "string" {
			t.Errorf("%s lacks --confirm-project", c.CommandPath())
		}
	}
}
