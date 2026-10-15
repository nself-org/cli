package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
)

// A completed remote rollback must report failure to persist its local record.
func TestRollbackRemoteV15ReportsStateSaveFailure(t *testing.T) {
	for _, mode := range []string{"unwritable-directory", "blocked-temp-file"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "unwritable-directory" && os.Geteuid() == 0 {
				t.Skip("root bypasses directory write permissions; blocked-temp-file still exercises the save failure")
			}
			r := openFrontStub(t)
			proj := t.TempDir()
			if err := deploy.SaveDeployState(proj, frontRecord(false)); err != nil {
				t.Fatal(err)
			}
			statePath := filepath.Join(proj, deploy.DeployStateFile)
			before, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}
			// Opening the remote happens after the record is read. Make the final
			// save fail without interfering with the successful remote rollback.
			deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) {
				if mode == "unwritable-directory" {
					dir := filepath.Dir(statePath)
					t.Cleanup(func() {
						if err := os.Chmod(dir, 0755); err != nil {
							t.Error(err)
						}
					})
					if err := os.Chmod(dir, 0500); err != nil {
						return nil, err
					}
				} else if err := os.Mkdir(statePath+".tmp", 0700); err != nil {
					return nil, err
				}
				return r, nil
			}
			handled, err := rollbackRemoteV15(yesCmd(), proj, "prod")
			if !handled || err == nil {
				t.Fatalf("rollback handled=%v err=%v; want a state save error", handled, err)
			}
			if !strings.Contains(err.Error(), "hosts were rolled back") || !strings.Contains(err.Error(), "record could not be saved") {
				t.Errorf("error does not distinguish remote success from local save failure: %v", err)
			}
			var pathErr *os.PathError
			if !errors.As(err, &pathErr) || pathErr.Path != statePath+".tmp" {
				t.Errorf("save error lost its filesystem cause: %v", err)
			}
			if r.idx("tar -xzf") < 0 {
				t.Fatal("remote files were not restored")
			}
			after, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("failed save changed the deploy record")
			}
		})
	}
}
