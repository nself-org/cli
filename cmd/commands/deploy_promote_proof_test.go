package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
)

func promoteProofFixture(t *testing.T) (*promoteWiring, *cpromote.State, *promoteSafetyRemote) {
	t.Helper()
	locked := compose.LockedImages()[0]
	repo, tag := splitImageTag(locked.LegacyRef)
	r := &promoteSafetyRemote{label: "prod-app", repository: repo, tag: tag, imageID: "sha256:running-target", index: locked.IndexDigest}
	w := &promoteWiring{workdir: t.TempDir(), targetRemote: r, targetServer: controlplane.Server{Host: "example.invalid", RemotePath: "/srv/fixture"}, started: time.Now()}
	st := &cpromote.State{Source: "ls", Target: "prodsim", BackupHost: r.label, BackupID: "fixture_backup", MigrateStarted: true, Values: map[string]any{}, Digests: []cpromote.DigestRow{{Service: "api", Source: locked.Platforms["linux/amd64"], Target: locked.Platforms["linux/amd64"], Status: "equal"}}}
	return w, st, r
}

func TestPromoteMigrateNeverStarted(t *testing.T) {
	for _, reason := range []string{"cancelled", "checkpoint-save"} {
		t.Run(reason, func(t *testing.T) {
			w, st, r := promoteProofFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if reason == "cancelled" {
				cancel()
			} else {
				if err := os.WriteFile(filepath.Join(w.workdir, ".nself"), []byte("blocked"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			step := checkpointPromoteStep(w, cpromote.MigrateStep(r, "/srv/fixture"), true)
			if err := step.Run(ctx, st); err == nil || st.MigrateStarted {
				t.Fatalf("err=%v migrate_started=%v; unsent migration must not authorize DB restore", err, st.MigrateStarted)
			}
			if strings.Contains(strings.Join(r.log, "\n"), "db migrate") {
				t.Fatal("migration was sent")
			}
		})
	}
}

func TestPromotePostApplyProofBeforeMigrate(t *testing.T) {
	for _, drift := range []bool{false, true} {
		t.Run(map[bool]string{false: "equal", true: "drift"}[drift], func(t *testing.T) {
			w, st, r := promoteProofFixture(t)
			if drift {
				st.Digests[0].Target = "sha256:" + strings.Repeat("b", 64)
			}
			step := checkpointPromoteStep(w, cpromote.MigrateStep(r, "/srv/fixture"), true)
			err := step.Run(context.Background(), st)
			log := strings.Join(r.log, "\n")
			if drift {
				if err == nil || !strings.Contains(err.Error(), "E490") || st.MigrateStarted || strings.Contains(log, "db migrate") {
					t.Fatalf("drift must fail before migration: err=%v migrate_started=%v commands=%s", err, st.MigrateStarted, log)
				}
			} else if err != nil || !strings.Contains(log, "docker image inspect") || !strings.Contains(log, "db migrate") {
				t.Fatalf("equal identity must be proved before migration: err=%v commands=%s", err, log)
			}
		})
	}
}

func TestPromotePostApplyDriftRestoresFilesOnly(t *testing.T) {
	w, st, r := promoteProofFixture(t)
	st.Digests[0].Target = "sha256:" + strings.Repeat("b", 64)
	var events []string
	steps := make([]cpromote.Step, len(cpromote.StepNames))
	for i, name := range cpromote.StepNames {
		steps[i] = cpromote.Step{Name: name, Run: func(context.Context, *cpromote.State) error { return nil }}
		switch name {
		case "snapshot":
			steps[i].Undo = func(context.Context, *cpromote.State) error { events = append(events, "db"); return nil }
		case "apply":
			steps[i].Undo = func(context.Context, *cpromote.State) error { events = append(events, "files"); return nil }
		case "migrate":
			steps[i] = checkpointPromoteStep(w, cpromote.MigrateStep(r, "/srv/fixture"), true)
		}
	}
	result, err := (cpromote.Engine{Steps: steps}).Run(context.Background(), st)
	if err == nil || !strings.Contains(err.Error(), "E488") || result.FailedStep != "migrate" || result.Status != "rolled_back" || strings.Join(events, ",") != "files" || st.MigrateStarted {
		t.Fatalf("result=%+v err=%v recovery=%v migrate_started=%v", result, err, events, st.MigrateStarted)
	}
	if strings.Contains(strings.Join(r.log, "\n"), "db migrate") {
		t.Fatal("migration was sent after post-apply drift")
	}
}
