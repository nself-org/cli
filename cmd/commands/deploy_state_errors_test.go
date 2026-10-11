package commands

// D-0144 at the callers: an unreadable or corrupt deploy record must not read
// as "no deploy". Rollback refuses; the prune knows nothing and removes less.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/deploy"
	"github.com/spf13/cobra"
)

func corruptDeployState(t *testing.T) string {
	t.Helper()
	proj := t.TempDir()
	path := filepath.Join(proj, deploy.DeployStateFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"target": "prod", "pre_deploy_tars": {`), 0o600); err != nil {
		t.Fatal(err)
	}
	return proj
}

func TestRollbackRemoteV15RefusesCorruptState(t *testing.T) {
	handled, err := rollbackRemoteV15(&cobra.Command{}, corruptDeployState(t), "prod")
	if !handled || err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("handled=%v err=%v, want a refusal that stops the old rollback", handled, err)
	}
}

func TestPreviousShippedCorruptStateIsEmpty(t *testing.T) {
	if got := previousShipped(corruptDeployState(t), "prod"); len(got) != 0 {
		t.Fatalf("previousShipped = %v, want nothing from an unreadable record", got)
	}
}
