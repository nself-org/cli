package plugin

// Purpose: table-driven tests for ValidateName and for the library entry
// points that join a plugin name to the plugin directory (P7-PLUG-20 fix).
// Inputs: temp plugin directories; no network, no Docker.
// Constraints: the traversal tests build a victim directory NEXT TO the plugin
// directory, so a missing guard deletes or modifies it.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

func TestValidateName(t *testing.T) {
	valid := []string{
		"notify", "cron", "claw", "chat", "ai", "content-acquisition",
		"pentest-kit", "dup-a", "py", "a1", "9lives", "notify.prev",
		"my_plugin", strings.Repeat("a", maxPluginNameLen),
	}
	for _, n := range valid {
		if err := ValidateName(n); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", n, err)
		}
	}

	invalid := []struct{ name, in string }{
		{"empty", ""},
		{"dot", "."},
		{"dotdot", ".."},
		{"dot slash", "./py"},
		{"parent traversal", "../victim"},
		{"deep traversal", "../../.ssh"},
		{"trailing slash", "py/"},
		{"nested", "a/b"},
		{"backslash", `a\b`},
		{"backslash traversal", `..\victim`},
		{"absolute", "/etc"},
		{"absolute home", "/Users/x/.ssh"},
		{"leading dash", "-rf"},
		{"leading dot", ".hidden"},
		{"trailing dot", "py."},
		{"nul", "py\x00"},
		{"newline", "py\n"},
		{"tab", "py\t"},
		{"del", "py\x7f"},
		{"space", "my plugin"},
		{"uppercase", "Py"},
		{"leading underscore", "_py"},
		{"tilde", "~"},
		{"url", "https://x.test/p.tgz"},
		{"too long", strings.Repeat("a", maxPluginNameLen+1)},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateName(tc.in)
			if err == nil {
				t.Fatalf("ValidateName(%q) = nil, want an error", tc.in)
			}
			if d := errs.Describe(err); d == nil || d.Code != "E401" {
				t.Errorf("ValidateName(%q) error = %v, want code E401", tc.in, err)
			}
		})
	}
}

// victimFixture returns pluginDir and a sibling directory outside it that holds a marker file.
func victimFixture(t *testing.T) (pluginDir, victim string) {
	t.Helper()
	root := t.TempDir()
	pluginDir = filepath.Join(root, "plugins")
	victim = filepath.Join(root, "victim")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victim, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	return pluginDir, victim
}

func assertVictimIntact(t *testing.T, victim string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(victim, "keep.txt")); err != nil {
		t.Fatalf("the directory outside the plugin dir was damaged: %v", err)
	}
}

// TestRemoveRefusesPathNames is the P7-PLUG-20 regression: Remove used to
// RemoveAll(filepath.Join(pluginDir, name)) for any name, so "../victim" deleted
// a sibling of the plugin directory.
func TestRemoveRefusesPathNames(t *testing.T) {
	pluginDir, victim := victimFixture(t)
	for _, name := range []string{"../victim", "../victim/", "./../victim", "..", ".", victim} {
		t.Run(name, func(t *testing.T) {
			err := Remove(context.Background(), &config.Config{}, name, pluginDir, true, true)
			if err == nil {
				t.Fatalf("Remove(%q) = nil, want a refusal", name)
			}
			if d := errs.Describe(err); d == nil || d.Code != "E401" {
				t.Errorf("Remove(%q) error = %v, want code E401", name, err)
			}
			assertVictimIntact(t, victim)
		})
	}
}

// TestPathNamesRefusedEverywhere covers the other entry points that join a name.
func TestPathNamesRefusedEverywhere(t *testing.T) {
	pluginDir, victim := victimFixture(t)
	ctx := context.Background()
	cases := map[string]func(name string) error{
		"Update":  func(n string) error { return Update(ctx, &config.Config{}, n, pluginDir) },
		"Install": func(n string) error { return Install(ctx, &config.Config{}, n, pluginDir) },
		"Disable": func(n string) error { return DisablePlugin(n, pluginDir) },
		"Enable":  func(n string) error { return EnablePlugin(n, pluginDir) },
		"Start":   func(n string) error { return Start(ctx, pluginDir, n) },
		"Stop":    func(n string) error { return Stop(ctx, n) },
		"Status":  func(n string) error { _, err := Status(n); return err },
	}
	for entry, call := range cases {
		for _, name := range []string{"../victim", "./py", "a/b", ""} {
			err := call(name)
			if d := errs.Describe(err); d == nil || d.Code != "E401" {
				t.Errorf("%s(%q) error = %v, want code E401", entry, name, err)
			}
			assertVictimIntact(t, victim)
		}
	}
	if _, err := os.Stat(filepath.Join(victim, ".disabled")); err == nil {
		t.Error("Disable wrote a marker outside the plugin dir")
	}
	if IsDisabled("../victim", pluginDir) {
		t.Error("IsDisabled answered for a path-shaped name")
	}
}
