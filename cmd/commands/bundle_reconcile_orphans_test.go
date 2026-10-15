package commands

// Purpose: test that an extension reconcile swaps containers in one apply (D-0432).
// Inputs: a temporary project, a local plugin registry and a fake container runtime;
// outputs: the orphan removals and service starts the reconcile asks for.
// Constraints: no docker daemon, no external registry.

import (
	"context"
	"reflect"
	"strings"
	"testing"

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
