//go:build e2eagent

package agent

// Purpose: the agent sandbox (SHIP Epic D7): a temp HOME holding only the sim
// hosts' ssh config and key, a fixture project with the inventory v2 file, and
// the argv that starts `nself mcp` through scripts/e2e/agent/mcp-sandbox.sh.
// Inputs: a temp root, the sim fleet's generated key and host addresses.
// Outputs: a Sandbox with Home, Project and the server command.
// Constraints: no credential is ever written into the repo: the only key is
// the one simharness generates per fleet at run time, copied into the temp
// HOME. Hosts are 127.0.0.1 only (published sim ports); mcp-sandbox.sh
// refuses anything else, an SSH agent socket, and a HOME that reaches the
// operator's ~/.ssh.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nself-org/cli/internal/controlplane"
)

// SimKeyEnv names the variable whose value is the sim key path; the inventory
// refers to it through ssh_key_ref (secrets are never stored inline).
const SimKeyEnv = "NSELF_E2E_SIM_KEY"

// LoopbackHost is the only host the sandbox may name.
const LoopbackHost = "127.0.0.1"

// SimHost is one sim host as the inventory sees it.
type SimHost struct {
	Env, Server, User string
	Port              int
	Tier              controlplane.Tier
	RemotePath        string
}

// Host returns the inventory host spec user@127.0.0.1:port.
func (h SimHost) Host() string { return fmt.Sprintf("%s@%s:%d", h.User, LoopbackHost, h.Port) }

// Sandbox is the agent's whole world: HOME and the project directory.
type Sandbox struct {
	Root, Home, Project string
	// KeyPath is the copy of the fleet key inside Home (empty until InstallKey).
	KeyPath string
}

// NewSandbox makes root/home (with an empty .ssh) and root/project.
func NewSandbox(root string) (*Sandbox, error) {
	s := &Sandbox{Root: root, Home: filepath.Join(root, "home"), Project: filepath.Join(root, "project")}
	for _, d := range []string{filepath.Join(s.Home, ".ssh"), s.Project} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// InstallKey copies the fleet's generated private key into HOME/.ssh (0600).
func (s *Sandbox) InstallKey(fleetKey string) error {
	b, err := os.ReadFile(fleetKey)
	if err != nil {
		return err
	}
	s.KeyPath = filepath.Join(s.Home, ".ssh", "id_sim")
	return os.WriteFile(s.KeyPath, b, 0o600)
}

// WriteSSHConfig points every loopback connection at the sim key and the
// sandbox known_hosts file, and nothing else.
func (s *Sandbox) WriteSSHConfig() error {
	if s.KeyPath == "" {
		return fmt.Errorf("sandbox: InstallKey first")
	}
	known := filepath.Join(s.Home, ".ssh", "known_hosts")
	cfg := strings.Join([]string{
		"Host " + LoopbackHost,
		"  IdentityFile " + s.KeyPath,
		"  IdentitiesOnly yes",
		"  IdentityAgent none",
		"  UserKnownHostsFile " + known,
		"",
	}, "\n")
	return os.WriteFile(filepath.Join(s.Home, ".ssh", "config"), []byte(cfg), 0o600)
}

// WriteInventory writes the inventory v2 file: local, plus one remote env per
// sim host (qa tier local-servers, prod tier prod).
func (s *Sandbox) WriteInventory(hosts ...SimHost) error {
	inv := &controlplane.Inventory{SchemaVersion: 2, Environments: map[string]controlplane.Environment{
		"local": {Name: "local", Kind: "local", Tier: controlplane.TierLocal},
	}}
	for _, h := range hosts {
		inv.Environments[h.Env] = controlplane.Environment{Name: h.Env, Kind: "remote", Tier: h.Tier,
			Servers: []controlplane.Server{{Name: h.Server, Role: controlplane.RoleApp, Host: h.Host(),
				RemotePath: h.RemotePath, Primary: true, SSHKeyRef: SimKeyEnv}}}
	}
	return controlplane.Write(s.Project, inv)
}

// InventoryPath is where controlplane.Write puts the inventory.
func (s *Sandbox) InventoryPath() string {
	return filepath.Join(s.Project, ".nself", "control-plane.yaml")
}

// SandboxScript returns the absolute path of scripts/e2e/agent/mcp-sandbox.sh.
func SandboxScript() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "scripts", "e2e", "agent", "mcp-sandbox.sh")
}

// ServerCommand returns the program and argv that start bin's `nself mcp` in
// the sandbox. operatorHome is the real HOME the script must keep out of
// reach; extraEnv entries are K=V and pass the script's own allowlist.
func (s *Sandbox) ServerCommand(bin, operatorHome string, extraEnv ...string) (string, []string) {
	args := []string{SandboxScript(), "--home", s.Home, "--project", s.Project,
		"--operator-home", operatorHome}
	if _, err := os.Stat(s.InventoryPath()); err == nil {
		args = append(args, "--inventory", s.InventoryPath())
	}
	if s.KeyPath != "" {
		args = append(args, "--env", SimKeyEnv+"="+s.KeyPath)
	}
	if dh := os.Getenv("DOCKER_HOST"); strings.HasPrefix(dh, "unix://") {
		args = append(args, "--env", "DOCKER_HOST="+dh)
	}
	for _, e := range extraEnv {
		args = append(args, "--env", e)
	}
	args = append(args, "--", bin, "mcp")
	return "bash", args
}
