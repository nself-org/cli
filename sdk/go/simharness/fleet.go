package simharness

// Purpose: Fleet and Start: the parallel-safe lifecycle of a set of sshd containers on one private network.
// Inputs: a TB (a *testing.T) and a Config of NodeSpecs.
// Outputs: a *Fleet whose nodes accept the fleet key; every container and the network are removed by Close.
// Constraints: INTEGRATION=1 is required (skip otherwise, fail when Docker is unreachable); names are unique per Start; only this fleet's own objects are ever removed.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Labels set on every network and container simharness creates.
const (
	LabelKey   = "org.nself.simharness"
	LabelFleet = "org.nself.simharness.fleet"
)

// EnvIntegration must be "1" for Start to run.
const EnvIntegration = "INTEGRATION"

// TB is the part of testing.TB the harness uses. *testing.T satisfies it; a
// fake can stand in for it in tests of the harness itself.
type TB interface {
	Helper()
	Logf(format string, args ...any)
	Fatalf(format string, args ...any)
	Skip(args ...any)
	Cleanup(func())
	TempDir() string
}

// NodeSpec describes one node.
type NodeSpec struct {
	// Name is the logical name (letters, digits, '-', '_', '.'); unique in the fleet.
	Name string
	// Image is a table key (openssh, debian, fedora, alpine) or a digest-pinned
	// reference. Default: openssh.
	Image string
	// Platform is passed to docker as --platform (for example linux/arm64).
	Platform string
	// CapAdd lists capabilities for this node only. Netem needs NET_ADMIN.
	CapAdd []string
	// Env adds container environment variables.
	Env map[string]string
	// Port and User override the image's sshd port and login user.
	Port int
	User string
}

// Config is the input of Start.
type Config struct {
	Nodes []NodeSpec
}

// Node is one running container.
type Node struct {
	// Name is the logical name from the NodeSpec.
	Name string
	// Container is the unique Docker container name.
	Container string
	// ID is the Docker container id.
	ID string
	// Host and Port are where the test host reaches sshd (127.0.0.1, published port).
	Host string
	Port int
	// User is the login user that accepts the fleet key.
	User string

	spec NodeSpec
	img  image
}

// Addr returns "host:port".
func (n *Node) Addr() string { return fmt.Sprintf("%s:%d", n.Host, n.Port) }

// Fleet is a running set of nodes.
type Fleet struct {
	// ID is the random fleet id (8 hex characters).
	ID string
	// Network is the Docker network name (nself-sim-<id>).
	Network string
	// Nodes are in Config order.
	Nodes []*Node

	t       TB
	keyPath string
	mu      sync.Mutex
	closed  bool
}

// KeyPath returns the OpenSSH private key every node accepts.
func (f *Fleet) KeyPath() string { return f.keyPath }

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,40}$`)

func validate(cfg Config) error {
	if len(cfg.Nodes) == 0 {
		return fmt.Errorf("Config.Nodes is empty")
	}
	seen := map[string]bool{}
	for _, s := range cfg.Nodes {
		if !nameRe.MatchString(s.Name) {
			return fmt.Errorf("node name %q is invalid", s.Name)
		}
		if seen[s.Name] {
			return fmt.Errorf("node name %q is used twice", s.Name)
		}
		seen[s.Name] = true
		if _, err := lookup(s); err != nil {
			return err
		}
	}
	return nil
}

func newID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// Start creates the network and containers for cfg and waits until every sshd
// answers. It skips the test unless INTEGRATION=1, fails it when Docker is
// unreachable, and registers Close with t.Cleanup before it creates anything.
func Start(t TB, cfg Config) *Fleet {
	t.Helper()
	if os.Getenv(EnvIntegration) != "1" {
		t.Skip("set INTEGRATION=1 to run the Docker sshd harness")
		return nil
	}
	ctx := context.Background()
	// Some docker clients exit 0 with empty output when the daemon is down, so
	// an empty server version counts as unreachable too.
	if v, err := docker(ctx, "info", "--format", "{{.ServerVersion}}"); err != nil || strings.TrimSpace(v) == "" {
		t.Fatalf("simharness: INTEGRATION=1 but Docker is unreachable: %v", err)
	}
	if err := validate(cfg); err != nil {
		t.Fatalf("simharness: %v", err)
	}

	f := &Fleet{ID: newID(), t: t}
	f.Network = "nself-sim-" + f.ID
	register(f)
	t.Cleanup(f.Close)

	pub, keyPath, err := newKeyPair(t.TempDir())
	if err != nil {
		t.Fatalf("simharness: key pair: %v", err)
	}
	f.keyPath = keyPath

	if _, err := docker(ctx, "network", "create", "--driver", "bridge",
		"--label", LabelKey+"=1", "--label", LabelFleet+"="+f.ID, f.Network); err != nil {
		t.Fatalf("simharness: %v", err)
	}
	for _, spec := range cfg.Nodes {
		n, err := f.startNode(ctx, spec, pub)
		if err != nil {
			t.Fatalf("simharness: node %s: %v", spec.Name, err)
		}
		f.mu.Lock()
		f.Nodes = append(f.Nodes, n)
		f.mu.Unlock()
		if err := n.waitSSH(90 * time.Second); err != nil {
			t.Fatalf("simharness: node %s: %v", spec.Name, err)
		}
	}
	return f
}

// Close removes this fleet's containers and network. It is idempotent, runs
// from t.Cleanup and from the signal handler, and touches nothing it did not
// create: it removes containers by this fleet's label and the network by name.
func (f *Fleet) Close() { f.closeWith(f.logf) }

func (f *Fleet) logf(format string, args ...any) {
	if f.t != nil {
		f.t.Logf(format, args...)
	}
}

func (f *Fleet) closeWith(logf func(string, ...any)) {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	f.closed = true
	f.mu.Unlock()
	defer unregister(f)

	ctx := context.Background()
	ids, err := docker(ctx, "ps", "-aq", "--no-trunc", "--filter", "label="+LabelFleet+"="+f.ID)
	if err != nil {
		logf("simharness: list containers of fleet %s: %v", f.ID, err)
	}
	args := []string{"rm", "-f", "-v"}
	list := strings.Fields(ids)
	sort.Strings(list)
	if len(list) > 0 {
		if _, err := docker(ctx, append(args, list...)...); err != nil {
			logf("simharness: %v", err)
		}
	}
	if _, err := docker(ctx, "network", "rm", f.Network); err != nil {
		logf("simharness: %v", err)
	}
}
