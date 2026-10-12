package commands

// Purpose: D-0330 and D-0504 at command level. v1.5 `nself remove <plugin>`
// keeps data unless --delete-data, which needs --yes or a terminal answer
// (E403), --force or not; v1.4 keeps its rule and warns when data goes by
// default. A project .env NSELF_PLUGIN_DIR picks the plugin directory.
// Inputs: chatFixture (plugins bots and livekit, bundle chat recorded) plus a
// plugin no bundle references; plugin.Remove is swapped for a recorder.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// removeCall is one plugin.Remove call the command made.
type removeCall struct {
	name, dir string
	keep      bool
}

// recordPluginRemove swaps plugin.Remove for a recorder that deletes the plugin directory.
func recordPluginRemove(t *testing.T) *[]removeCall {
	t.Helper()
	var calls []removeCall
	orig := pluginRemoveFn
	pluginRemoveFn = func(_ context.Context, _ *config.Config, name, dir string, keep, _ bool) error {
		calls = append(calls, removeCall{name: name, dir: dir, keep: keep})
		return os.RemoveAll(filepath.Join(dir, name))
	}
	t.Cleanup(func() { pluginRemoveFn = orig })
	return &calls
}

// installSoloPlugin puts a plugin no bundle references into dir.
func installSoloPlugin(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"` + name + `","version":"1.0.0","description":"fixture","category":"utility","license":"MIT"}`
	if err := os.WriteFile(filepath.Join(dir, name, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPluginRemoveDataPolicyCmd(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		cases := []struct {
			flags      []string
			wantKeep14 bool
			wantKeep15 bool
			e403In15   bool   // v1.5 refuses with E403 and removes nothing
			warn14     string // v1.4 stderr must contain this
			warn15     string // v1.5 stderr must contain this
		}{
			{nil, false, true, false, "drops the plugin's schema/tables by default in v1.4 mode", ""},
			{[]string{"keep-data"}, true, true, false, "", "--keep-data is deprecated"},
			{[]string{"delete-data"}, false, false, true, "", ""},
			{[]string{"delete-data", "yes"}, false, false, false, "", ""},
		}
		for _, c := range cases {
			c := c
			t.Run(strings.Join(append([]string{"flags"}, c.flags...), "-"), func(t *testing.T) {
				p := chatFixture(t)
				installSoloPlugin(t, p.plugins, "solo")
				calls := recordPluginRemove(t)
				setFlags(t, removeCmd, c.flags...)
				var err error
				stderr := captureStderr(t, func() { err = runPluginRemove(removeCmd, []string{"solo"}) })
				for _, f := range c.flags {
					resetFlag(removeCmd, f)
				}
				if compat.V15() && c.e403In15 {
					if d := errs.Describe(err); d == nil || d.Code != "E403" || errs.ExitCodeFor(err) != 4 {
						t.Errorf("%v: err = %v, want E403 exit 4", c.flags, err)
					}
					if len(*calls) != 0 {
						t.Errorf("%v: an unconfirmed --delete-data removed the plugin: %+v", c.flags, *calls)
					}
					return
				}
				if err != nil {
					t.Errorf("%v: %v", c.flags, err)
					return
				}
				want, warn := c.wantKeep14, c.warn14
				if compat.V15() {
					want, warn = c.wantKeep15, c.warn15
				}
				if len(*calls) != 1 || (*calls)[0].keep != want {
					t.Errorf("%v: calls = %+v, want one with keep=%v", c.flags, *calls, want)
				}
				if warn != "" && !strings.Contains(stderr, warn) {
					t.Errorf("%v: stderr lacks %q:\n%s", c.flags, warn, stderr)
				}
				if compat.V15() && c.flags == nil && strings.Contains(stderr, "warning:") {
					t.Errorf("v1.5 default remove warned:\n%s", stderr)
				}
			})
		}
	})
}

// TestPluginRemoveForceBundleMemberDataCmd: --force past E133 never drops a bundle
// member's data silently: v1.5 keeps it and asks before --delete-data, v1.4 warns.
func TestPluginRemoveForceBundleMemberDataCmd(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p := chatFixture(t)
		calls := recordPluginRemove(t)
		setFlags(t, removeCmd, "force", "delete-data")
		var err error
		captureStderr(t, func() { err = runPluginRemove(removeCmd, []string{"bots"}) })
		resetFlag(removeCmd, "delete-data")
		if compat.V15() {
			if d := errs.Describe(err); d == nil || d.Code != "E403" {
				t.Fatalf("v1.5 --force --delete-data: err = %v, want E403", err)
			}
			if len(*calls) != 0 {
				t.Fatalf("v1.5: an unconfirmed --delete-data removed bots: %+v", *calls)
			}
			l, _, lerr := bundle.LedgerStore(p.plugins).Load()
			if lerr != nil {
				t.Fatal(lerr)
			}
			if _, ok := l.Plugins["bots"]; !ok {
				t.Error("v1.5: a refused remove dropped bots from the ledger")
			}
		} else if err != nil || len(*calls) != 1 || (*calls)[0].keep {
			t.Fatalf("v1.4 --force --delete-data: err = %v, calls = %+v, want one drop", err, *calls)
		}
		*calls = nil
		stderr := captureStderr(t, func() { err = runPluginRemove(removeCmd, []string{"livekit"}) })
		if err != nil || len(*calls) != 1 {
			t.Fatalf("--force livekit: err = %v, calls = %+v", err, *calls)
		}
		if compat.V15() && !(*calls)[0].keep {
			t.Error("v1.5: --force dropped livekit's data with no --delete-data")
		}
		if !compat.V15() && !strings.Contains(stderr, "drops the plugin's schema/tables") {
			t.Errorf("v1.4: --force dropped livekit's data with no warning:\n%s", stderr)
		}
	})
}

func TestPluginRemoveDryRunDataLineCmd(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		chatFixture(t)
		calls := recordPluginRemove(t)
		for _, c := range []struct {
			flags  []string
			want14 string
			want15 string
		}{
			{nil, "data: drop plugin schema/tables (the default for plugin remove)", "data: preserve (the default from v1.5; --delete-data drops it)"},
			{[]string{"delete-data"}, "data: drop plugin schema/tables (--delete-data)", "data: drop plugin schema/tables (--delete-data; needs --yes or a terminal confirmation)"},
			{[]string{"keep-data"}, "data: preserve (--keep-data)", "data: preserve (the default from v1.5)"},
		} {
			setFlags(t, removeCmd, append([]string{"dry-run"}, c.flags...)...)
			var err error
			stderr := captureStderr(t, func() { err = runPluginRemove(removeCmd, []string{"bots"}) })
			for _, f := range c.flags {
				resetFlag(removeCmd, f)
			}
			want := c.want14
			if compat.V15() {
				want = c.want15
			}
			if err != nil || !strings.Contains(stderr, want) {
				t.Errorf("%v: err = %v, stderr lacks %q:\n%s", c.flags, err, want, stderr)
			}
		}
		if len(*calls) != 0 {
			t.Errorf("a dry run removed a plugin: %+v", *calls)
		}
	})
}

// TestPluginRemoveProjectPluginDirCmd (D-0504): the project .env NSELF_PLUGIN_DIR is
// read before the plugin directory is chosen, for the removal and the dry run. Only
// the project dir's ledger records solo in a bundle, so the plan proves the dir too.
func TestPluginRemoveProjectPluginDirCmd(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p := chatFixture(t)
		t.Setenv("NSELF_PLUGIN_DIR", "")
		projectDir := filepath.Join(t.TempDir(), "project-plugins")
		installSoloPlugin(t, projectDir, "solo")
		seedBundleLedger(t, projectDir, "chat", "solo")
		envPath := filepath.Join(p.project, ".env")
		body, err := os.ReadFile(envPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(envPath, append(body, []byte("\nNSELF_PLUGIN_DIR="+projectDir+"\n")...), 0o600); err != nil {
			t.Fatal(err)
		}
		calls := recordPluginRemove(t)

		setFlags(t, removeCmd, "dry-run")
		stderr := captureStderr(t, func() { err = runPluginRemove(removeCmd, []string{"solo"}) })
		resetFlag(removeCmd, "dry-run")
		if err != nil || !strings.Contains(stderr, "bundles: referenced by chat") {
			t.Errorf("dry run: err = %v, want the plan for the project dir:\n%s", err, stderr)
		}
		t.Setenv("NSELF_PLUGIN_DIR", "") // the dry run exported the project value; the removal must load it again
		setFlags(t, removeCmd, "force")
		captureStderr(t, func() { err = runPluginRemove(removeCmd, []string{"solo"}) })
		if err != nil {
			t.Fatalf("remove: %v", err)
		}
		if len(*calls) != 1 || (*calls)[0].dir != projectDir {
			t.Errorf("calls = %+v, want one in %s", *calls, projectDir)
		}
	})
}

// TestPluginRemoveDryRunWithoutConfigCmd: a dry run still prints its plan when the
// project config does not load (it planned without one before D-0504).
func TestPluginRemoveDryRunWithoutConfigCmd(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p := chatFixture(t)
		calls := recordPluginRemove(t)
		if err := os.WriteFile(filepath.Join(p.project, ".env"), []byte("A=1\x00\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		setFlags(t, removeCmd, "dry-run")
		var err error
		stderr := captureStderr(t, func() { err = runPluginRemove(removeCmd, []string{"bots"}) })
		resetFlag(removeCmd, "dry-run")
		if err != nil || !strings.Contains(stderr, "did not load") || !strings.Contains(stderr, "(dry-run) no changes made.") {
			t.Errorf("dry run: err = %v, want a warning and the plan:\n%s", err, stderr)
		}
		if err = runPluginRemove(removeCmd, []string{"bots"}); err == nil || len(*calls) != 0 {
			t.Errorf("real remove with a broken config: err = %v, calls = %+v, want a config error and no removal", err, *calls)
		}
	})
}

// TestPluginRemoveLegacyVerbDataNoteCmd: v1.5 `nself plugin remove` (no --delete-data
// flag) keeps data and names the flag that drops it; v1.4 prints no such note.
func TestPluginRemoveLegacyVerbDataNoteCmd(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p := chatFixture(t)
		installSoloPlugin(t, p.plugins, "solo")
		calls := recordPluginRemove(t)
		var err error
		stderr := captureStderr(t, func() { err = runPluginRemove(pluginRemoveCmd, []string{"solo"}) })
		if err != nil || len(*calls) != 1 {
			t.Fatalf("err = %v, calls = %+v", err, *calls)
		}
		note := strings.Contains(stderr, "`nself remove <name> --delete-data` drops it")
		if compat.V15() && (!note || !(*calls)[0].keep) {
			t.Errorf("v1.5: keep = %v, want true and the note:\n%s", (*calls)[0].keep, stderr)
		}
		if !compat.V15() && note {
			t.Errorf("v1.4 printed the v1.5 note:\n%s", stderr)
		}
	})
}
