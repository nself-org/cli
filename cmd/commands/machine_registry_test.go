package commands

// Purpose: prove machineRegistry builds the v1.5 registry in both compat modes,
// mounts installed plugins, and puts the process tree back.
// Constraints: the real RootCmd is shared with tests that redeclare flags on
// real commands, so every case runs in a fresh copy of this test binary (the
// registry golden pattern); the parent only checks the child's PASS lines.

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/plugin/mount"
)

const machineChild = "NSELF_MACHINE_REGISTRY_CHILD"

// inFreshProcess re-runs the named test in a child test binary and reports
// whether this is that child. The parent fails unless the child printed the
// sentinel after its checks.
func inFreshProcess(t *testing.T, name string) (child bool) {
	t.Helper()
	if os.Getenv(machineChild) == name {
		return true
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+name+"$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), machineChild+"="+name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s in a fresh process failed: %v\n%s", name, err, out)
	}
	if !bytes.Contains(out, []byte("CHECKED-"+name)) || !bytes.Contains(out, []byte("--- PASS: "+name)) {
		t.Fatalf("the fresh process did not run %s; output:\n%s", name, out)
	}
	return false
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
	if !inFreshProcess(t, "TestMachineRegistry") {
		return
	}
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
	t.Log("CHECKED-TestMachineRegistry")
}

func TestMachineRegistryPluginMount(t *testing.T) {
	if !inFreshProcess(t, "TestMachineRegistryPluginMount") {
		return
	}
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
	t.Log("CHECKED-TestMachineRegistryPluginMount")
}

// TestMachineRegistryKeepsMountedPlugins covers a server started the normal
// way, whose invocation already mounted the plugin: the same nodes come back.
func TestMachineRegistryKeepsMountedPlugins(t *testing.T) {
	if !inFreshProcess(t, "TestMachineRegistryKeepsMountedPlugins") {
		return
	}
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
	t.Log("CHECKED-TestMachineRegistryKeepsMountedPlugins")
}

func TestMachineRegistryEnvRestored(t *testing.T) {
	t.Setenv(compat.EnvVar, "")
	os.Unsetenv(compat.EnvVar)
	if _, had := os.LookupEnv(compat.EnvVar); had {
		t.Fatal("setup")
	}
	if inFreshProcess(t, "TestMachineRegistryEnvRestored") {
		resetMachineRegistry()
		if _, err := machineRegistry(); err != nil {
			t.Fatal(err)
		}
		if _, had := os.LookupEnv(compat.EnvVar); had {
			t.Error("an unset NSELF_V15 must stay unset")
		}
		t.Log("CHECKED-TestMachineRegistryEnvRestored")
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
