package simharness

// Purpose: per-node operations: container creation, readiness, Exec, CopyTo, Restart, Banner.
// Inputs: node names (the logical NodeSpec.Name) and argv slices.
// Outputs: command stdout, copied files, restarted nodes.
// Constraints: argv only, no host shell; a failed step fails the test with the docker error.

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Node returns the node named name, or fails the test.
func (f *Fleet) Node(t TB, name string) *Node {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.Nodes {
		if n.Name == name {
			return n
		}
	}
	t.Fatalf("simharness: no node %q in fleet %s", name, f.ID)
	return nil
}

func (f *Fleet) startNode(ctx context.Context, spec NodeSpec, pubKey string) (*Node, error) {
	img, err := lookup(spec)
	if err != nil {
		return nil, err
	}
	ref, err := img.reference(ctx, spec.Platform)
	if err != nil {
		return nil, err
	}
	n := &Node{Name: spec.Name, Container: "nself-sim-" + f.ID + "-" + spec.Name,
		Host: "127.0.0.1", User: img.user, spec: spec, img: img}
	args := []string{"run", "-d", "--name", n.Container, "--network", f.Network,
		"--network-alias", spec.Name, "--label", LabelKey + "=1", "--label", LabelFleet + "=" + f.ID,
		"-p", "127.0.0.1::" + strconv.Itoa(img.port)}
	if spec.Platform != "" {
		args = append(args, "--platform", spec.Platform)
	}
	for _, c := range spec.CapAdd {
		args = append(args, "--cap-add", c)
	}
	env := map[string]string{"PUBLIC_KEY": strings.TrimSpace(pubKey)}
	for k, v := range img.env {
		env[k] = v
	}
	for k, v := range spec.Env {
		env[k] = v
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-e", k+"="+env[k])
	}
	id, err := docker(ctx, append(args, ref)...)
	if err != nil {
		return nil, err
	}
	n.ID = id
	return n, n.resolvePort(ctx)
}

// resolvePort reads the published host port from Docker.
func (n *Node) resolvePort(ctx context.Context) error {
	out, err := docker(ctx, "port", n.Container, strconv.Itoa(n.img.port)+"/tcp")
	if err != nil {
		return err
	}
	line := strings.Fields(out)
	if len(line) == 0 {
		return fmt.Errorf("container %s publishes no port %d", n.Container, n.img.port)
	}
	i := strings.LastIndex(line[0], ":")
	p, err := strconv.Atoi(line[0][i+1:])
	if i < 0 || err != nil {
		return fmt.Errorf("cannot parse published port %q", out)
	}
	n.Port = p
	return nil
}

// Banner dials the node and returns its SSH identification line. Unlike a
// bare TCP connect it needs sshd itself to answer, so it fails for a paused,
// partitioned or still-starting node even when a port forwarder accepts.
func (n *Node) Banner(timeout time.Duration) (string, error) {
	c, err := net.DialTimeout("tcp", n.Addr(), timeout)
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	_ = c.SetReadDeadline(time.Now().Add(timeout))
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(line, "SSH-") {
		return "", fmt.Errorf("not an ssh banner: %q", strings.TrimSpace(line))
	}
	return strings.TrimSpace(line), nil
}

func (n *Node) waitSSH(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		if _, last = n.Banner(2 * time.Second); last == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("sshd on %s not ready after %s: %v", n.Addr(), timeout, last)
}

// Exec runs argv in the node's container and returns its stdout. A non-zero
// exit fails the test with the command's stderr.
func (f *Fleet) Exec(t TB, node string, argv ...string) string {
	t.Helper()
	n := f.Node(t, node)
	out, err := docker(context.Background(), append([]string{"exec", n.Container}, argv...)...)
	if err != nil {
		t.Fatalf("simharness: %v", err)
	}
	return out
}

// CopyTo copies the host file src to dst inside the node and makes it
// world-readable, so the login user can read it. The dst directory must exist.
func (f *Fleet) CopyTo(t TB, node, src, dst string) {
	t.Helper()
	n := f.Node(t, node)
	ctx := context.Background()
	if _, err := docker(ctx, "cp", src, n.Container+":"+dst); err != nil {
		t.Fatalf("simharness: %v", err)
	}
	if _, err := docker(ctx, "exec", "-u", "0", n.Container, "chmod", "a+r", dst); err != nil {
		t.Fatalf("simharness: %v", err)
	}
}

// Restart restarts the node's container and waits for sshd. The published
// port may change; read it again from Node.Port.
func (f *Fleet) Restart(t TB, node string) {
	t.Helper()
	n := f.Node(t, node)
	ctx := context.Background()
	if _, err := docker(ctx, "restart", "--time", "2", n.Container); err != nil {
		t.Fatalf("simharness: %v", err)
	}
	f.settle(t, n)
}

// settle re-reads the published port and waits for sshd.
func (f *Fleet) settle(t TB, n *Node) {
	t.Helper()
	f.mu.Lock()
	err := n.resolvePort(context.Background())
	f.mu.Unlock()
	if err == nil {
		err = n.waitSSH(90 * time.Second)
	}
	if err != nil {
		t.Fatalf("simharness: node %s: %v", n.Name, err)
	}
}
