package commands

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

func TestPromoteRollbackConfirmation(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		root := promoteFixture(t)
		inv, err := controlplane.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		old := deployConfirmPrompt
		t.Cleanup(func() { deployConfirmPrompt = old })
		calls := 0
		deployConfirmPrompt = func(context.Context, string) bool { calls++; return false }
		rec := &cpromote.Record{ID: "fixture", Target: "prodsim", MigrateStarted: true}
		cmd := &cobra.Command{}
		cmd.Flags().Bool("yes", false, "")
		err = confirmPromoteRecordRollback(withPromoteRollbackConfirmation(context.Background(), cmd), inv, rec)
		if compat.V15() {
			if err == nil || !strings.Contains(err.Error(), "E403") || errs.ExitCodeFor(err) != 4 || calls != 1 {
				t.Fatalf("non-TTY prod restore: err=%v exit=%d prompts=%d", err, errs.ExitCodeFor(err), calls)
			}
		} else if err != nil || calls != 0 {
			t.Fatalf("v1.4 gate changed: %v prompts=%d", err, calls)
		}
		if err := cmd.Flags().Set("yes", "true"); err != nil {
			t.Fatal(err)
		}
		if err := confirmPromoteRecordRollback(withPromoteRollbackConfirmation(context.Background(), cmd), inv, rec); err != nil {
			t.Fatalf("--yes: %v", err)
		}
		cmd.Flags().Set("yes", "false") //nolint:errcheck // registered bool flag
		for _, exempt := range []*cpromote.Record{{Target: "ls", MigrateStarted: true}, {Target: "prodsim"}, {Target: "prodsim", MigrateStarted: true, DBRestoredAt: "already"}} {
			if err := confirmPromoteRecordRollback(context.Background(), inv, exempt); err != nil {
				t.Fatalf("exempt record: %v", err)
			}
		}
		deployConfirmPrompt = func(context.Context, string) bool { return true }
		if err := confirmPromoteRecordRollback(context.Background(), inv, rec); err != nil {
			t.Fatalf("TTY acceptance: %v", err)
		}
	})
}

func TestPromoteRollbackYesFlags(t *testing.T) {
	for _, cmd := range []*cobra.Command{deployRollbackCmd, promoteRollbackCmd} {
		if cmd.Flags().Lookup("yes") == nil {
			t.Errorf("%s rollback lacks --yes", cmd.Name())
		}
	}
}

func TestPromoteRollbackConfirmationBeforeRemote(t *testing.T) {
	for _, shim := range []bool{false, true} {
		t.Run(map[bool]string{false: "deploy", true: "shim"}[shim], func(t *testing.T) {
			compattest.Set(t, true)
			root := promoteFixture(t)
			rec := cpromote.Record{ID: "fixture", Target: "prodsim", Status: "ok", CompletedAt: time.Now(), MigrateStarted: true, BackupID: "fixture_backup", BackupHost: "prod-app", PreDeployTar: map[string]string{"prod-app": "/srv/fixture/.nself/backups/pre-deploy-fixture.tar.gz"}, Changed: map[string][]string{"prod-app": {"docker-compose.yml"}}, RemotePaths: map[string]string{"prod-app": "/srv/fixture"}}
			if err := cpromote.SaveRecord(root, rec); err != nil {
				t.Fatal(err)
			}
			oldOpen, oldPrompt := deployRemoteOpen, deployConfirmPrompt
			t.Cleanup(func() { deployRemoteOpen, deployConfirmPrompt = oldOpen, oldPrompt })
			opens := 0
			deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) {
				opens++
				return &v15Remote{label: "prod-app"}, nil
			}
			deployConfirmPrompt = func(context.Context, string) bool { return false }
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			cmd.Flags().Bool("yes", false, "")
			var err error
			if shim {
				err = runPromoteRecordRollback(cmd)
			} else {
				err = runDeployRollback(cmd, []string{"prodsim"})
			}
			if err == nil || !strings.Contains(err.Error(), "E403") || errs.ExitCodeFor(err) != 4 || opens != 0 {
				t.Fatalf("err=%v exit=%d remote opens=%d; want E403/4 before remote", err, errs.ExitCodeFor(err), opens)
			}
		})
	}
}
