package commands

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// stageWritePlugin writes a minimal tableless plugin (<dir>/<name>/plugin.json).
func stageWritePlugin(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"` + name + `","version":"1.0.0","description":"fake","category":"misc","license":"MIT","tables":[]}`
	if err := os.WriteFile(filepath.Join(dir, name, "plugin.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// stageExists reports whether <dir>/<name> exists.
func stageExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func stageInstallPlugin(t *testing.T, stage *installStage, names ...string) {
	t.Helper()
	if err := stage.install(context.Background(), func() error {
		for _, name := range names {
			stageWritePlugin(t, stage.pluginDir, name)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestInstallStageUndoesRefusedReconcile: when the reconcile is refused, the
// plugin this run created is removed (and forgotten in the ledger), a plugin
// that already existed is kept, and the refusal keeps its error code.
func TestInstallStageUndoesRefusedReconcile(t *testing.T) {
	for _, code := range []string{"E403", "E480", "E481"} {
		t.Run(code, func(t *testing.T) {
			fakeHome := t.TempDir()
			t.Setenv("HOME", fakeHome)
			t.Setenv("USERPROFILE", fakeHome)
			dir := t.TempDir()
			stageWritePlugin(t, dir, "keepme")
			store := bundle.LedgerStore(dir)
			if err := store.Update(func(l *ledger.Ledger) error {
				l.AddBundle("task", []ledger.InstalledPlugin{{Name: "traefik", Version: "1.0.0", Tier: ledger.TierFree}}, time.Now())
				return nil
			}); err != nil {
				t.Fatal(err)
			}

			stage := beginInstallStage(dir)
			stageInstallPlugin(t, stage, "traefik")
			var w bytes.Buffer
			err := stage.reconcile(context.Background(), &config.Config{}, &w, func() error {
				return errs.New(code, "refused")
			})
			if err == nil {
				t.Fatal("the refusal was swallowed")
			}
			if d := errs.Describe(err); d.Code != code {
				t.Errorf("Describe code = %q, want %q (err: %v)", d.Code, code, err)
			}
			if stageExists(dir, "traefik") {
				t.Error("traefik was left on disk after the refused reconcile")
			}
			if !stageExists(dir, "keepme") {
				t.Error("keepme existed before the stage and must stay")
			}
			if !strings.Contains(w.String(), "the build did not apply, so these plugins were removed again: traefik") {
				t.Errorf("missing notice, got %q", w.String())
			}
			l, _, lerr := store.Load()
			if lerr != nil {
				t.Fatal(lerr)
			}
			if _, ok := l.Plugins["traefik"]; ok {
				t.Error("the ledger still records traefik")
			}
		})
	}
}

// TestInstallStageKeepsOnSuccess: a clean reconcile removes nothing.
func TestInstallStageKeepsOnSuccess(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)
	dir := t.TempDir()
	stage := beginInstallStage(dir)
	stageInstallPlugin(t, stage, "traefik")
	var w bytes.Buffer
	if err := stage.reconcile(context.Background(), &config.Config{}, &w, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if !stageExists(dir, "traefik") || w.Len() != 0 {
		t.Errorf("a successful reconcile changed state: exists=%v out=%q", stageExists(dir, "traefik"), w.String())
	}
}

// TestInstallStagePreexistingNeverRemoved: a plugin present before the stage
// survives a refusal even when it is the one named by the refused reconcile.
func TestInstallStagePreexistingNeverRemoved(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)
	dir := t.TempDir()
	stageWritePlugin(t, dir, "traefik")
	stage := beginInstallStage(dir)
	var w bytes.Buffer
	err := stage.reconcile(context.Background(), &config.Config{}, &w, func() error { return errs.New("E403", "refused") })
	if err == nil || !stageExists(dir, "traefik") || w.Len() != 0 {
		t.Errorf("pre-existing plugin handled wrongly: err=%v exists=%v out=%q", err, stageExists(dir, "traefik"), w.String())
	}
}

// TestInstallStageCreatedFilters: shared bin, dot dirs, backups and files are ignored.
func TestInstallStageCreatedFilters(t *testing.T) {
	dir := t.TempDir()
	stage := beginInstallStage(filepath.Join(dir, "missing"))
	if got := stage.created(); len(got) != 0 {
		t.Fatalf("missing dir: created = %v", got)
	}
	stage = beginInstallStage(dir)
	if err := stage.install(context.Background(), func() error {
		for _, n := range []string{"b", "a", "bin", ".hidden", "x.prev"} {
			if err := os.MkdirAll(filepath.Join(dir, n), 0o755); err != nil {
				return err
			}
		}
		return os.WriteFile(filepath.Join(dir, "afile"), nil, 0o644)
	}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(stage.created(), ","); got != "a,b" {
		t.Errorf("created = %q, want a,b", got)
	}
}

// TestInstallStageJoinsUndoFailure: a failed undo is joined to, and does not
// replace, the reconcile error.
func TestInstallStageJoinsUndoFailure(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)
	dir := t.TempDir()
	prev := undoPluginInstall
	undoPluginInstall = func(context.Context, *config.Config, string, string) error { return errors.New("undo broke") }
	t.Cleanup(func() { undoPluginInstall = prev })

	stage := beginInstallStage(dir)
	stageInstallPlugin(t, stage, "traefik")
	err := stage.reconcile(context.Background(), &config.Config{}, &bytes.Buffer{}, func() error { return errs.New("E480", "no provider") })
	if err == nil || !strings.Contains(err.Error(), "undo broke") {
		t.Fatalf("undo failure not reported: %v", err)
	}
	if d := errs.Describe(err); d.Code != "E480" {
		t.Errorf("Describe code = %q, want E480", d.Code)
	}
}

// A successful second invocation must survive the first invocation's refusal.
func TestInstallStageConcurrentInstallRollback(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)
	dir := t.TempDir()
	stage := beginInstallStage(dir)
	stageInstallPlugin(t, stage, "refused")
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		stageInstallPlugin(t, beginInstallStage(dir), "successful")
	}()
	<-finished
	err := stage.reconcile(context.Background(), &config.Config{}, &bytes.Buffer{}, func() error {
		return errs.New("E403", "refused")
	})
	if err == nil {
		t.Fatal("the refusal was swallowed")
	}
	if stageExists(dir, "refused") {
		t.Error("the refused invocation's plugin was left on disk")
	}
	if !stageExists(dir, "successful") {
		t.Error("rollback removed another invocation's successful plugin install")
	}
}

func TestInstallStageCapturesDependenciesOnFailure(t *testing.T) {
	dir := t.TempDir()
	stageWritePlugin(t, dir, "existing")
	stage := beginInstallStage(dir)
	failed := errors.New("target download failed")
	err := stage.install(context.Background(), func() error {
		stageWritePlugin(t, dir, "dependency")
		return failed
	})
	if !errors.Is(err, failed) || strings.Join(stage.created(), ",") != "dependency" {
		t.Fatalf("failed install ownership: err=%v created=%v", err, stage.created())
	}
}

func TestInstallStagePreservesReplacement(t *testing.T) {
	dir := t.TempDir()
	stage := beginInstallStage(dir)
	stageInstallPlugin(t, stage, "replaced")
	if err := os.Rename(filepath.Join(dir, "replaced"), filepath.Join(dir, "old.prev")); err != nil {
		t.Fatal(err)
	}
	stageInstallPlugin(t, beginInstallStage(dir), "replaced")
	stageInstallPlugin(t, stage, "unrelated")
	if got := strings.Join(stage.created(), ","); got != "unrelated" {
		t.Fatalf("claimed another invocation's replacement: %v", got)
	}
}

func TestInstallStageTracksOwnReplacement(t *testing.T) {
	dir := t.TempDir()
	stage := beginInstallStage(dir)
	stageInstallPlugin(t, stage, "replaced")
	if err := stage.install(context.Background(), func() error {
		if err := os.Rename(filepath.Join(dir, "replaced"), filepath.Join(dir, "old.prev")); err != nil {
			return err
		}
		stageWritePlugin(t, dir, "replaced")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(stage.created(), ","); got != "replaced" {
		t.Fatalf("lost this invocation's replacement: %q", got)
	}
}

// The helper is another invocation holding the same operation lock as bundles.
func TestInstallStageWaitsBeforeOwnershipCapture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native Windows bundle operation lock is a no-op; locking is supported through WSL2")
	}
	if dir := os.Getenv("NSELF_STAGE_LOCK_HELPER"); dir != "" {
		err := bundle.LedgerStore(dir).Operation(context.Background(), "concurrent install", func() error {
			if err := os.WriteFile(filepath.Join(dir, ".ready"), nil, 0o600); err != nil {
				return err
			}
			deadline := time.Now().Add(10 * time.Second)
			for !stageExists(dir, ".release") {
				if time.Now().After(deadline) {
					return errors.New("parent did not release helper")
				}
				time.Sleep(10 * time.Millisecond)
			}
			stageWritePlugin(t, dir, "successful")
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	dir := t.TempDir()
	stage := beginInstallStage(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInstallStageWaitsBeforeOwnershipCapture$")
	child.Env = append(os.Environ(), "NSELF_STAGE_LOCK_HELPER="+dir)
	var output bytes.Buffer
	child.Stdout, child.Stderr = &output, &output
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = child.Wait() })
	for !stageExists(dir, ".ready") {
		if ctx.Err() != nil {
			t.Fatal("helper did not acquire operation lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, ".release"), nil, 0o600)
	}()
	if err := stage.install(ctx, func() error {
		if !stageExists(dir, "successful") {
			return errors.New("ownership capture ran before the competing invocation released its lock")
		}
		stageWritePlugin(t, dir, "refused")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err != nil {
		t.Fatalf("helper failed: %v\n%s", err, &output)
	}
	if got := strings.Join(stage.created(), ","); got != "refused" {
		t.Fatalf("ownership captured a waiting invocation: %q", got)
	}
}
