//go:build integration

package sim_test

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
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

func simDeployTools(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "argv.log")
	for _, name := range []string{"ssh", "rsync"} {
		script := "#!/bin/sh\nprintf '%s\\n' '" + name + "' \"$@\" >> '" + log + "'\nexit 0\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	compose := filepath.Join(dir, "compose.yml")
	if err := os.WriteFile(compose, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return compose, log
}

func TestDeploySimTiers(t *testing.T) {
	skipUnlessIntegration(t)
	inv := &controlplane.Inventory{SchemaVersion: 1, Environments: map[string]controlplane.Environment{
		"local": {Name: "local", Kind: "local", Servers: []controlplane.Server{{Name: "local-app", Role: controlplane.RoleApp}}},
		"qa":    {Name: "qa", Kind: "remote", Servers: []controlplane.Server{{Name: "qa-app", Role: controlplane.RoleApp, Host: "u@localhost", Primary: true}}},
		"prod":  {Name: "prod", Kind: "remote", Servers: []controlplane.Server{{Name: "prod-app", Role: controlplane.RoleApp, Host: "u@localhost", Primary: true}}},
	}}
	if err := controlplane.Migrate(inv); err != nil {
		t.Fatal(err)
	}
	if inv.Environments["qa"].Tier != controlplane.TierLocalServers || inv.Environments["prod"].Tier != controlplane.TierProd {
		t.Fatalf("tiers: %+v", inv.Environments)
	}
	if _, err := controlplane.ResolveTargets(inv, controlplane.Selector{Tier: controlplane.TierProd}); err != nil {
		t.Fatal(err)
	}
}

func TestDeploySimNonDefaultPort(t *testing.T) {
	skipUnlessIntegration(t)
	fleet := sim.Start(t, sim.FleetConfig{AppCount: 1})
	defer fleet.Close()
	t.Setenv("NSELF_V15", "0")
	compose, log := simDeployTools(t)
	cfg := deploy.SSHConfig{Host: "nself@" + fleet.Servers[0].SSHHostPort(), KeyPath: fleet.PrivateKeyPath, RemotePath: "/tmp/nself-sim"}
	if err := deploy.DeployViaSsh(context.Background(), cfg, compose); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "-p\n") || !strings.Contains(string(b), "--\n") {
		t.Fatalf("missing port or end of options: %s", b)
	}
}

func TestDeploySimUnknownHostKey(t *testing.T) {
	skipUnlessIntegration(t)
	fleet := sim.Start(t, sim.FleetConfig{AppCount: 1})
	defer fleet.Close()
	if out, err := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output(); err == nil {
		t.Setenv("DOCKER_HOST", strings.TrimSpace(string(out)))
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NSELF_V15", "1")
	compose, log := simDeployTools(t)
	envFile := filepath.Join(t.TempDir(), ".env.qa")
	if err := os.WriteFile(envFile, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := deploy.SSHConfig{Host: "nself@" + fleet.Servers[0].SSHHostPort(), KeyPath: fleet.PrivateKeyPath, RemotePath: "/tmp/nself-sim", EnvFile: envFile, EnvName: "qa", ServerName: "qa-app"}
	t.Setenv("SIM_SSH_KEY", fleet.PrivateKeyPath)
	root := t.TempDir()
	if err := controlplane.Write(root, &controlplane.Inventory{SchemaVersion: 2, Environments: map[string]controlplane.Environment{
		"local": {Name: "local", Kind: "local", Tier: controlplane.TierLocal},
		"qa":    {Name: "qa", Kind: "remote", Tier: controlplane.TierLocalServers, Servers: []controlplane.Server{{Name: "qa-app", Role: controlplane.RoleApp, Host: cfg.Host, RemotePath: cfg.RemotePath, Primary: true}}},
	}}); err != nil {
		t.Fatal(err)
	}
	prober := controlplane.NewSSHProber(root, true)
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = stderrW
	_, _, probeErr := prober.SSHReachable(controlplane.Server{Name: "qa-app", Host: cfg.Host, SSHKeyRef: "SIM_SSH_KEY"})
	_ = stderrW.Close()
	os.Stderr = oldStderr
	if probeErr != nil {
		t.Fatal(probeErr)
	}
	probeMessage := make([]byte, 2048)
	n, _ := stderrR.Read(probeMessage)
	if strings.Count(string(probeMessage[:n]), "SHA256:") != 1 {
		t.Fatalf("probe fingerprint line: %q", probeMessage[:n])
	}
	prodCfg := cfg
	prodCfg.EnvFile, prodCfg.EnvName = "", "prod"
	if err := deploy.DeployViaSsh(context.Background(), prodCfg, compose); err == nil || !strings.Contains(err.Error(), "E487") {
		t.Fatalf("prod unknown key: %v", err)
	}
	err = deploy.DeployViaSsh(context.Background(), cfg, compose)
	if err == nil || !strings.Contains(err.Error(), "E487") || !strings.Contains(err.Error(), "--trust-host-key SHA256:") {
		t.Fatalf("unknown key: %v", err)
	}
	b, _ := os.ReadFile(log)
	if strings.Contains(string(b), "rsync") {
		t.Fatalf("rsync ran before trust: %s", b)
	}
	if err := controlplane.TrustHostKey(context.Background(), cfg.Host, "SHA256:wrong"); err == nil {
		t.Fatal("wrong fingerprint accepted")
	}
	spec, _ := remote.ParseHostSpec(cfg.Host)
	keyscanOut, err := exec.Command("ssh-keyscan", "-T", "5", "-t", "ed25519", "-p", strconv.Itoa(spec.Port), "--", spec.Host).Output()
	if err != nil {
		t.Fatal(err)
	}
	fp := ""
	for _, line := range strings.Split(string(keyscanOut), "\n") {
		if got, err := remote.Fingerprint(line); err == nil {
			fp = got
			break
		}
	}
	if fp == "" {
		t.Fatalf("no key in scan: %s", keyscanOut)
	}
	var trustErr error
	for attempt := 0; attempt < 5; attempt++ {
		trustErr = controlplane.TrustHostKey(context.Background(), cfg.Host, fp)
		if trustErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if trustErr != nil {
		t.Fatal(trustErr)
	}
	if err := deploy.DeployViaSsh(context.Background(), cfg, compose); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(log)
	if !strings.Contains(string(b), "rsync") {
		t.Fatalf("trusted deploy did not copy: %s", b)
	}
}
