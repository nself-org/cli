package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/deploy"
)

type promoteSignalRemote struct {
	*v15Remote
	root        string
	provisional bool
	proof       *promoteSafetyRemote
}

func (r *promoteSignalRemote) Run(ctx context.Context, command string) ([]byte, error) {
	if strings.Contains(command, "db migrate") {
		rec, err := cpromote.LoadRecord(r.root)
		r.provisional = err == nil && rec != nil && rec.Status == "in_progress" && rec.MigrateStarted && rec.BackupID != "" && rec.SnapshotTag != "" && len(rec.PreDeployTar) > 0
		fmt.Println("READY")
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if strings.Contains(command, "{{.Id}}") {
		return []byte("sha256:" + strings.Repeat("a", 64) + " web:1\n"), nil
	}
	raw, err := r.v15Remote.Run(ctx, command)
	if err == nil && (strings.Contains(command, "images --format json") || strings.Contains(command, " ps --all --format json") || strings.Contains(command, "image inspect") || strings.Contains(command, "config --format json --no-env-resolution") || strings.Contains(command, "__NSELF_ABSENT__")) {
		return r.proof.Run(ctx, command)
	}
	return raw, err
}

func TestPromoteInterruptDuringMigrate(t *testing.T) {
	want := "RESULT e488=true restored=true provisional=true rolledback=true"
	if got := runSignalChild(t, "TestPromoteInterruptHelper"); got != want {
		t.Fatalf("promotion interruption = %q, want %q", got, want)
	}
}

func TestPromoteInterruptHelper(t *testing.T) {
	if os.Getenv(rollbackSignalEnv) != "1" {
		return
	}
	compattest.Set(t, true)
	locked := compose.LockedImages()[0]
	source, proof := setupPromoteSafety(t, locked.LegacyRef, locked.String(), false)
	root := mustProjectRoot(t)
	// Build services require no external registry or SSH architecture preflight.
	wf(t, filepath.Join(root, "docker-compose.yml"), "services:\n  api:\n    image: "+locked.LegacyRef+"\n    build: .\n")
	key := filepath.Join(t.TempDir(), "key")
	wf(t, key, "fixture")
	t.Setenv("TEST_KEY", key)
	target := &promoteSignalRemote{v15Remote: &v15Remote{label: "prod-app"}, root: root, proof: proof}
	deployRemoteOpen = func(_ context.Context, srv controlplane.Server, _, _ string) (deploy.Remote, error) {
		if srv.Name == "ls-app" {
			return source, nil
		}
		return target, nil
	}
	err := runPromoteEnvironments(promoteTestCommand(&bytes.Buffer{}, false, true, false), "ls", "prodsim")
	rec, recordErr := cpromote.LoadRecord(root)
	fmt.Printf("RESULT e488=%t restored=%t provisional=%t rolledback=%t\n", err != nil && strings.Contains(err.Error(), "E488"), firstIndex(target.commands(), "nself backup restore") >= 0, target.provisional, recordErr == nil && rec != nil && rec.Status == "rolled_back")
}
