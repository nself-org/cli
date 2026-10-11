package commands

import (
	"bytes"
	"context"
	"fmt"
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

func TestPromoteDeployRollbackInProgress(t *testing.T) {
	for _, migrated := range []bool{false, true} {
		t.Run(fmt.Sprint(migrated), func(t *testing.T) {
			compattest.Set(t, true)
			root := promoteFixture(t)
			rec := cpromote.Record{ID: "interrupted", Source: "ls", Target: "prodsim", Status: "in_progress",
				MigrateStarted: migrated, BackupID: "fixture_backup", BackupHost: "prod-app", SnapshotTag: "pre-promote-fixture",
				PreDeployTar: map[string]string{"prod-app": "/srv/fixture/.nself/backups/pre-deploy-fixture.tar.gz"},
				Changed:      map[string][]string{"prod-app": {"docker-compose.yml"}}, RemotePaths: map[string]string{"prod-app": "/srv/fixture"},
				AppliedManifestHash: map[string]string{"prod-app": cpromote.ManifestHash(nil)}}
			if err := cpromote.SaveRecord(root, rec); err != nil {
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
			if restored := strings.Contains(log, "nself backup restore 'fixture_backup' --yes"); restored != migrated {
				t.Fatalf("DB restored=%v migrate started=%v commands=%s", restored, migrated, log)
			}
			if !strings.Contains(log, "tar -xzf") {
				t.Fatalf("files not restored: %s", log)
			}
			saved, err := cpromote.LoadRecord(root)
			if err != nil || saved.Status != "rolled_back" || (saved.DBRestoredAt != "") != migrated {
				t.Fatalf("saved recovery=%+v err=%v", saved, err)
			}
		})
	}
}

func TestPromoteFinalRecordFailurePreservesProvisional(t *testing.T) {
	root := t.TempDir()
	w := &promoteWiring{workdir: root, started: time.Now()}
	st := &cpromote.State{Source: "ls", Target: "prodsim", SnapshotTag: "pre-promote-fixture", BackupID: "fixture_backup", BackupHost: "prod-app", MigrateStarted: true,
		PreDeployTar:     map[string]string{"prod-app": "/srv/fixture/.nself/backups/pre-deploy-fixture.tar.gz"},
		PreDeployChanged: map[string][]string{"prod-app": {"docker-compose.yml"}}, RemotePaths: map[string]string{"prod-app": "/srv/fixture"},
		AppliedManifestHash: map[string]string{"prod-app": cpromote.ManifestHash(nil)}, Values: map[string]any{}}
	if err := savePromoteRecord(w, st, "in_progress"); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, cpromote.RecordFile)
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file+".tmp", 0o700); err != nil {
		t.Fatal(err)
	}
	undos := 0
	eng := cpromote.Engine{}
	for _, name := range cpromote.StepNames {
		eng.Steps = append(eng.Steps, cpromote.Step{Name: name, Run: func(context.Context, *cpromote.State) error { return nil }, Undo: func(context.Context, *cpromote.State) error { undos++; return nil }})
	}
	eng.Steps[len(eng.Steps)-1].Run = runPromoteRecord(w)
	result, err := eng.Run(context.Background(), st)
	if err == nil || !strings.Contains(err.Error(), "E489") || result.Status != "failed" || result.FailedStep != "record" || undos != 0 {
		t.Fatalf("record failure result=%+v err=%v undos=%d", result, err, undos)
	}
	for _, hint := range []string{"promotion is live", st.SnapshotTag, st.BackupID, st.PreDeployTar["prod-app"]} {
		if !strings.Contains(err.Error(), hint) {
			t.Fatalf("missing %q: %v", hint, err)
		}
	}
	after, readErr := os.ReadFile(file)
	if readErr != nil || !bytes.Equal(before, after) {
		t.Fatalf("provisional changed after final write failure: %s err=%v", after, readErr)
	}
}
