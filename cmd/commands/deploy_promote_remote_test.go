package commands

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/spf13/cobra"
)

type promoteEvidenceTestRemote struct{ writes int }

func (*promoteEvidenceTestRemote) Label() string { return "target" }
func (r *promoteEvidenceTestRemote) Run(_ context.Context, command string) ([]byte, error) {
	if strings.Contains(command, "nself-promote-manifest-absent") {
		return []byte("prior-manifest\n"), nil
	}
	if strings.Contains(command, "tar -hczf") {
		if strings.Contains(command, "nginx/sites/new.conf") {
			return []byte("ABSENT nginx/sites/new.conf\nABSENT " + deploy.ShippedFile + "\n"), nil
		}
		return []byte("ABSENT " + deploy.ShippedFile + "\n"), nil
	}
	r.writes++
	panic("process died during up")
}
func (r *promoteEvidenceTestRemote) Rsync(context.Context, []string, string, string, string) error {
	r.writes++
	return nil
}

func TestPromoteRemoteEvidenceBeforeHostWrite(t *testing.T) {
	ctx := context.Background()
	st := &cpromote.State{Values: map[string]any{}}
	base := &promoteEvidenceTestRemote{}
	saved := false
	r := newPromoteEvidenceRemote(base, "/srv/project", deploy.ArtifactSet{Files: []string{"docker-compose.yml", "nginx/sites/new.conf"}}, nil, st, func() error {
		saved = true
		if base.writes != 0 || st.PreDeployTar["target"] == "" || st.RemotePaths["target"] != "/srv/project" || st.PriorManifestIdentity["target"].Hash != cpromote.ManifestHash([]byte("prior-manifest\n")) {
			t.Fatal("checkpoint lacks pre-write recovery evidence")
		}
		return nil
	})
	if _, err := deploy.CreatePreDeployTar(ctx, r, "/srv/project", deploy.NewBackupID(time.Now()), []string{"docker-compose.yml", "nginx/sites/new.conf"}); err != nil {
		t.Fatal(err)
	}
	if !saved {
		t.Fatal("tar recovery evidence was not persisted before host writes")
	}
	func() {
		defer func() { _ = recover() }()
		_, _ = r.Run(ctx, "docker compose up -d")
	}()
	if st.PreDeployTar["target"] == "" || len(st.PreDeployAbsent["target"]) != 2 {
		t.Fatal("process death erased rollback evidence")
	}
}

func TestPromoteRemoteEvidenceSaveFailureBlocksSync(t *testing.T) {
	base := &promoteEvidenceTestRemote{}
	r := newPromoteEvidenceRemote(base, "/srv/project", deploy.ArtifactSet{Files: []string{"docker-compose.yml"}}, nil, &cpromote.State{Values: map[string]any{}}, func() error { return errors.New("checkpoint disk full") })
	_, err := deploy.CreatePreDeployTar(context.Background(), r, "/srv/project", deploy.NewBackupID(time.Now()), []string{"docker-compose.yml"})
	if err == nil || !strings.Contains(err.Error(), "checkpoint disk full") || base.writes != 0 {
		t.Fatalf("save failure did not stop apply: %v, writes=%d", err, base.writes)
	}
}

func TestPromoteKilledApplyFirstRollbackRestoresFiles(t *testing.T) {
	for _, migrated := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-migrate", true: "after-migrate"}[migrated], func(t *testing.T) {
			compattest.Set(t, true)
			root := promoteFixture(t)
			w := &promoteWiring{workdir: root, started: time.Now().UTC()}
			st := &cpromote.State{Source: "stage", Target: "prodsim", BackupID: "backup-live", BackupHost: "target", ApplyStarted: true, MigrateStarted: migrated,
				Values: map[string]any{}, AppliedManifestHash: map[string]string{"target": cpromote.ManifestHash(nil)}}
			r := newPromoteEvidenceRemote(&promoteEvidenceTestRemote{}, "/srv/fixture", deploy.ArtifactSet{Files: []string{"docker-compose.yml"}}, nil, st, func() error { return savePromoteRecord(w, st, "in_progress") })
			if _, err := deploy.CreatePreDeployTar(context.Background(), r, "/srv/fixture", deploy.NewBackupID(time.Now()), []string{"docker-compose.yml"}); err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() { _ = recover() }()
				_, _ = r.Run(context.Background(), "docker compose up -d")
			}()
			rec, err := cpromote.LoadLatestRecord(root, "prodsim")
			if err != nil || rec == nil {
				t.Fatalf("first rollback lost interrupted apply authority: %v", err)
			}
			recovery := &v15Remote{label: "target"}
			old := deployRemoteOpen
			t.Cleanup(func() { deployRemoteOpen = old })
			deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) {
				return recovery, nil
			}
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			cmd.Flags().Bool("yes", true, "")
			if err := runDeployRollback(cmd, []string{"prodsim"}); err != nil {
				t.Fatal(err)
			}
			log := recovery.commands()
			filesAt, dbAt := firstIndex(log, "tar -xzf"), firstIndex(log, "backup restore")
			if filesAt < 0 || (dbAt >= 0) != migrated || (migrated && dbAt <= filesAt) {
				t.Fatalf("first rollback files/DB gating or order: %v", log)
			}
		})
	}
}

func TestPromoteInterruptedApplyDeployFallback(t *testing.T) {
	for _, matching := range []bool{false, true} {
		t.Run(map[bool]string{false: "different-attempt", true: "same-attempt"}[matching], func(t *testing.T) {
			root := t.TempDir()
			when := time.Now().UTC()
			rec := cpromote.Record{ID: "interrupted", Target: "prodsim", Status: "in_progress", StartedAt: when, ApplyStarted: true, BackupID: "backup-live", BackupHost: "target",
				DeployIdentity: &cpromote.DeployIdentity{StartedAt: when}, RemotePaths: map[string]string{"target": "/srv/fixture"}, AppliedManifestHash: map[string]string{"target": cpromote.ManifestHash(nil)}}
			if err := cpromote.SaveAttempt(root, rec); err != nil {
				t.Fatal(err)
			}
			state := deploy.StrategyState{Target: rec.Target, StartedAt: when, PreDeployTars: map[string]string{"target": "/srv/fixture/.nself/backups/pre-deploy-fixture.tar.gz"}, PreDeployChanged: map[string][]string{"target": {"docker-compose.yml"}}}
			if !matching {
				state.StartedAt = when.Add(time.Second)
			}
			if err := deploy.SaveDeployState(root, state); err != nil {
				t.Fatal(err)
			}
			got, err := cpromote.LoadLatestRecord(root, rec.Target)
			if !matching {
				if err == nil || !strings.Contains(err.Error(), "E485") || !strings.Contains(err.Error(), rec.BackupID) {
					t.Fatalf("mismatched fallback must refuse naming DB backup: %v", err)
				}
				return
			}
			if err != nil || got == nil || got.PreDeployTar["target"] != state.PreDeployTars["target"] {
				t.Fatalf("same-attempt fallback lost file recovery: %#v %v", got, err)
			}
		})
	}
}
