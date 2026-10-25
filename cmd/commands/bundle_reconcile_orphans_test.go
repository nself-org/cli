package commands

// Purpose: test that an extension reconcile swaps containers in one apply (D-0432).
// Inputs: a temporary project, a local plugin registry and a fake container runtime;
// outputs: the orphan removals and service starts the reconcile asks for.
// Constraints: no docker daemon, no external registry.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/reconcile"
)

// fakeSwapRuntime reports a stack that runs one container no planned service
// defines (the old proxy) and no container for the planned "postgres" service.
type fakeSwapRuntime struct{ running []reconcile.RunningContainer }

func (f fakeSwapRuntime) Running(context.Context, string) ([]reconcile.RunningContainer, error) {
	return f.running, nil
}

func (f fakeSwapRuntime) ConfigHashes(context.Context, []string, []string, string) (map[string]string, error) {
	return map[string]string{"postgres": "h1"}, nil
}

// swapSeams records the removals and starts and restores the seams after the test.
func swapSeams(t *testing.T, rt reconcile.ContainerRuntime) (removed *[]string, started *[][]string, order *[]string) {
	t.Helper()
	oldRT, oldRm, oldStart := extensionRuntime, extensionRemoveOrphans, extensionStartServices
	t.Cleanup(func() { extensionRuntime, extensionRemoveOrphans, extensionStartServices = oldRT, oldRm, oldStart })
	removed, started, order = &[]string{}, &[][]string{}, &[]string{}
	extensionRuntime = rt
	extensionRemoveOrphans = func(_ context.Context, o []docker.OrphanContainer) []error {
		for _, c := range o {
			*removed = append(*removed, c.Name)
		}
		*order = append(*order, "remove")
		return nil
	}
	extensionStartServices = func(_ context.Context, _ string, services []string) error {
		*started = append(*started, services)
		*order = append(*order, "start")
		return nil
	}
	return removed, started, order
}

func TestExtensionReconcileSwapsContainers(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		if !compat.V15() {
			return
		}
		extensionFixture(t, "")
		localPluginRegistry(t)
		removed, started, order := swapSeams(t, fakeSwapRuntime{running: []reconcile.RunningContainer{
			{Name: "proj_oldproxy", Service: "oldproxy", State: "running", ConfigHash: "x"},
		}})
		if err := runPluginInstall(pluginInstallCmd, []string{"zzfixture"}); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(*removed, []string{"proj_oldproxy"}) {
			t.Fatalf("orphan not removed in the same apply: %v", *removed)
		}
		if !reflect.DeepEqual(*started, [][]string{{"postgres"}}) {
			t.Fatalf("new service not started in the same apply: %v", *started)
		}
		if strings.Join(*order, ",") != "remove,start" {
			t.Fatalf("orphans must go before the start frees the ports: %v", *order)
		}
	})
}

func TestExtensionReconcileWithoutOrphansStartsNothing(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		if !compat.V15() {
			return
		}
		extensionFixture(t, "")
		localPluginRegistry(t)
		removed, started, _ := swapSeams(t, fakeSwapRuntime{running: []reconcile.RunningContainer{
			{Name: "proj_postgres", Service: "postgres", State: "running", ConfigHash: "h1"},
		}})
		if err := runPluginInstall(pluginInstallCmd, []string{"zzfixture"}); err != nil {
			t.Fatal(err)
		}
		if len(*removed) != 0 || len(*started) != 0 {
			t.Fatalf("no orphan: nothing may be removed or started, got %v %v", *removed, *started)
		}
	})
}

func TestExtensionReconcileProdRefusalKeepsContainers(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		if !compat.V15() {
			return
		}
		extensionFixture(t, "prod")
		localPluginRegistry(t)
		removed, started, _ := swapSeams(t, fakeSwapRuntime{running: []reconcile.RunningContainer{
			{Name: "proj_oldproxy", Service: "oldproxy", State: "running", ConfigHash: "x"},
		}})
		if err := runPluginInstall(pluginInstallCmd, []string{"zzfixture"}); err == nil {
			t.Fatal("prod-class without --yes must be refused")
		}
		if len(*removed) != 0 || len(*started) != 0 {
			t.Fatalf("a refused apply must not touch containers, got %v %v", *removed, *started)
		}
	})
}

// The swap start must see plugin services: they are defined in the fragments
// of .nself/compose-files.txt, so the compose call has to list them (D-0432).
func TestExtensionComposeUsesProjectManifest(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "docker-compose.yml")
	frag := filepath.Join(dir, ".nself", "plugins", "traefik", "docker-compose.plugin.yml")
	for _, p := range []string{base, frag} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("services: {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for rel, body := range map[string]string{
		".nself/compose-files.txt": base + "\n" + frag + "\n",
		".env":                     "A=1\n",
		".nself/compose.env":       "B=2\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c := extensionCompose(dir)
	if !reflect.DeepEqual(c.ComposeFiles, []string{base, frag}) {
		t.Fatalf("compose files must come from the manifest: %v", c.ComposeFiles)
	}
	wantEnv := []string{filepath.Join(dir, ".env"), filepath.Join(dir, ".nself", "compose.env")}
	if !reflect.DeepEqual(c.EnvFiles, wantEnv) {
		t.Fatalf("env files must follow the project: %v", c.EnvFiles)
	}
}

// A failed container swap happens after Apply wrote the generated files, so the
// error must say so (appready.ErrCommitted): the bundle install keeps its
// plugins then. A refused apply wrote nothing and must not carry the mark.
func TestExtensionReconcileSwapFailureIsCommitted(t *testing.T) {
	trigger := reconcile.Trigger{Kind: reconcile.TriggerBundle, Subject: "zzfixture"}
	running := fakeSwapRuntime{running: []reconcile.RunningContainer{
		{Name: "proj_oldproxy", Service: "oldproxy", State: "running", ConfigHash: "x"},
	}}
	cases := map[string]func(){
		"remove": func() {
			extensionRemoveOrphans = func(context.Context, []docker.OrphanContainer) []error {
				return []error{errors.New("docker rm failed")}
			}
		},
		"start": func() {
			extensionStartServices = func(context.Context, string, []string) error { return errors.New("compose up failed") }
		},
	}
	for name, fail := range cases {
		t.Run(name, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				if !compat.V15() {
					return
				}
				extensionFixture(t, "")
				localPluginRegistry(t)
				swapSeams(t, running)
				fail()
				err := reconcileAfterExtension(pluginInstallCmd, trigger)
				if err == nil || !errors.Is(err, appready.ErrCommitted) {
					t.Fatalf("err = %v, want a post-commit failure", err)
				}
			})
		})
	}
	t.Run("refused", func(t *testing.T) {
		compattest.Both(t, func(t *testing.T) {
			if !compat.V15() {
				return
			}
			extensionFixture(t, "prod")
			localPluginRegistry(t)
			swapSeams(t, running)
			err := reconcileAfterExtension(pluginInstallCmd, trigger)
			if err == nil || errors.Is(err, appready.ErrCommitted) {
				t.Fatalf("err = %v, want a pre-commit refusal", err)
			}
		})
	})
}

// Apply publishes the generated files and then fails to write its state record:
// the files stay, so the error must carry the commit mark and a bundle install
// keeps its plugins (the review finding on D-0760).
func TestExtensionReconcileRecordFailureIsCommitted(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	trigger := reconcile.Trigger{Kind: reconcile.TriggerBundle, Subject: "zzfixture"}
	compattest.Both(t, func(t *testing.T) {
		if !compat.V15() {
			return
		}
		p := extensionFixture(t, "")
		localPluginRegistry(t)
		swapSeams(t, fakeSwapRuntime{})
		stateDir := filepath.Dir(reconcile.GeneratedStatePath(p.project))
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(stateDir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(stateDir, 0o755) })
		err := reconcileAfterExtension(pluginInstallCmd, trigger)
		if err == nil || !errors.Is(err, appready.ErrCommitted) {
			t.Fatalf("err = %v, want a post-commit failure", err)
		}
		if _, statErr := os.Stat(filepath.Join(p.project, "docker-compose.yml")); statErr != nil {
			t.Fatalf("the published compose file is missing: %v", statErr)
		}
	})
}
