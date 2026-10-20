package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
)

func TestRollbackSaveFailure(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "writable", true: "blocked"}[blocked], func(t *testing.T) {
			proj, r := v15Project(t)
			st := frontRecord(false)
			st.PreDeployAbsent = nil // the fake remote tar contains docker-compose.yml
			if err := deploy.SaveDeployState(proj, st); err != nil {
				t.Fatal(err)
			}
			deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) {
				if blocked {
					// Block the atomic writer after the record has been read. This works
					// even under root and on Windows, unlike chmod on the state directory.
					if err := os.Mkdir(filepath.Join(proj, deploy.DeployStateFile+".tmp"), 0700); err != nil {
						t.Fatal(err)
					}
				}
				return r, nil
			}
			handled, err := rollbackRemoteV15(yesCmd(), proj, "prod")
			if !handled {
				t.Fatal("rollback was not handled")
			}
			if blocked {
				if err == nil || !strings.Contains(err.Error(), "hosts were rolled back") || !strings.Contains(err.Error(), "record was not saved") {
					t.Fatalf("missing actionable persistence error: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
