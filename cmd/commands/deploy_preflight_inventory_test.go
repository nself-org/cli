package commands

import (
	"github.com/nself-org/cli/internal/controlplane"
	"testing"
)

func TestDeployPreflightInventory(t *testing.T) {
	const host = "deploy@app.example.test"
	for _, target := range []string{"ops", "prod"} {
		for _, match := range []bool{false, true} {
			t.Run(target+map[bool]string{false: "-synthetic", true: "-inventory"}[match], func(t *testing.T) {
				inv := &controlplane.Inventory{Environments: map[string]controlplane.Environment{
					"other": {Servers: []controlplane.Server{{Name: "wrong", Host: host, SSHKeyRef: "WRONG_KEY"}}},
				}}
				if match {
					inv.Environments[target] = controlplane.Environment{Servers: []controlplane.Server{{Name: "selected", Host: host, SSHKeyRef: "DEPLOY_KEY", Arch: "arm64"}}}
				}
				got := deployPreflightInventory(inv, target, host)
				if len(got.Environments) != 1 {
					t.Fatal("unscoped inventory")
				}
				servers := got.Environments[target].Servers
				if len(servers) != 1 {
					t.Fatal("expected one server")
				}
				s := servers[0]
				if match {
					if s.Name != "selected" || s.SSHKeyRef != "DEPLOY_KEY" || s.Arch != "arm64" {
						t.Fatalf("lost inventory fields: %+v", s)
					}
				} else if s.Name != target+"-app" || s.SSHKeyRef != "" {
					t.Fatalf("wrong fallback: %+v", s)
				}
			})
		}
	}
}
