package commands

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
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
			stageWritePlugin(t, dir, "traefik")
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
	stageWritePlugin(t, dir, "traefik")
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

// TestInstallStageCreatedFilters: dot dirs, .prev backups and files are ignored.
func TestInstallStageCreatedFilters(t *testing.T) {
	dir := t.TempDir()
	stage := beginInstallStage(filepath.Join(dir, "missing"))
	if got := stage.created(); len(got) != 0 {
		t.Fatalf("missing dir: created = %v", got)
	}
	stage = beginInstallStage(dir)
	for _, n := range []string{"b", "a", ".hidden", "x.prev"} {
		if err := os.MkdirAll(filepath.Join(dir, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "afile"), nil, 0o644); err != nil {
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
	stageWritePlugin(t, dir, "traefik")
	err := stage.reconcile(context.Background(), &config.Config{}, &bytes.Buffer{}, func() error { return errs.New("E480", "no provider") })
	if err == nil || !strings.Contains(err.Error(), "undo broke") {
		t.Fatalf("undo failure not reported: %v", err)
	}
	if d := errs.Describe(err); d.Code != "E480" {
		t.Errorf("Describe code = %q, want E480", d.Code)
	}
}
