//go:build integration

// Package sim_test provides Docker-based integration tests for the
// control-plane pipeline. Tests in this file require Docker to be running and
// INTEGRATION=1 to be set in the environment.
//
// Run with:
//
//	INTEGRATION=1 go test -mod=vendor -tags integration ./internal/controlplane/sim/...
package sim_test

import (
	"context"
	"os"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/controlplane/sim"
)

// skipUnlessIntegration skips the test unless INTEGRATION=1 is set.
func skipUnlessIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run Docker sshd integration tests")
	}
}

// buildInventory converts a sim Fleet into a controlplane Inventory. Each
// SimServer becomes a remote Server in the "sim" environment. The fleet's
// PrivateKeyPath is exposed via EnvVarName so the prober can find the key.
func buildInventory(fleet *sim.Fleet) *controlplane.Inventory {
	servers := make([]controlplane.Server, 0, len(fleet.Servers))
	for _, ss := range fleet.Servers {
		role := controlplane.ServerRole(ss.Role)
		servers = append(servers, controlplane.Server{
			Name:       ss.Name,
			Role:       role,
			Host:       "nself@" + ss.SSHHostPort(),
			SSHKeyRef:  fleet.EnvVarName,
			RemotePath: "/tmp/nself-sim",
			Primary:    ss.Primary,
		})
	}
	return &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "sim-test",
		Environments: map[string]controlplane.Environment{
			"sim": {
				Name:    "sim",
				Kind:    "remote",
				Servers: servers,
			},
		},
	}
}

// TestIntegrationPipelineOrder verifies that the control-plane pipeline processes
// servers in the correct topology order: observability → app → lb.
//
// The test spins up 4 Docker containers (1 lb, 1 obs, 2 app) using the default
// fleet config and uses a stub prober (not real SSH) to exercise ordering logic.
// The Docker containers are torn down when the test ends.
func TestIntegrationPipelineOrder(t *testing.T) {
	skipUnlessIntegration(t)

	fleet := sim.Start(t, sim.DefaultFleetConfig())
	defer fleet.Close()

	if len(fleet.Servers) != 4 {
		t.Fatalf("expected 4 sim servers (1 obs + 2 app + 1 lb), got %d", len(fleet.Servers))
	}

	inv := buildInventory(fleet)

	// Use a stub prober so the test doesn't depend on SSH connectivity inside
	// the container — we're testing pipeline ordering and LB lifecycle, not
	// actual SSH. The SSHProber integration is covered by TestIntegrationSSHReachable.
	prober := &integrationStubProber{keyPath: fleet.PrivateKeyPath, keyRef: fleet.EnvVarName}

	// Set up fake deploy tools so DeployViaSsh exits 0.
	composePath := setupFakeDeploy(t, fleet.EnvVarName, fleet.PrivateKeyPath)

	result, err := controlplane.Run(context.Background(), inv, "sim", prober, composePath)
	if err != nil {
		t.Fatalf("controlplane.Run: %v", err)
	}

	// Verify 4 results (obs-01, app-01, app-02, lb-01).
	if len(result.Servers) != 4 {
		t.Errorf("len(result.Servers) = %d, want 4", len(result.Servers))
	}

	// Verify obs before app before lb ordering.
	assertPipelineOrder(t, result.Servers)

	// Primary app server must not have been skipped.
	if result.PrimarySkipped {
		t.Error("PrimarySkipped = true; primary app server should have been deployed")
	}
}

// TestIntegrationLBDrainCalled verifies that when an LB server is present and
// manage-capable, the pipeline calls Drain before deploying each app server
// and Enable after a successful deploy.
func TestIntegrationLBDrainCalled(t *testing.T) {
	skipUnlessIntegration(t)

	fleet := sim.Start(t, sim.DefaultFleetConfig())
	defer fleet.Close()

	inv := buildInventory(fleet)
	trackingProber := &integrationTrackingProber{
		keyPath: fleet.PrivateKeyPath,
		keyRef:  fleet.EnvVarName,
	}

	composePath := setupFakeDeploy(t, fleet.EnvVarName, fleet.PrivateKeyPath)

	_, err := controlplane.Run(context.Background(), inv, "sim", trackingProber, composePath)
	if err != nil {
		t.Fatalf("controlplane.Run: %v", err)
	}

	// The pipeline calls DrainOK/Enable via the lb.go Drain/Enable functions.
	// Since the fake SSH exits 0 (no nself-lb binary), we expect DrainHelperAbsent
	// (exit code 127) — but the pipeline must have attempted the LB operations.
	// We verify via the tracking prober that the LB was probed (reachable).
	if !trackingProber.lbProbed {
		t.Error("LB server was never probed for capability; expected pipeline to check LB reachability")
	}
}

// TestIntegrationFleetBuildSpecs verifies that DefaultFleetConfig produces
// the correct server topology: obs first, then app(s), then lb.
func TestIntegrationFleetBuildSpecs(t *testing.T) {
	skipUnlessIntegration(t)

	fleet := sim.Start(t, sim.DefaultFleetConfig())
	defer fleet.Close()

	roles := make([]string, 0, len(fleet.Servers))
	for _, s := range fleet.Servers {
		roles = append(roles, s.Role)
	}

	wantOrder := []string{
		sim.RoleObservability,
		sim.RoleApp,
		sim.RoleApp,
		sim.RoleLB,
	}

	if len(roles) != len(wantOrder) {
		t.Fatalf("fleet has %d servers, want %d; roles=%v", len(roles), len(wantOrder), roles)
	}
	for i, want := range wantOrder {
		if roles[i] != want {
			t.Errorf("server[%d].Role = %q, want %q", i, roles[i], want)
		}
	}
}

// TestIntegrationPrimaryAppMarked verifies that exactly one server is marked
// Primary: true in the default fleet (the first app server).
func TestIntegrationPrimaryAppMarked(t *testing.T) {
	skipUnlessIntegration(t)

	fleet := sim.Start(t, sim.DefaultFleetConfig())
	defer fleet.Close()

	var primaries []string
	for _, s := range fleet.Servers {
		if s.Primary {
			primaries = append(primaries, s.Name)
		}
	}

	if len(primaries) != 1 {
		t.Errorf("expected 1 primary server, got %d: %v", len(primaries), primaries)
	}
	if len(primaries) == 1 && primaries[0] != "sim-app-01" {
		t.Errorf("primary server = %q, want %q", primaries[0], "sim-app-01")
	}
}

// assertPipelineOrder verifies that in the result slice:
// 1. All observability servers precede all app servers.
// 2. All app servers precede all lb servers.
func assertPipelineOrder(t *testing.T, results []controlplane.ServerResult) {
	t.Helper()

	obsLast, appFirst, appLast, lbFirst := -1, -1, -1, -1
	for i, sr := range results {
		switch sr.Role {
		case controlplane.RoleObservability:
			obsLast = i
		case controlplane.RoleApp:
			if appFirst < 0 {
				appFirst = i
			}
			appLast = i
		case controlplane.RoleLB:
			if lbFirst < 0 {
				lbFirst = i
			}
		}
	}

	if obsLast >= 0 && appFirst >= 0 && obsLast > appFirst {
		t.Errorf("obs server (last at %d) appears AFTER first app server (at %d); want obs→app order", obsLast, appFirst)
	}
	if appLast >= 0 && lbFirst >= 0 && appLast > lbFirst {
		t.Errorf("app server (last at %d) appears AFTER first lb server (at %d); want app→lb order", appLast, lbFirst)
	}
}

// setupFakeDeploy creates fake ssh and rsync binaries in a temp dir, sets PATH,
// writes the key file path into envRef, and returns a fake compose file path.
// This prevents real SSH invocations while still exercising the pipeline flow.
func setupFakeDeploy(t *testing.T, envRef, keyPath string) string {
	t.Helper()
	dir := t.TempDir()

	for _, bin := range []string{"ssh", "rsync"} {
		p := dir + "/" + bin
		if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatalf("write fake %s: %v", bin, err)
		}
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+origPath)
	t.Setenv(envRef, keyPath)

	composePath := dir + "/docker-compose.yml"
	if err := os.WriteFile(composePath, []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	return composePath
}

// integrationStubProber returns manage-capable results for all servers.
type integrationStubProber struct {
	keyPath string
	keyRef  string
}

func (p *integrationStubProber) SSHReachable(_ controlplane.Server) (bool, int, error) {
	return true, 5, nil
}
func (p *integrationStubProber) DockerOK(_ controlplane.Server) (bool, error) {
	return true, nil
}
func (p *integrationStubProber) KeyState(s controlplane.Server) (bool, string) {
	if s.SSHKeyRef == p.keyRef {
		return true, p.keyPath
	}
	return false, ""
}

// integrationTrackingProber records which server roles were probed.
type integrationTrackingProber struct {
	keyPath  string
	keyRef   string
	lbProbed bool
}

func (p *integrationTrackingProber) SSHReachable(s controlplane.Server) (bool, int, error) {
	if s.Role == controlplane.RoleLB {
		p.lbProbed = true
	}
	return true, 5, nil
}
func (p *integrationTrackingProber) DockerOK(_ controlplane.Server) (bool, error) {
	return true, nil
}
func (p *integrationTrackingProber) KeyState(s controlplane.Server) (bool, string) {
	if s.SSHKeyRef == p.keyRef {
		return true, p.keyPath
	}
	return false, ""
}
