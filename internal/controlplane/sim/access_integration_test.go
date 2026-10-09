//go:build integration

package sim_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/access"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/controlplane/sim"
)

const accessSimKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMHXHuK8L4SFSmmpHWBnzPFAcJGYHjABCulfo5ZbKvum t@sim"

func TestAccessSimTierExactDryRunAndLastKey(t *testing.T) {
	skipUnlessIntegration(t)
	fleet := sim.Start(t, sim.FleetConfig{AppCount: 2})
	defer fleet.Close()
	if out, err := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output(); err == nil {
		t.Setenv("DOCKER_HOST", strings.TrimSpace(string(out)))
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("NSELF_V15", "0")
	undoAudit := access.SetAuditLogPathForTest(filepath.Join(home, "audit.log"))
	defer undoAudit()
	inv := buildInventory(fleet)
	if err := controlplane.Migrate(inv); err != nil {
		t.Fatal(err)
	}
	targets, err := controlplane.ResolveTargets(inv, controlplane.Selector{Tier: controlplane.TierLocalServers})
	if err != nil || len(targets) != 2 {
		t.Fatalf("tier targets=%d err=%v", len(targets), err)
	}
	key, err := access.LoadPublicKeyArg(accessSimKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		transport := &access.SSHTransport{Host: target.Server.Host, IdentityPath: fleet.PrivateKeyPath, RemotePath: ".ssh/nself-access-test"}
		if _, err := access.Grant(context.Background(), transport, access.GrantRequest{User: "t", Key: key, DryRun: true}); err != nil {
			t.Fatal(err)
		}
		before, err := access.List(context.Background(), transport)
		if err != nil || len(before.Entries) != 0 {
			t.Fatalf("dry-run changed %s: %+v %v", target.Server.Name, before, err)
		}
		if _, err := access.Grant(context.Background(), transport, access.GrantRequest{User: "t", Key: key}); err != nil {
			t.Fatal(err)
		}
		after, err := access.List(context.Background(), transport)
		if err != nil || len(after.Entries) != 1 || after.Entries[0].User != "t" {
			t.Fatalf("grant missed %s: %+v %v", target.Server.Name, after, err)
		}
		if _, err := access.Revoke(context.Background(), transport, access.RevokeRequest{User: "t"}); !errors.Is(err, access.ErrLastKey) {
			t.Fatalf("last-key guard on %s: %v", target.Server.Name, err)
		}
	}
}

func TestAccessSimProdUnknownHostKey(t *testing.T) {
	skipUnlessIntegration(t)
	fleet := sim.Start(t, sim.FleetConfig{AppCount: 1})
	defer fleet.Close()
	if out, err := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output(); err == nil {
		t.Setenv("DOCKER_HOST", strings.TrimSpace(string(out)))
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NSELF_V15", "1")
	host := "nself@" + fleet.Servers[0].SSHHostPort()
	transport := &access.SSHTransport{Host: host, IdentityPath: fleet.PrivateKeyPath, RemotePath: ".ssh/nself-access-test", HostKeyOptions: func(ctx context.Context) ([]string, error) {
		return controlplane.HostKeyOptions(ctx, "prod", "app", controlplane.TierProd, host, true)
	}}
	if _, err := transport.Read(context.Background()); err == nil || !strings.Contains(err.Error(), "E487") {
		t.Fatalf("unknown prod host key was not refused: %v", err)
	}
}
