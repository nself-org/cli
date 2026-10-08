package commands

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/oplock"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/mount"
	"github.com/spf13/cobra"
)

func mountFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".nself", "plugins")
	for _, slug := range []string{"demo", "claw"} {
		b, err := os.ReadFile(filepath.Join("testdata", "mount", slug, "plugin.json"))
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, slug)
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "plugin.json"), b, 0644); err != nil {
			t.Fatal(err)
		}
		b, err = os.ReadFile(filepath.Join("testdata", "mount", "bin", "nself-"+slug))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "bin"), 0755); err != nil {
			t.Fatal(err)
		}
		name := "nself-" + slug
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		if err := os.WriteFile(filepath.Join(dir, "bin", name), b, 0755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func mountRoot(t *testing.T, dir string) (*cobra.Command, []mount.Spec) {
	t.Helper()
	specs, problems := mount.Discover(dir, canonTable.Verbs)
	if len(problems) != 0 || len(specs) != 2 {
		t.Fatalf("specs=%+v problems=%+v", specs, problems)
	}
	r := &cobra.Command{Use: "nself", SilenceUsage: true, SilenceErrors: true}
	mountInstalled(r, specs, nil)
	return r, specs
}

func TestMountInstalled(t *testing.T) {
	d := mountFixture(t)
	r, _ := mountRoot(t, d)
	for _, path := range [][]string{{"demo"}, {"demo", "sub"}, {"demo", "nested", "two"}} {
		c, _, err := r.Find(path)
		if err != nil || c == nil || c == r || !c.DisableFlagParsing || c.RunE == nil {
			t.Fatalf("path %v: %v %v", path, c, err)
		}
	}
}

func TestMountCollisions(t *testing.T) {
	r := &cobra.Command{Use: "nself"}
	r.AddCommand(&cobra.Command{Use: "start", Aliases: []string{"up"}})
	for _, name := range []string{"start", "up", "completion"} {
		_, p := planMount(r, []mount.Spec{{Slug: "demo", Command: name}})
		if len(p) != 1 || p[0].Code != "E405" || !strings.Contains(p[0].Message, "demo") {
			t.Fatalf("%s: %+v", name, p)
		}
	}
}

func TestMountPreRunIsolation(t *testing.T) {
	r := &cobra.Command{Use: "nself", PersistentPreRunE: func(*cobra.Command, []string) error { return errors.New("root hook") }}
	n := installedNode(mount.Spec{Slug: "demo", Command: "demo", Binary: "nself-demo"})
	r.AddCommand(n)
	if n.PersistentPreRunE == nil || n.PersistentPostRunE == nil || n.GroupID != groupPlugins {
		t.Fatalf("hooks/group absent: %+v", n)
	}
	if err := n.PersistentPreRunE(n, nil); err != nil {
		t.Fatal(err)
	}
	if err := n.PersistentPostRunE(n, nil); err != nil {
		t.Fatal(err)
	}
	// The real decorator must not wrap installed RunE in the project lock.
	called := false
	n.RunE = func(*cobra.Command, []string) error {
		called = true
		if os.Getenv(oplock.EnvToken) != "" {
			t.Fatal("lock token reached plugin")
		}
		return nil
	}
	originalRun := reflect.ValueOf(n.RunE).Pointer()
	installInvocationDecorator(r)
	if reflect.ValueOf(n.RunE).Pointer() != originalRun {
		t.Fatal("installed RunE was wrapped by invocation decorator")
	}
	if err := n.RunE(n, nil); err != nil || !called {
		t.Fatalf("wrapped run: called=%t err=%v", called, err)
	}
}

func TestMountBuiltinInstalledCollision(t *testing.T) {
	r := &cobra.Command{Use: "nself"}
	mountBuiltin(r, []builtinFamily{{slug: "admin", build: func() *cobra.Command { return &cobra.Command{Use: "admin"} }}})
	specs, problems := planMount(r, []mount.Spec{{Slug: "evil", Command: "admin"}})
	if len(specs) != 0 || len(problems) != 1 || problems[0].Code != "E405" {
		t.Fatalf("specs=%+v problems=%+v", specs, problems)
	}
}

func TestDoctorBuiltinInstalledCollision(t *testing.T) {
	d := mountFixture(t)
	t.Setenv("NSELF_PLUGIN_DIR", d)
	r := &cobra.Command{Use: "nself"}
	mountBuiltin(r, []builtinFamily{{slug: "admin", build: func() *cobra.Command { return &cobra.Command{Use: "demo"} }}})
	rows := checkPluginMount(r, false)
	if len(rows) != 1 || rows[0].Status != "warn" || !strings.Contains(rows[0].Message, "E405") {
		t.Fatalf("doctor rows: %+v", rows)
	}
}

func TestMountBreakoutOwner(t *testing.T) {
	saved := canonTable.Breakouts
	canonTable.Breakouts = []canonRowT{{From: []string{"ci"}, Plugin: "ci"}}
	t.Cleanup(func() { canonTable.Breakouts = saved })
	specs, problems := planMount(&cobra.Command{Use: "nself"}, []mount.Spec{{Slug: "evil", Command: "ci"}})
	if len(specs) != 0 || len(problems) != 1 || !strings.Contains(problems[0].Message, "ci") {
		t.Fatalf("specs=%+v problems=%+v", specs, problems)
	}
}

func TestMountProblemEscapesControls(t *testing.T) {
	for _, s := range []string{"a\x1b[2Jb", "a\nb"} {
		if got := safeMountText(s); strings.ContainsAny(got, "\x1b\n") {
			t.Fatalf("raw controls in %q", got)
		}
	}
}

func TestDoctorProblemEscapesControls(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows rejects control characters in file names")
	}
	d := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", d)
	slug := "bad\x1b[2J"
	if err := os.MkdirAll(filepath.Join(d, slug), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, slug, "plugin.json"), []byte("{"), 0644); err != nil {
		t.Fatal(err)
	}
	rows := checkPluginMount(&cobra.Command{Use: "nself"}, false)
	if len(rows) != 1 || strings.ContainsAny(rows[0].Name+rows[0].Message, "\x1b\n") {
		t.Fatalf("unsafe doctor row: %+v", rows)
	}
}

func TestMountUsesDiscoveredPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	root := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", root)
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bin, "nself-demo")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 7\n"), 0755); err != nil {
		t.Fatal(err)
	}
	n := installedNode(mount.Spec{Slug: "demo", Command: "demo", Binary: "nself-demo", BinaryPath: path})
	var exit *plugin.ExitCodeError
	if err := n.RunE(n, nil); !errors.As(err, &exit) || exit.Code != 7 {
		t.Fatalf("discovered path: %v", err)
	}
}

func TestMountNoCanonLoad(t *testing.T) {
	d := mountFixture(t)
	calls := 0
	saved := canonLoad
	canonLoad = func(v15 bool) (*canon.File, error) { calls++; return saved(v15) }
	t.Cleanup(func() { canonLoad = saved })
	specs, problems := mount.Discover(d, canonTable.Verbs)
	mountInstalled(&cobra.Command{Use: "nself"}, specs, problems)
	if calls != 0 {
		t.Fatalf("mount called canon loader %d times", calls)
	}
}

func TestMountJSONPassthrough(t *testing.T) {
	in := []string{"--flag", "x", "--json", "--no-monorepo"}
	got := stripRootPersistentFlags(in)
	if !reflect.DeepEqual(got, []string{"--flag", "x", "--json"}) {
		t.Fatalf("got %v", got)
	}
}

func TestMountBareBinaryFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".nself", "plugins", "bin")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nself-sentry-server"), []byte("#!/bin/sh\nexit 7\n"), 0755); err != nil {
		t.Fatal(err)
	}
	var exit *plugin.ExitCodeError
	if err := plugin.ProxyCommand("sentry-server", nil); !errors.As(err, &exit) || exit.Code != 7 {
		t.Fatalf("fallback: %v", err)
	}
}

func TestMountBuiltinFamily(t *testing.T) {
	d := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", d)
	r := &cobra.Command{Use: "nself"}
	makeFamily := func() *cobra.Command {
		return &cobra.Command{Use: "fixture", RunE: func(*cobra.Command, []string) error { return nil }}
	}
	mountBuiltin(r, []builtinFamily{{slug: "fixture", build: makeFamily}})
	c, _, _ := r.Find([]string{"fixture"})
	if c == r || c.Annotations[mount.AnnSource] != sourceBuiltin {
		t.Fatalf("builtin: %+v", c)
	}
	p := filepath.Join(d, "fixture")
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, ".disabled"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	r = &cobra.Command{Use: "nself"}
	mountBuiltin(r, []builtinFamily{{slug: "fixture", build: makeFamily}})
	if len(r.Commands()) != 0 {
		t.Fatal("disabled builtin mounted")
	}
}

func TestMountBuiltinExecutesGroupedCommand(t *testing.T) {
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	r := &cobra.Command{Use: "nself"}
	r.AddCommand(&cobra.Command{Use: "access", GroupID: groupAdvanced, RunE: func(*cobra.Command, []string) error { return nil }})
	mountBuiltin(r, []builtinFamily{{slug: "fixture", build: func() *cobra.Command {
		return &cobra.Command{Use: "fixture", RunE: func(*cobra.Command, []string) error { return nil }}
	}}})
	r.SetArgs([]string{"access"})
	if err := r.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestMountAnnotations(t *testing.T) {
	d := mountFixture(t)
	r, _ := mountRoot(t, d)
	s, _, _ := r.Find([]string{"demo", "sub"})
	n, _, _ := r.Find([]string{"demo", "nested", "two"})
	if s.Annotations[mount.AnnConfirm] != `{"flags":["yes"]}` || n.Annotations[mount.AnnSurface] != "cli-only" || n.Annotations[mount.AnnConfirm] != "" {
		t.Fatalf("sub=%v nested=%v", s.Annotations, n.Annotations)
	}
}

func TestDoctorPluginMount(t *testing.T) {
	d := mountFixture(t)
	t.Setenv("NSELF_PLUGIN_DIR", d)
	b, err := os.ReadFile(filepath.Join("testdata", "mount", "collide-completion", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(d, "collide-completion")
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "plugin.json"), b, 0644); err != nil {
		t.Fatal(err)
	}
	name := "nself-collide-completion"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(d, "bin", name), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	results := checkPluginMount(&cobra.Command{Use: "nself"}, false)
	if len(results) == 0 || results[0].Status != "warn" || !strings.Contains(results[0].Message, "E405") {
		t.Fatalf("doctor: %+v", results)
	}
}
