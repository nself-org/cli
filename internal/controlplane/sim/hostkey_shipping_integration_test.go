//go:build integration

package sim_test

// Tests for P7-DEPL-29 against a real sim ssh host: a v1.4 pipeline run that
// ships an env file fails E487 on an unpinned host key (no copy, no accept-new
// probe) and ships once the key is pinned.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/controlplane/sim"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// simHostFingerprint scans the sim host's ed25519 key the way the strict check does.
func simHostFingerprint(t *testing.T, host string) string {
	t.Helper()
	spec, err := remote.ParseHostSpec(host)
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ssh-keyscan", "-T", "5", "-t", "ed25519", "-p", strconv.Itoa(spec.Port), "--", spec.Host).Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if fp, err := remote.Fingerprint(line); err == nil {
			return fp
		}
	}
	t.Fatalf("no key in scan: %s", out)
	return ""
}

func TestDeploySimEnvShippingUnknownHostKeyV14(t *testing.T) {
	skipUnlessIntegration(t)
	fleet := sim.Start(t, sim.FleetConfig{AppCount: 1})
	defer fleet.Close()
	if out, err := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output(); err == nil {
		t.Setenv("DOCKER_HOST", strings.TrimSpace(string(out)))
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NSELF_V15", "0")
	compose, log := simDeployTools(t)
	envFile := filepath.Join(t.TempDir(), ".env.qa")
	if err := os.WriteFile(envFile, []byte("FAKE_VALUE=not-a-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIM_SSH_KEY", fleet.PrivateKeyPath)
	host := "nself@" + fleet.Servers[0].SSHHostPort()
	root := t.TempDir()
	inv := &controlplane.Inventory{SchemaVersion: 2, Environments: map[string]controlplane.Environment{
		"local": {Name: "local", Kind: "local", Tier: controlplane.TierLocal},
		"qa": {Name: "qa", Kind: "remote", Tier: controlplane.TierLocalServers, Servers: []controlplane.Server{
			{Name: "qa-app", Role: controlplane.RoleApp, Host: host, SSHKeyRef: "SIM_SSH_KEY", RemotePath: "/tmp/nself-sim", Primary: true}}},
	}}
	if err := controlplane.Write(root, inv); err != nil {
		t.Fatal(err)
	}
	run := func() error {
		_, err := controlplane.RunWithEnv(context.Background(), inv, "qa", controlplane.NewSSHProber(root, true), compose, envFile, "qa")
		return err
	}
	err := run()
	if err == nil || !strings.Contains(err.Error(), "E487") || !strings.Contains(err.Error(), "--trust-host-key SHA256:") {
		t.Fatalf("unknown key: %v", err)
	}
	if b, _ := os.ReadFile(log); len(b) != 0 {
		t.Fatalf("a probe or copy ran before the host key was pinned:\n%s", b)
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "known_hosts")); !os.IsNotExist(err) {
		t.Fatalf("~/.ssh/known_hosts was touched: %v", err)
	}
	fp := simHostFingerprint(t, host)
	var trustErr error
	for attempt := 0; attempt < 5; attempt++ {
		if trustErr = controlplane.TrustHostKey(context.Background(), host, fp); trustErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if trustErr != nil {
		t.Fatal(trustErr)
	}
	if err := run(); err != nil {
		t.Fatalf("pinned run: %v", err)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "rsync") || strings.Contains(string(b), "accept-new") {
		t.Fatalf("pinned run did not ship strictly:\n%s", b)
	}
}
