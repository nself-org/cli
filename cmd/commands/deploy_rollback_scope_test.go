package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
)

func TestRollbackInventoryScope(t *testing.T) {
	for _, tc := range []struct{ target, want string }{{"prod", "prod-app"}, {"live", "live-app"}} {
		t.Run(tc.target, func(t *testing.T) {
			proj, r := v15Project(t)
			st := frontRecord(false)
			st.PreDeployAbsent = nil // the fake remote tar contains docker-compose.yml
			st.Target = tc.target
			if err := deploy.SaveDeployState(proj, st); err != nil {
				t.Fatal(err)
			}
			// A matching host in a different environment must not supply its name/key.
			data := "schema_version: 2\nproject: test\nenvironments:\n  staging:\n    name: staging\n    kind: remote\n    tier: staging\n    servers:\n      - {name: wrong-app, role: app, host: deploy@app.example.test, ssh_key_ref: WRONG_ROLLBACK_KEY, remote_path: /srv/other, primary: true}\n"
			if err := os.WriteFile(filepath.Join(proj, ".nself", "control-plane.yaml"), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("WRONG_ROLLBACK_KEY", "/tmp/wrong-key")
			var got controlplane.Server
			var gotKey string
			deployRemoteOpen = func(_ context.Context, srv controlplane.Server, key, _ string) (deploy.Remote, error) {
				got, gotKey = srv, key
				return r, nil
			}
			if handled, err := rollbackRemoteV15(yesCmd(), proj, tc.target); !handled || err != nil {
				t.Fatalf("handled=%v err=%v", handled, err)
			}
			if got.Name != tc.want || gotKey == "/tmp/wrong-key" {
				t.Fatalf("used another environment: name=%s key matches wrong=%v", got.Name, gotKey == "/tmp/wrong-key")
			}
		})
	}
}
