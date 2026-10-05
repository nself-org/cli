// Package sim provides a multi-server Docker sshd simulation harness for
// integration-testing the control-plane pipeline without touching live hosts.
//
// The harness spins up N Docker containers (each running sshd) on an isolated
// bridge network, injects an ephemeral ED25519 key-pair, and exposes a Fleet
// value whose servers can be passed directly to controlplane.Resolve /
// controlplane.Run.
//
// The container work is done by cli:sdk/go/simharness (digest-pinned images,
// unique container and network names per Start, cleanup also on t.Fatal and
// SIGINT). This package keeps the control-plane view of it: roles, the
// SimServer shape and the key environment variable. SimServer.Name stays the
// logical name; the Docker container name is unique per Start.
//
// Close tears all containers and the network down, even after a t.Fatal.
//
// Build tag: integration — tests in this package are skipped unless
// INTEGRATION=1 is set in the environment; with INTEGRATION=1 and no Docker
// daemon Start fails the test.
package sim

import (
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/simharness"
)

// Role constants mirror controlplane.ServerRole but kept local to avoid
// importing the parent package (the harness must not import impl under test).
const (
	RoleApp           = "app"
	RoleLB            = "lb"
	RoleObservability = "observability"
)

// ContainerSpec describes one simulated host in the fleet.
type ContainerSpec struct {
	Name string
	Role string
	// Primary marks the server as the primary app server (non-zero exit on skip).
	Primary bool
}

// SimServer is one running container with its connection details.
type SimServer struct {
	ContainerSpec
	// Host is the container's address reachable from the test host.
	Host string
	// Port is the sshd port mapped to a random high port on the host.
	Port int
	// ContainerID is the Docker container ID for cleanup.
	ContainerID string
}

// Fleet is the full simulation environment returned by Start.
type Fleet struct {
	// Servers is the ordered list of simulated hosts.
	Servers []SimServer
	// PrivateKeyPath is the path to the ED25519 private key that all
	// containers accept. Set this env var value as the SSH key for probers.
	PrivateKeyPath string
	// EnvVarName is the environment variable that must be set to PrivateKeyPath
	// so the control-plane prober can find the key.
	EnvVarName string
	// NetworkName is the Docker bridge network created for this fleet.
	NetworkName string

	harness *simharness.Fleet
}

// Close tears down all containers and the bridge network.
func (f *Fleet) Close() {
	if f.harness != nil {
		f.harness.Close()
	}
}

// FleetConfig controls how many servers of each role are started.
type FleetConfig struct {
	// AppCount is the number of app-role containers (≥1).
	AppCount int
	// HasLB controls whether a load-balancer container is included.
	HasLB bool
	// HasObservability controls whether an observability container is added.
	HasObservability bool
	// EnvVarName is the SSH key env var name injected into the fleet.
	// Defaults to "NSELF_SSH_KEY_SIM".
	EnvVarName string
}

// DefaultFleetConfig returns the 4-server configuration used by CP-T19:
// 1 lb + 1 observability + 2 app.
func DefaultFleetConfig() FleetConfig {
	return FleetConfig{
		AppCount:         2,
		HasLB:            true,
		HasObservability: true,
		EnvVarName:       "NSELF_SSH_KEY_SIM",
	}
}

// Start launches the simulation fleet. It is called from integration tests
// gated by INTEGRATION=1. The caller must call fleet.Close() or defer it;
// Start also registers Close with t.Cleanup.
//
// Start generates an ephemeral ED25519 key-pair, writes the private key to a
// temp file (mode 0600), and configures each sshd container to accept that key
// as the only authorised identity.
func Start(t *testing.T, cfg FleetConfig) *Fleet {
	t.Helper()

	if cfg.EnvVarName == "" {
		cfg.EnvVarName = "NSELF_SSH_KEY_SIM"
	}

	specs := buildSpecs(cfg)
	nodes := make([]simharness.NodeSpec, 0, len(specs))
	for _, spec := range specs {
		nodes = append(nodes, simharness.NodeSpec{Name: spec.Name})
	}
	h := simharness.Start(t, simharness.Config{Nodes: nodes})

	f := &Fleet{
		EnvVarName:     cfg.EnvVarName,
		PrivateKeyPath: h.KeyPath(),
		NetworkName:    h.Network,
		harness:        h,
	}
	for _, spec := range specs {
		n := h.Node(t, spec.Name)
		f.Servers = append(f.Servers, SimServer{
			ContainerSpec: spec,
			Host:          n.Host,
			Port:          n.Port,
			ContainerID:   n.ID,
		})
	}

	// Set the key env var so probe-based tests can pick it up.
	t.Setenv(cfg.EnvVarName, f.PrivateKeyPath)

	return f
}
