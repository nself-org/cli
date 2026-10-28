package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
)

func TestRollbackInventoryUsesOnlyTargetEnvironment(t *testing.T) {
	for _, targetPresent := range []bool{true, false} {
		name := "target present"
		if !targetPresent {
			name = "target absent"
		}
		t.Run(name, func(t *testing.T) {
			proj, _ := v15Project(t)
			const host = "deploy@app.example.test"
			t.Setenv("NSELF_TEST_PROD_KEY", "/keys/prod")
			t.Setenv("NSELF_TEST_OTHER_KEY", "/keys/other")
			t.Setenv("NSELF_DEPLOY_SSH_KEY", "/keys/fallback")
			envs := map[string]controlplane.Environment{
				"other": {Name: "other", Kind: "remote", Servers: []controlplane.Server{{
					Name: "other-app", Role: controlplane.RoleApp, Host: host,
					SSHKeyRef: "NSELF_TEST_OTHER_KEY", RemotePath: "/other", Primary: true,
				}}},
			}
			wantName, wantKey := "prod-app", "/keys/fallback"
			if targetPresent {
				envs["prod"] = controlplane.Environment{Name: "prod", Kind: "remote", Servers: []controlplane.Server{{
					Name: "target-app", Role: controlplane.RoleApp, Host: host,
					SSHKeyRef: "NSELF_TEST_PROD_KEY", RemotePath: "/inventory-path", Primary: true,
				}}}
				wantName, wantKey = "target-app", "/keys/prod"
			}
			writeInventory(t, proj, &controlplane.Inventory{SchemaVersion: 2, Environments: envs})
			rollbackRecord(t, proj, host, host)
			stop := errors.New("test stops before remote operations")
			opened := false
			deployRemoteOpen = func(_ context.Context, srv controlplane.Server, key, env string) (deploy.Remote, error) {
				opened = true
				if srv.Name != wantName || key != wantKey || env != "prod" || srv.Host != host || srv.RemotePath != "/opt/nself" {
					t.Errorf("rollback opened server=%+v key=%q env=%q; want name=%q key=%q and recorded path", srv, key, env, wantName, wantKey)
				}
				return nil, stop
			}
			if handled, err := rollbackRemoteV15(yesCmd(), proj, "prod"); !handled || err == nil || !opened {
				t.Fatalf("rollback handled=%v err=%v opened=%v", handled, err, opened)
			}
		})
	}
}
