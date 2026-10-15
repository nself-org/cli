package commands

// Purpose: prove machineRegistry builds the v1.5 registry in both compat modes,
// mounts installed plugins, and puts the process tree back.
// Constraints: checks run on the shared RootCmd in-process. Restore mounted
// plugins and clear the registry cache so cases do not leak state.

import (
	"bytes"
	"os"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/plugin/mount"
)

// prepareMachineRegistryTest repairs parent pointers changed by other test
// harnesses and ensures neither this test nor the next uses a stale cache.
func prepareMachineRegistryTest(t *testing.T) {
	t.Helper()
	reattachRealTree()
	resetMachineRegistry()
	t.Cleanup(resetMachineRegistry)
}

func marshalOrFatal(t *testing.T, build func() (interface{ Marshal() ([]byte, error) }, error)) []byte {
	t.Helper()
	reg, err := build()
	if err != nil {
		t.Fatal(err)
	}
	b, err := reg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMachineRegistry(t *testing.T) {
	prepareMachineRegistryTest(t)
	committed, err := os.ReadFile(committedRegistry)
	if err != nil {
		t.Fatal(err)
	}
	want := generatedLine.ReplaceAll(committed, []byte("{\n"))
	compattest.Both(t, func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("NSELF_PLUGIN_DIR", "")
		resetMachineRegistry()
		mode := compat.Mode()
		base := marshalOrFatal(t, func() (interface{ Marshal() ([]byte, error) }, error) { return BuildRegistry(compat.V15()) })

		reg, err := machineRegistry()
		if err != nil {
			t.Fatal(err)
		}
		got, err := reg.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s: the machine registry differs from %s. First difference: %s", mode, committedRegistry, firstDiff(got, want))
		}
		if compat.Mode() != mode {
			t.Fatalf("machineRegistry left the compat mode at %s, was %s", compat.Mode(), mode)
		}
		if again, _ := machineRegistry(); again != reg {
			t.Error("machineRegistry must be memoised")
		}
		after := marshalOrFatal(t, func() (interface{ Marshal() ([]byte, error) }, error) { return BuildRegistry(compat.V15()) })
		if !bytes.Equal(after, base) {
			t.Fatalf("%s: the tree was not restored: %s", mode, firstDiff(after, base))
		}
	})
}

func TestMachineRegistryPluginMount(t *testing.T) {
	prepareMachineRegistryTest(t)
	compattest.Both(t, func(t *testing.T) {
		dir := mountFixture(t)
		t.Setenv("NSELF_PLUGIN_DIR", dir)
		resetMachineRegistry()
		base := marshalOrFatal(t, func() (interface{ Marshal() ([]byte, error) }, error) { return BuildRegistry(compat.V15()) })
		// os.Args[1] is a test flag here: the mount hook would skip, the machine registry must not.
		reg, err := machineRegistry()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{"demo", "demo sub", "demo nested two"} {
			c, ok := reg.Lookup(p)
			if !ok || c.Canon != canon.CanonPlugin || c.Plugin == nil || *c.Plugin != "demo" {
				t.Fatalf("%s: plugin command %q missing or not canon plugin: %+v", compat.Mode(), p, c)
			}
		}
		sub, _ := reg.Lookup("demo sub")
		if sub.Confirm == nil || len(sub.Confirm.Flags) != 1 || sub.Confirm.Flags[0] != "yes" || sub.SideEffect != "write" {
			t.Errorf("the plugin's confirm declaration did not reach the registry: %+v", sub)
		}
		if nested, _ := reg.Lookup("demo nested two"); nested.Surface != "cli-only" {
			t.Errorf("surface cli-only lost: %+v", nested)
		}
		if n, _, _ := RootCmd.Find([]string{"demo"}); n != nil && n != RootCmd && n.Name() == "demo" {
			t.Error("the mounted plugin node must be removed from the process tree")
		}
		after := marshalOrFatal(t, func() (interface{ Marshal() ([]byte, error) }, error) { return BuildRegistry(compat.V15()) })
		if !bytes.Equal(after, base) {
			t.Fatalf("%s: tree not restored: %s", compat.Mode(), firstDiff(after, base))
		}
	})
}

// TestMachineRegistryKeepsMountedPlugins covers a server started the normal
// way, whose invocation already mounted the plugin: the same nodes come back.
func TestMachineRegistryKeepsMountedPlugins(t *testing.T) {
	prepareMachineRegistryTest(t)
	lifted := detachMounts(RootCmd)
	t.Cleanup(func() {
		detachMounts(RootCmd)
		RootCmd.AddCommand(lifted...)
	})
	dir := mountFixture(t)
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	specs, problems := mount.Discover(dir, canonTable.Verbs)
	mountInstalled(RootCmd, specs, problems)
	pre, _, _ := RootCmd.Find([]string{"demo"})
	if pre == nil || pre.Name() != "demo" {
		t.Fatal("fixture plugin was not mounted")
	}
	resetMachineRegistry()
	reg, err := machineRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Lookup("demo sub"); !ok {
		t.Error("the machine registry lost the plugin")
	}
	if post, _, _ := RootCmd.Find([]string{"demo"}); post != pre {
		t.Error("the already-mounted node must be put back unchanged")
	}
}

func TestMachineRegistryEnvRestored(t *testing.T) {
	prepareMachineRegistryTest(t)
	t.Setenv(compat.EnvVar, "")
	os.Unsetenv(compat.EnvVar)
	if _, had := os.LookupEnv(compat.EnvVar); had {
		t.Fatal("setup")
	}
	if _, err := machineRegistry(); err != nil {
		t.Fatal(err)
	}
	if _, had := os.LookupEnv(compat.EnvVar); had {
		t.Error("an unset NSELF_V15 must stay unset")
	}
}

// TestMCPExecSelfDelegates proves the legacy handlers and the invoker resolve
// the child binary by one rule.
func TestMCPExecSelfDelegates(t *testing.T) {
	t.Setenv(mcpSelfExecOverrideEnv, "/opt/fake-nself")
	want, _ := invoke.SelfExecutable()
	got, _ := selfExecutablePath()
	if got != want || got != "/opt/fake-nself" || mcpSelfExecOverrideEnv != invoke.SelfExecOverrideEnv {
		t.Errorf("selfExecutablePath %q, invoke.SelfExecutable %q", got, want)
	}
}

// TestInvokeArgvOnPluginParent runs invoker argv through the real cobra lookup
// of a mounted plugin parent (demo, children sub and nested). Plugin nodes
// parse no flags, so a bare trailing bool flag would eat the "--" and promote
// the first positional to a subcommand: demo --json --verbose -- sub resolves
// to demo sub, which the invoker never classified (post-merge review of
// P7-SURF-24, must-fix 1). BuildArgv renders --verbose=true, which resolves
// to demo.
func TestInvokeArgvOnPluginParent(t *testing.T) {
	r := pinRoot(t)
	demo := &cmdregistry.Command{Path: "nself demo", Args: []cmdregistry.Arg{{Name: "word", Required: true}},
		Flags: []cmdregistry.Flag{{Name: "verbose", Type: "bool", Default: "false"}}}
	req, err := invoke.DecodeRequest([]byte(`{"args":["sub"],"flags":{"verbose":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	argv, err := invoke.BuildArgv(demo, req)
	if err != nil {
		t.Fatal(err)
	}
	c, _, _ := r.Find(argv)
	if c == nil || c.CommandPath() != "nself demo" {
		t.Fatalf("argv %q resolved to %v, want nself demo", argv, c.CommandPath())
	}
	// The hazard the rendering avoids, pinned so the test proves something.
	bare := []string{"demo", "--json", "--verbose", "--", "sub"}
	if c, _, _ := r.Find(bare); c == nil || c.CommandPath() != "nself demo sub" {
		t.Fatalf("bare form resolved to %v; the fixture no longer shows the lookup shift", c.CommandPath())
	}
}
