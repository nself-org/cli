package sim

// Purpose: container-spec building and small accessors backing the Fleet simulation harness in harness.go.
// Inputs: FleetConfig (for buildSpecs) and *testing.T for TempFile.
// Outputs: ContainerSpec lists, temp paths and the SSH address of a SimServer.
// Constraints: the Docker work (images, names, keys, waits, cleanup) lives in cli:sdk/go/simharness; this file runs no process.

import (
	"fmt"
	"path/filepath"
	"testing"
)

// buildSpecs returns ContainerSpec list: observability first, then app(s), then lb.
// This ordering matches the topology-aware pipeline expectation (obs→app→lb).
func buildSpecs(cfg FleetConfig) []ContainerSpec {
	var specs []ContainerSpec
	if cfg.HasObservability {
		specs = append(specs, ContainerSpec{Name: "sim-obs-01", Role: RoleObservability})
	}
	for i := 0; i < cfg.AppCount; i++ {
		primary := i == 0
		specs = append(specs, ContainerSpec{
			Name:    fmt.Sprintf("sim-app-%02d", i+1),
			Role:    RoleApp,
			Primary: primary,
		})
	}
	if cfg.HasLB {
		specs = append(specs, ContainerSpec{Name: "sim-lb-01", Role: RoleLB})
	}
	return specs
}

// TempFile returns a path inside t.TempDir() for storing auxiliary files.
func TempFile(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name)
}

// SSHHostPort returns the SSH connection address for a SimServer as "host:port".
func (s SimServer) SSHHostPort() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}
