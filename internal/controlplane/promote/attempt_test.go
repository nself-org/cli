package promote

import (
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/deploy"
)

func TestPromoteInterruptedAttemptSupersededByDeployIdentity(t *testing.T) {
	for _, delta := range []time.Duration{-time.Hour, time.Hour} {
		t.Run(delta.String(), func(t *testing.T) {
			root := t.TempDir()
			when := time.Unix(10000, 0).UTC()
			state := deploy.StrategyState{Target: "prodsim", StartedAt: when.Add(delta),
				PreDeployTars: map[string]string{"primary": "/srv/app/.nself/backups/pre-deploy-live.tar.gz"}}
			if err := deploy.SaveDeployState(root, state); err != nil {
				t.Fatal(err)
			}
			live := Record{ID: "live", Target: state.Target, Status: "ok", StartedAt: when.Add(-2 * time.Hour), CompletedAt: when,
				PreDeployTar: state.PreDeployTars, Changed: map[string][]string{"primary": {}}}
			if err := SaveRecord(root, live); err != nil {
				t.Fatal(err)
			}
			attempt := Record{ID: "interrupted", Target: state.Target, Status: "in_progress", StartedAt: when,
				ApplyStarted: true, BackupID: "backup-attempt", DeployIdentity: &DeployIdentity{StartedAt: when}}
			if err := SaveAttempt(root, attempt); err != nil {
				t.Fatal(err)
			}
			got, err := LoadLatestRecord(root, state.Target)
			if err != nil || got == nil || got.ID != live.ID {
				t.Fatalf("different deploy identity must select live record independent of clocks: %#v %v", got, err)
			}
		})
	}
}

func TestPromoteInterruptedAttemptMissingEvidenceFix(t *testing.T) {
	root := t.TempDir()
	when := time.Unix(10000, 0).UTC()
	attempt := Record{ID: "interrupted", Target: "prodsim", Status: "in_progress", StartedAt: when,
		ApplyStarted: true, BackupID: "backup-attempt", DeployIdentity: &DeployIdentity{StartedAt: when}}
	if err := SaveAttempt(root, attempt); err != nil {
		t.Fatal(err)
	}
	if err := deploy.SaveDeployState(root, deploy.StrategyState{Target: attempt.Target, StartedAt: when}); err != nil {
		t.Fatal(err)
	}
	_, err := LoadLatestRecord(root, attempt.Target)
	if err == nil {
		t.Fatal("same identity without recovery tars must refuse")
	}
	text := err.Error()
	for _, hint := range []string{"E485", "backup-attempt", "Fix:", "on the target", "by hand"} {
		if !strings.Contains(text, hint) {
			t.Fatalf("missing recovery hint %q: %v", hint, err)
		}
	}
	if strings.Contains(text, "control-plane.yaml") || strings.Contains(text, "inventory violates") {
		t.Fatalf("interrupted recovery must not show inventory boilerplate: %v", err)
	}
}

func TestPromoteInterruptedAttemptSupersededWithoutLiveRecord(t *testing.T) {
	root := t.TempDir()
	when := time.Unix(10000, 0).UTC()
	attempt := Record{ID: "interrupted", Target: "prodsim", Status: "in_progress", StartedAt: when,
		ApplyStarted: true, BackupID: "backup-attempt", DeployIdentity: &DeployIdentity{StartedAt: when}}
	if err := SaveAttempt(root, attempt); err != nil {
		t.Fatal(err)
	}
	if err := deploy.SaveDeployState(root, deploy.StrategyState{Target: attempt.Target, StartedAt: when.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadLatestRecord(root, attempt.Target); err != nil || got != nil {
		t.Fatalf("superseded attempt must allow deploy.json rollback: %#v %v", got, err)
	}
}
