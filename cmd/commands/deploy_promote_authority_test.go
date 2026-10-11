package commands

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/spf13/cobra"
)

func TestPromoteFailedLaterAttemptPreservesAuthority(t *testing.T) {
	compattest.Set(t, true)
	root := promoteFixture(t)
	p1 := cpromote.Record{ID: "p1", Target: "prodsim", Status: "ok", StartedAt: time.Now().Add(-time.Hour), CompletedAt: time.Now(),
		BackupID: "backup_p1", BackupHost: "prod-app", MigrateStarted: true,
		PreDeployTar: map[string]string{"prod-app": "/srv/fixture/.nself/backups/pre-deploy-p1.tar.gz"},
		Changed:      map[string][]string{"prod-app": {"docker-compose.yml"}}, RemotePaths: map[string]string{"prod-app": "/srv/fixture"},
		AppliedManifestHash: map[string]string{"prod-app": cpromote.ManifestHash(nil)}}
	if err := deploy.SaveDeployState(root, deploy.StrategyState{Target: p1.Target, StartedAt: p1.StartedAt, PreDeployTars: p1.PreDeployTar, PreDeployChanged: p1.Changed}); err != nil {
		t.Fatal(err)
	}
	if err := cpromote.SaveRecord(root, p1); err != nil {
		t.Fatal(err)
	}
	w := &promoteWiring{workdir: root, started: time.Now()}
	st := &cpromote.State{Source: "ls", Target: "prodsim", BackupID: "backup_p2", BackupHost: "prod-app", Values: map[string]any{}}
	// Snapshot checkpoint, followed by a save failure compensated without sync.
	eng := cpromote.Engine{}
	for _, name := range cpromote.StepNames {
		step := cpromote.Step{Name: name, Run: func(context.Context, *cpromote.State) error { return nil }}
		if name == "snapshot" {
			step.Run = func(context.Context, *cpromote.State) error { return savePromoteRecord(w, st, "in_progress") }
		}
		if name == "save" {
			step.Run = func(context.Context, *cpromote.State) error { return fmt.Errorf("injected save failure") }
		}
		eng.Steps = append(eng.Steps, step)
	}
	result, err := eng.Run(context.Background(), st)
	if err == nil || result.FailedStep != "save" || result.Status != "rolled_back" {
		t.Fatalf("P2 result=%+v err=%v", result, err)
	}
	if err := savePromoteRecord(w, st, result.Status); err != nil {
		t.Fatal(err)
	}
	r := &v15Remote{label: "prod-app"}
	old := deployRemoteOpen
	t.Cleanup(func() { deployRemoteOpen = old })
	deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) { return r, nil }
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("yes", true, "")
	if err := runDeployRollback(cmd, []string{"prodsim"}); err != nil {
		t.Fatal(err)
	}
	log := strings.Join(r.commands(), "\n")
	files, db := strings.Index(log, "tar -xzf"), strings.Index(log, "backup restore 'backup_p1'")
	if files < 0 || db < files {
		t.Fatalf("lost live backup authority or wrong D16 order: %s", log)
	}
}

func TestPromoteApplyDeployRecordKeepsBackup(t *testing.T) {
	compattest.Set(t, true)
	locked := compose.LockedImages()[0]
	source, _ := setupPromoteSafety(t, locked.LegacyRef, locked.String(), false)
	root := mustProjectRoot(t)
	wf(t, filepath.Join(root, "docker-compose.yml"), "services:\n  api:\n    image: "+locked.LegacyRef+"\n    build: .\n")
	r := &v15Remote{label: "prod-app"}
	deployRemoteOpen = func(_ context.Context, srv controlplane.Server, _, _ string) (deploy.Remote, error) {
		if srv.Name == "ls-app" {
			return source, nil
		}
		return r, nil
	}
	oldClock := deployGateClock
	t.Cleanup(func() { deployGateClock = oldClock })
	deployGateClock = &noClock{now: time.Unix(0, 0)}
	eng, st, err := preparePromoteEngine(promoteTestCommand(&bytes.Buffer{}, false, true, false), root, "ls", "prodsim", false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range eng.Steps[:11] {
		if step.Run != nil {
			if err := step.Run(context.Background(), st); err != nil {
				t.Fatalf("%s: %v", step.Name, err)
			}
		}
	}
	state, err := deploy.LoadDeployState(root)
	if err != nil || st.BackupID == "" || state.BackupID != st.BackupID || state.BackupHost != st.BackupHost {
		t.Fatalf("deploy lost snapshot authority state=%+v promote=%+v err=%v", state, st, err)
	}
}
