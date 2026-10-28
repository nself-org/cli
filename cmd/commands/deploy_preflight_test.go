package commands

import (
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
)

// TestDeployPreflightInventory preserves host-specific architecture and key
// metadata while limiting both legacy deploy paths to the one deployed host.
func TestDeployPreflightInventory(t *testing.T) {
	configured := controlplane.Server{Name: "configured", Role: controlplane.RoleApp, Host: "deploy@app.example.com", SSHKeyRef: "APP_KEY", Arch: "arm64"}
	loaded := &controlplane.Inventory{Environments: map[string]controlplane.Environment{
		"production": {Name: "production", Kind: "remote", Servers: []controlplane.Server{configured, {Name: "other", Host: "other.example.com"}}},
	}}
	for _, target := range []string{"ops", "production"} {
		t.Run(target, func(t *testing.T) {
			for _, matched := range []bool{true, false} {
				host := "deploy@new.example.com"
				want := controlplane.Server{Name: target + "-app", Role: controlplane.RoleApp, Host: host, Primary: true}
				if matched {
					host, want = configured.Host, configured
				}
				got := deployPreflightInventory(loaded, target, host)
				wantInv := &controlplane.Inventory{Environments: map[string]controlplane.Environment{
					target: {Name: target, Kind: "remote", Servers: []controlplane.Server{want}},
				}}
				if !reflect.DeepEqual(got, wantInv) {
					t.Fatalf("matched=%v: inventory = %#v, want %#v", matched, got, wantInv)
				}
			}
		})
	}
}
