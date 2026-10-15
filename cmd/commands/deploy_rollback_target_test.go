package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
)

// Rollback must select credentials from the actual target, even when another
// environment uses the same host. Missing target credentials keep the fallback.
func TestRollbackRemoteV15TargetCredentials(t *testing.T) {
	for _, mode := range []string{"shared host", "unset target key", "missing target"} {
		t.Run(mode, func(t *testing.T) {
			proj, _ := v15Project(t)
			const host = "deploy@app.example.test"
			t.Setenv("NSELF_DEPLOY_SSH_KEY", "/fallback-key")
			t.Setenv("NSELF_TEST_LIVE_KEY", "/live-key")
			t.Setenv("NSELF_TEST_PROD_KEY", "/prod-key")
			live := controlplane.Server{Name: "live-app", Role: controlplane.RoleApp, Host: host, SSHKeyRef: "NSELF_TEST_LIVE_KEY", RemotePath: "/different-live-path", Primary: true}
			other := controlplane.Server{Name: "prod-app", Role: controlplane.RoleApp, Host: host, SSHKeyRef: "NSELF_TEST_PROD_KEY", RemotePath: "/different-prod-path", Primary: true}
			inv := &controlplane.Inventory{SchemaVersion: 2, Environments: map[string]controlplane.Environment{
				"local": {Name: "local", Kind: "local", Tier: controlplane.TierLocal},
				"live":  {Name: "live", Kind: "remote", Tier: controlplane.TierProd, Servers: []controlplane.Server{live}},
				"prod":  {Name: "prod", Kind: "remote", Tier: controlplane.TierProd, Servers: []controlplane.Server{other}},
			}}
			wantName, wantKey := "live-app", "/live-key"
			if mode == "unset target key" {
				t.Setenv("NSELF_TEST_LIVE_KEY", "")
				wantKey = "/fallback-key"
			}
			if mode == "missing target" {
				delete(inv.Environments, "live")
				wantKey = "/fallback-key"
			}
			if err := controlplane.Write(proj, inv); err != nil {
				t.Fatal(err)
			}
			if _, err := controlplane.Load(proj); err != nil {
				t.Fatal(err)
			}
			st := deploy.StrategyState{Target: "live",
				PreDeployTars:    map[string]string{host: "/opt/nself/.nself/backups/pre-deploy-test.tar.gz"},
				PreDeployChanged: map[string][]string{host: {"docker-compose.yml"}},
			}
			if err := deploy.SaveDeployState(proj, st); err != nil {
				t.Fatal(err)
			}
			stop := errors.New("stop before remote writes")
			opened := 0
			deployRemoteOpen = func(_ context.Context, srv controlplane.Server, key, env string) (deploy.Remote, error) {
				opened++
				if srv.Name != wantName || key != wantKey || env != "live" || srv.Host != host || srv.RemotePath != "/opt/nself" {
					t.Fatalf("opened server=%+v key=%q env=%q; want name=%s key=%s and recorded path", srv, key, env, wantName, wantKey)
				}
				return nil, stop
			}
			// Repeat to catch either ordering of the old environment-map scan.
			for i := 0; i < 32; i++ {
				if handled, err := rollbackRemoteV15(yesCmd(), proj, "live"); !handled || err == nil {
					t.Fatalf("handled=%v err=%v; want remote-open failure", handled, err)
				}
			}
			if opened != 32 {
				t.Fatalf("opened %d times, want 32", opened)
			}
		})
	}
}
