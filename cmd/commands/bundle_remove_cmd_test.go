package commands

// Purpose: P7-PLUG-20 command-level tests: the wiring of bundle remove
// (--delete-data confirmation), remove <plugin> (E133 guard, ledger forget) and
// remove <plugin> --dry-run, through the real RunE functions and cobra flags.
// Inputs: extensionFixture (project, plugin dir, HOME) plus the shared catalog
// fixture bundle "chat" = {bots, livekit}, recorded in the ledger.
// Constraints: tests run not on a TTY, so interactiveConfirm is nil.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

// chatFixture installs bots and livekit as plain plugins and records bundle chat.
func chatFixture(t *testing.T) *l03Project {
	t.Helper()
	resetCmdWriters() // earlier tests leave closed pipes or buffers as error writers
	p := extensionFixture(t, "")
	if err := bundle.LoadBytes([]byte(fixtureBundlesJSON)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bundle.LoadBytes([]byte(fixtureBundlesJSON)) })
	for _, pl := range []string{"bots", "livekit"} {
		dir := filepath.Join(p.plugins, pl)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := `{"name":"` + pl + `","version":"1.0.0","description":"fixture","category":"utility","license":"MIT"}`
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	seedBundleLedger(t, p.plugins, "chat", "bots", "livekit")
	return p
}

// resetCmdWriters drops output writers earlier tests set on the remove command tree
// (SetErr on the root or a parent leaks into every child), so output goes to os.Stderr.
func resetCmdWriters() {
	for _, c := range []*cobra.Command{RootCmd, bundleCmd, removeCmd, bundleRemoveCmd, pluginRemoveCmd, pluginInstallCmd, installCmd} {
		for p := c; p != nil; p = p.Parent() { // the writer is inherited from any ancestor
			p.SetErr(nil)
			p.SetOut(nil)
		}
	}
}

// resetFlag puts a flag back to its default and clears Changed.
func resetFlag(cmd *cobra.Command, name string) {
	if f := cmd.Flags().Lookup(name); f != nil {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	}
}

// setFlags sets bool flags on cmd and resets them when the test ends.
func setFlags(t *testing.T, cmd *cobra.Command, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := cmd.Flags().Set(n, "true"); err != nil {
			t.Fatal(err)
		}
		n := n
		t.Cleanup(func() { resetFlag(cmd, n) })
	}
}

func TestBundleRemoveDeleteDataNeedsYesCmd(t *testing.T) {
	for name, run := range map[string]func() error{
		"nself bundle remove": func() error { return runBundleRemove(bundleRemoveCmd, []string{"chat"}) },
		"nself remove":        func() error { return runRemove(removeCmd, []string{"chat"}) },
	} {
		run := run
		t.Run(name, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				p := chatFixture(t)
				setFlags(t, bundleRemoveCmd, "delete-data")
				setFlags(t, removeCmd, "delete-data")
				err := run()
				if err == nil || errs.ExitCodeFor(err) != 4 {
					t.Fatalf("err = %v (exit %d), want E403 exit 4", err, errs.ExitCodeFor(err))
				}
				if d := errs.Describe(err); d == nil || d.Code != "E403" {
					t.Errorf("detail = %+v, want E403", d)
				}
				for _, pl := range []string{"bots", "livekit"} {
					if _, serr := os.Stat(filepath.Join(p.plugins, pl)); serr != nil {
						t.Errorf("%s was removed by a refused remove: %v", pl, serr)
					}
				}
			})
		})
	}
}

func TestPluginRemoveGuardCmd(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p := chatFixture(t)
		stderr := captureStderr(t, func() {
			err := runPluginRemove(pluginRemoveCmd, []string{"bots"})
			if compat.V15() {
				if d := errs.Describe(err); err == nil || d == nil || d.Code != "E133" || !strings.Contains(err.Error(), "chat") {
					t.Fatalf("v1.5: err = %v, want E133 naming chat", err)
				}
				if _, serr := os.Stat(filepath.Join(p.plugins, "bots")); serr != nil {
					t.Fatalf("v1.5: the refused plugin was removed: %v", serr)
				}
				return
			}
			if err != nil {
				t.Fatalf("v1.4: %v", err)
			}
		})
		if !compat.V15() {
			if !strings.Contains(stderr, "chat") || !strings.Contains(stderr, "warning:") {
				t.Errorf("v1.4: want a warning naming chat, got:\n%s", stderr)
			}
			if _, serr := os.Stat(filepath.Join(p.plugins, "bots")); !os.IsNotExist(serr) {
				t.Errorf("v1.4: bots should be removed: %v", serr)
			}
			return
		}
		// --force removes it and the ledger forgets the record (ForgetPlugin ran)
		setFlags(t, pluginRemoveCmd, "force")
		if err := runPluginRemove(pluginRemoveCmd, []string{"bots"}); err != nil {
			t.Fatalf("--force: %v", err)
		}
		if _, serr := os.Stat(filepath.Join(p.plugins, "bots")); !os.IsNotExist(serr) {
			t.Errorf("--force: bots still there: %v", serr)
		}
		l, _, lerr := bundle.LedgerStore(p.plugins).Load()
		if lerr != nil {
			t.Fatal(lerr)
		}
		if _, ok := l.Plugins["bots"]; ok {
			t.Error("--force: the ledger still records bots")
		}
		if _, ok := l.Plugins["livekit"]; !ok {
			t.Error("--force: the ledger lost livekit")
		}
	})
}

func TestPluginRemoveDryRunCmd(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p := chatFixture(t)
		store := bundle.LedgerStore(p.plugins)
		before, err := os.ReadFile(store.Path())
		if err != nil {
			t.Fatal(err)
		}
		setFlags(t, removeCmd, "dry-run")
		var runErr error
		stderr := captureStderr(t, func() { runErr = runPluginRemove(removeCmd, []string{"bots"}) })
		if runErr != nil {
			t.Fatalf("dry run: %v", runErr)
		}
		if _, serr := os.Stat(filepath.Join(p.plugins, "bots", "plugin.json")); serr != nil {
			t.Errorf("a dry run removed the plugin: %v", serr)
		}
		after, _ := os.ReadFile(store.Path())
		if !bytes.Equal(before, after) {
			t.Error("a dry run changed the ledger")
		}
		want, data := "would proceed", "data: drop"
		if compat.V15() {
			want, data = "would refuse", "data: preserve" // D-0330: v1.5 keeps plugin data by default
		}
		for _, w := range []string{"(dry-run) no changes made.", "chat", want, data} {
			if !strings.Contains(stderr, w) {
				t.Errorf("dry-run output lacks %q:\n%s", w, stderr)
			}
		}
		// a dry run with --delete-data needs no confirmation; with --keep-data it is contradictory
		setFlags(t, removeCmd, "delete-data")
		if runErr = runPluginRemove(removeCmd, []string{"bots"}); runErr != nil {
			t.Errorf("--delete-data dry run failed: %v", runErr)
		}
		setFlags(t, removeCmd, "keep-data")
		if runErr = runPluginRemove(removeCmd, []string{"bots"}); runErr == nil || !strings.Contains(runErr.Error(), "cannot be used together") {
			t.Errorf("--delete-data with --keep-data: err = %v, want the contradictory-flags error", runErr)
		}
	})
}

// unloadedCatalog puts the process in the state of `nself remove <x>`: the bundle
// catalog is not loaded, and the loader is aimed at url with an empty cache.
func unloadedCatalog(t *testing.T, url string) {
	t.Helper()
	t.Setenv("NSELF_BUNDLES_URL", url)
	t.Setenv("NSELF_BUNDLES_CACHE_PATH", filepath.Join(t.TempDir(), "bundles-cache.json"))
	t.Cleanup(bundle.UnloadCatalogForTest())
}

func catalogFixtureServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(fixtureBundlesJSON))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// installChatPlugins puts bots and livekit on disk with no ledger file and no catalog loaded.
func installChatPlugins(t *testing.T) *l03Project {
	t.Helper()
	resetCmdWriters()
	p := extensionFixture(t, "")
	for _, pl := range []string{"bots", "livekit"} {
		dir := filepath.Join(p.plugins, pl)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := `{"name":"` + pl + `","version":"1.0.0","description":"fixture","category":"utility","license":"MIT"}`
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// TestPluginRemoveGuardNoLedgerNoCatalogCmd is the case the other cmd tests miss: no ledger
// file and a catalog nobody loaded. v1.5 loads it and refuses with E133; with no catalog
// reachable it refuses too, because it cannot prove the plugin is unreferenced.
func TestPluginRemoveGuardNoLedgerNoCatalogCmd(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		if !compat.V15() {
			return
		}
		t.Run("catalog reachable", func(t *testing.T) {
			p := installChatPlugins(t)
			unloadedCatalog(t, catalogFixtureServer(t))
			err := runPluginRemove(pluginRemoveCmd, []string{"bots"})
			if d := errs.Describe(err); err == nil || d == nil || d.Code != "E133" || !strings.Contains(err.Error(), "chat") {
				t.Fatalf("err = %v, want E133 naming chat", err)
			}
			if _, serr := os.Stat(filepath.Join(p.plugins, "bots")); serr != nil {
				t.Errorf("bots was removed: %v", serr)
			}
		})
		t.Run("catalog unreachable", func(t *testing.T) {
			p := installChatPlugins(t)
			dead := httptest.NewServer(http.NotFoundHandler())
			url := dead.URL
			dead.Close()
			unloadedCatalog(t, url)
			err := runPluginRemove(pluginRemoveCmd, []string{"bots"})
			if err == nil || !strings.Contains(err.Error(), "--force") {
				t.Fatalf("err = %v, want a refusal that names --force", err)
			}
			if _, serr := os.Stat(filepath.Join(p.plugins, "bots")); serr != nil {
				t.Errorf("bots was removed: %v", serr)
			}
		})
	})
}

// TestRemoveBundleRoutesWithoutCatalogCmd: `nself remove chat` with the catalog not loaded
// still reaches bundle remove (E403 for --delete-data without --yes), not plugin remove.
func TestRemoveBundleRoutesWithoutCatalogCmd(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p := installChatPlugins(t)
		unloadedCatalog(t, catalogFixtureServer(t))
		setFlags(t, removeCmd, "delete-data")
		err := runRemove(removeCmd, []string{"chat"})
		if err == nil || errs.ExitCodeFor(err) != 4 {
			t.Fatalf("err = %v (exit %d), want the bundle remove E403 exit 4", err, errs.ExitCodeFor(err))
		}
		if _, serr := os.Stat(filepath.Join(p.plugins, "bots")); serr != nil {
			t.Errorf("bots was removed: %v", serr)
		}
	})
}

// TestBundleRemoveKeepDataDeprecationCmd: --keep-data prints its deprecation line in v1.5 only.
func TestBundleRemoveKeepDataDeprecationCmd(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		chatFixture(t)
		setFlags(t, bundleRemoveCmd, "keep-data", "dry-run")
		var err error
		stderr := captureStderr(t, func() { err = runBundleRemove(bundleRemoveCmd, []string{"chat"}) })
		if err != nil {
			t.Fatal(err)
		}
		has := strings.Contains(stderr, "--keep-data is deprecated")
		if has != compat.V15() {
			t.Errorf("deprecation line present = %v in %s mode:\n%s", has, compat.Mode(), stderr)
		}
	})
}

// TestRemoveRoutePluginNamedLikeBundleCmd: claw is both a plugin and a bundle. With the
// bundle not recorded in the ledger, `nself remove claw` removes the plugin; once the
// ledger records the bundle it routes to bundle remove (E403 for --delete-data without --yes).
func TestRemoveRoutePluginNamedLikeBundleCmd(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p := chatFixture(t) // catalog loaded; plugin dir has bots+livekit, bundle chat recorded
		dir := filepath.Join(p.plugins, "claw")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := `{"name":"claw","version":"1.0.0","description":"fixture","category":"utility","license":"MIT"}`
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}

		// bundle claw (ai, claw, claw-web) is not recorded: the plugin route runs and removes the plugin
		if err := runRemove(removeCmd, []string{"claw"}); err != nil {
			t.Fatalf("plugin route: %v", err)
		}
		if _, serr := os.Stat(dir); !os.IsNotExist(serr) {
			t.Errorf("the claw plugin was not removed: %v", serr)
		}

		// bundle chat is recorded: `remove chat` is the bundle route
		setFlags(t, removeCmd, "delete-data")
		err := runRemove(removeCmd, []string{"chat"})
		if err == nil || errs.ExitCodeFor(err) != 4 {
			t.Fatalf("recorded bundle: err = %v (exit %d), want the bundle remove E403", err, errs.ExitCodeFor(err))
		}
	})
}
