package promote

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/oplock"
)

func TestPromoteRecordUnsupportedLock(t *testing.T) {
	ctx := context.Background()
	got, release, err := acquireRecordLock(ctx, t.TempDir(), func(context.Context, string, oplock.Opts) (*oplock.Lock, error) {
		return nil, oplock.ErrUnsupported
	})
	if err != nil || got != ctx || release == nil {
		t.Fatalf("unsupported platform fallback: context=%v release=%v err=%v", got, release != nil, err)
	}
	release()
}

func TestPromoteRecordDeployIdentity(t *testing.T) {
	for _, delta := range []time.Duration{-time.Hour, time.Hour} {
		t.Run(delta.String(), func(t *testing.T) {
			root := t.TempDir()
			when := time.Unix(10000, 0).UTC()
			state := deploy.StrategyState{Target: "prodsim", StartedAt: when.Add(-time.Minute), CompletedAt: when.Add(delta), PreDeployTar: "/srv/app/.nself/backups/pre-deploy-1.tar.gz"}
			if err := deploy.SaveDeployState(root, state); err != nil {
				t.Fatal(err)
			}
			rec := Record{ID: "identity", Target: "prodsim", Status: "ok", CompletedAt: when, BackupID: "backup-1", BackupHost: "primary", MigrateStarted: true, PreDeployTar: map[string]string{"primary": state.PreDeployTar}, Changed: map[string][]string{"primary": {}}}
			if err := SaveRecord(root, rec); err != nil {
				t.Fatal(err)
			}
			got, err := LoadLatestRecord(root, rec.Target)
			if err != nil || got == nil {
				t.Fatalf("same deploy identity hidden by clock: %#v %v", got, err)
			}
			if got.DeployIdentity == nil || !got.DeployIdentity.StartedAt.Equal(state.StartedAt) {
				t.Fatalf("deploy identity was not persisted: %#v", got.DeployIdentity)
			}
			got.RemotePaths = map[string]string{"primary": "/srv/app"}
			got.AppliedManifestHash = map[string]string{"primary": ManifestHash(nil)}
			if err := SaveRecord(root, *got); err != nil {
				t.Fatal(err)
			}
			r := &recordRemote{label: "primary"}
			if err := RollbackRecord(context.Background(), root, got, map[string]deploy.Remote{"primary": r}, map[string]string{"primary": "/srv/app"}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(r.calls, "\n"), "backup restore 'backup-1'") {
				t.Fatal("same identity did not restore DB")
			}
			state.StartedAt = state.StartedAt.Add(time.Second)
			state.CompletedAt = got.CompletedAt.Add(delta)
			if err := deploy.SaveDeployState(root, state); err != nil {
				t.Fatal(err)
			}
			// Saving the same record must not rebind it to the new operation.
			if err := SaveRecord(root, *got); err != nil {
				t.Fatal(err)
			}
			if got, err := LoadLatestRecord(root, rec.Target); err != nil || got != nil {
				t.Fatalf("different deploy identity remained applicable: %#v %v", got, err)
			}
		})
	}
}

func TestPromoteRecordCorruptRepairHint(t *testing.T) {
	root := t.TempDir()
	if err := deploy.SaveDeployState(root, deploy.StrategyState{Target: "prodsim", PreDeployTar: "/tmp/pre-deploy-fixture.tar.gz"}); err != nil {
		t.Fatal(err)
	}
	deployFile := filepath.Join(root, deploy.DeployStateFile)
	deployRaw, err := os.ReadFile(deployFile)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, RecordFile)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte("{broken")
	if err := os.WriteFile(file, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = LoadLatestRecord(root, "prodsim")
	for _, hint := range []string{"E485", RecordFile, "move it aside", "deploy rollback", "deploy.json", "nothing was changed"} {
		if err == nil || !strings.Contains(err.Error(), hint) {
			t.Fatalf("missing %q repair hint: %v", hint, err)
		}
	}
	got, readErr := os.ReadFile(file)
	if readErr != nil || string(got) != string(raw) {
		t.Fatalf("corrupt record changed: %q %v", got, readErr)
	}
	got, readErr = os.ReadFile(deployFile)
	if readErr != nil || string(got) != string(deployRaw) {
		t.Fatalf("deploy record changed: %q %v", got, readErr)
	}
}

func TestPromoteLegacyRecordDeployIdentity(t *testing.T) {
	root := t.TempDir()
	rec := Record{ID: "legacy", Target: "prodsim", Status: "ok", CompletedAt: time.Unix(100, 0), PreDeployTar: map[string]string{"primary": "/tmp/pre-deploy-fixture.tar.gz"}, Changed: map[string][]string{"primary": {}}}
	if err := SaveRecord(root, rec); err != nil {
		t.Fatal(err)
	}
	state := deploy.StrategyState{Target: rec.Target, CompletedAt: rec.CompletedAt.Add(time.Hour), PreDeployTars: rec.PreDeployTar}
	if err := deploy.SaveDeployState(root, state); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadLatestRecord(root, rec.Target); err != nil || got == nil {
		t.Fatalf("legacy tar identity not applicable: %#v %v", got, err)
	}
	state.PreDeployTars = map[string]string{"primary": "/tmp/pre-deploy-other.tar.gz"}
	state.CompletedAt = rec.CompletedAt.Add(-time.Hour)
	if err := deploy.SaveDeployState(root, state); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadLatestRecord(root, rec.Target); err != nil || got != nil {
		t.Fatalf("different legacy tar identity applicable: %#v %v", got, err)
	}
}
