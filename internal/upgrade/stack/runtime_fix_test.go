package stack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/reconcile"
)

func TestUpdateStackRuntimeStepTimeouts(t *testing.T) {
	for _, name := range []string{"pull", "migrate", "build", "start", "readiness", "health"} {
		want := 10 * time.Minute
		if name == "pull" || name == "migrate" {
			want = 30 * time.Minute
		}
		if got := stepTimeout(name); got != want {
			t.Fatalf("%s timeout=%s want=%s", name, got, want)
		}
	}
	old := runtimeMigrateUp
	t.Cleanup(func() { runtimeMigrateUp = old })
	runtimeMigrateUp = func(ctx context.Context, _ *config.Config, _ string) (int, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) < 29*time.Minute {
			t.Fatal("migrate received short deadline")
		}
		return 0, nil
	}
	if err := NewRuntime(t.TempDir(), &config.Config{}, nil).Run(context.Background(), "migrate", &Journal{}); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateStackRuntimeBackupCreateFailure(t *testing.T) {
	old := runtimeBackupCreate
	t.Cleanup(func() { runtimeBackupCreate = old })
	want := errors.New("dump failed")
	runtimeBackupCreate = func(context.Context, *config.Config, backup.CreateOptions) error { return want }
	r := NewRuntime(t.TempDir(), &config.Config{}, nil)
	j := &Journal{RunID: "run"}
	if err := r.createRunBackup(context.Background(), j); !errors.Is(err, want) || !strings.Contains(err.Error(), "create stack backup") {
		t.Fatalf("err=%v", err)
	}
	if j.BackupID != "" || j.BackupSHA256 != "" {
		t.Fatal("failed backup acquired evidence")
	}
}

func TestUpdateStackRuntimeIdentifyRunBackup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		files  []string
		before map[string]bool
		old    bool
		want   string
	}{
		{"several backups", []string{"prior_stack-run.dump", "other.dump", "new_stack-run.dump"}, map[string]bool{"prior_stack-run.dump": true}, false, ""},
		{"multiple owned backups", []string{"a_stack-run.dump", "b_stack-run.dump"}, nil, false, "multiple backups"},
		{"predates run", []string{"old_stack-run.dump"}, nil, true, "predates"},
		{"no owned backup", []string{"other.dump"}, nil, false, "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			r := NewRuntime(dir, &config.Config{Backup: config.BackupConfig{Dir: dir}}, nil)
			j := &Journal{StartedAt: time.Now().Add(-time.Minute)}
			after := map[string]bool{}
			for _, name := range tc.files {
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte("PGDMP-test"), 0600); err != nil {
					t.Fatal(err)
				}
				if tc.old {
					if err := os.Chtimes(path, j.StartedAt.Add(-time.Hour), j.StartedAt.Add(-time.Hour)); err != nil {
						t.Fatal(err)
					}
				}
				after[name] = true
			}
			err := r.identifyRunBackup(j, tc.before, after, "stack-run")
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("err=%v want=%s", err, tc.want)
				}
				return
			}
			if err != nil || j.BackupID != "new_stack-run.dump" {
				t.Fatalf("id=%s err=%v", j.BackupID, err)
			}
		})
	}
}

func TestUpdateStackRuntimeVerifyFalse(t *testing.T) {
	dir := t.TempDir()
	r := NewRuntime(dir, &config.Config{Backup: config.BackupConfig{Dir: dir}}, nil)
	if err := os.WriteFile(filepath.Join(dir, "proof.dump"), []byte("PGDMP-proof"), 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := r.backupDigest("proof.dump")
	if err != nil {
		t.Fatal(err)
	}
	old := runtimeBackupVerify
	t.Cleanup(func() { runtimeBackupVerify = old })
	runtimeBackupVerify = func(context.Context, *config.Config, backup.VerifyOptions) (*backup.VerifyResult, error) {
		return &backup.VerifyResult{Verified: false}, nil
	}
	j := &Journal{StartedAt: time.Now().Add(-time.Minute), BackupCreatedAt: time.Now(), BackupID: "proof.dump", BackupSHA256: digest}
	if err := r.VerifyBackup(context.Background(), j); err == nil || !strings.Contains(err.Error(), "not verified") {
		t.Fatalf("err=%v", err)
	}
	if !j.BackupVerifiedAt.IsZero() {
		t.Fatal("unverified dump gained verified timestamp")
	}
}

func TestUpdateStackRuntimeProdBuildErrors(t *testing.T) {
	oldCompute, oldApply := runtimeCompute, runtimeApplyBuild
	t.Cleanup(func() { runtimeCompute, runtimeApplyBuild = oldCompute, oldApply })
	want := errors.New("fixture failure")
	for _, stage := range []string{"preview", "apply"} {
		t.Run(stage, func(t *testing.T) {
			calls := 0
			runtimeCompute = func(context.Context, reconcile.Request) (*reconcile.Plan, error) {
				if stage == "preview" {
					return nil, want
				}
				return &reconcile.Plan{}, nil
			}
			runtimeApplyBuild = func(_ context.Context, req reconcile.Request, opt reconcile.ApplyOptions) (*reconcile.Plan, *build.BuildResult, error) {
				calls++
				if req.Command != reconcile.CmdBuild || !opt.Yes {
					t.Fatal("lost authorized build inputs")
				}
				return nil, nil, want
			}
			err := NewRuntime(t.TempDir(), &config.Config{Env: "staging"}, nil).build(context.Background())
			if !errors.Is(err, want) || !strings.Contains(err.Error(), stage+" build") {
				t.Fatalf("err=%v", err)
			}
			if stage == "preview" && calls != 0 {
				t.Fatal("applied after failed preview")
			}
		})
	}
}

func TestUpdateStackRuntimePrivateStateMatchesJournal(t *testing.T) {
	dir := t.TempDir()
	r := NewRuntime(dir, &config.Config{}, nil)
	got, err := r.privateState()
	if err != nil {
		t.Fatal(err)
	}
	want, err := stateDirectory(dir, false)
	if err != nil || got != want {
		t.Fatalf("state=%s want=%s err=%v", got, want, err)
	}
	for _, name := range []string{".nself", "state"} {
		t.Run(name+" symlink", func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".nself")
			if name == "state" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(path, "state")
			}
			if err := os.Symlink(t.TempDir(), path); err != nil {
				t.Fatal(err)
			}
			_, wantErr := stateDirectory(dir, true)
			_, gotErr := NewRuntime(dir, &config.Config{}, nil).privateState()
			if wantErr == nil || gotErr == nil || gotErr.Error() != wantErr.Error() {
				t.Fatalf("snapshot state does not share journal refusal: got=%v want=%v", gotErr, wantErr)
			}
		})
	}
}
