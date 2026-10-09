//go:build integration

package sim_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/controlplane/sim"
)

// TestSimArchRefusal records that no deploy SSH or rsync runs on an arm64
// fixture when the only published manifest is linux/amd64.
func TestSimArchRefusal(t *testing.T) {
	skipUnlessIntegration(t)
	fleet := sim.Start(t, sim.FleetConfig{AppCount: 1})
	defer fleet.Close()
	compose, log := simDeployTools(t)
	if err := os.WriteFile(compose, []byte("services:\n  api:\n    image: example.test/amd64-only:1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	index := `{"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"digest":"sha256:abc","platform":{"os":"linux","architecture":"amd64"}}]}`
	if err := os.WriteFile(filepath.Join(filepath.Dir(compose), "docker"), []byte("#!/bin/sh\nprintf '%s\\n' '"+index+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	inv := buildInventory(fleet)
	inv.Environments["sim"] = controlplane.Environment{Name: "sim", Kind: "remote", Servers: []controlplane.Server{{Name: "sim-app-01", Role: controlplane.RoleApp, Host: "nself@" + fleet.Servers[0].SSHHostPort(), SSHKeyRef: fleet.EnvVarName, Primary: true, Arch: "arm64"}}}
	prober := &integrationStubProber{keyPath: fleet.PrivateKeyPath, keyRef: fleet.EnvVarName}
	_, err := controlplane.Run(context.Background(), inv, "sim", prober, compose)
	if err == nil || !strings.Contains(err.Error(), "E491") || !strings.Contains(err.Error(), "example.test/amd64-only:1") {
		t.Fatalf("arch refusal: %v", err)
	}
	if commands, _ := os.ReadFile(log); len(commands) != 0 {
		t.Fatalf("deploy command ran before refusal: %s", commands)
	}
}
