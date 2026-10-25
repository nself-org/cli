package commands

// Purpose: tests for 'nself ops deploy' ordering (D-0276): the arch-guard
// preflight must run after the build and before DeployViaSsh, a preflight
// error must stop the deploy, and --dry-run must reach neither.
// Constraints: no build, no network, no ssh (package-var seams).

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/spf13/cobra"
)

// opsSeams records the order in which the stubbed steps run.
type opsSeams struct {
	order        []string
	preflightErr error
	host         string
}

// stubOpsSeams swaps every ops seam and restores them on cleanup. It also
// chdirs into an empty temp project and sets a fake ops host.
func stubOpsSeams(t *testing.T) *opsSeams {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("NSELF_V15", "")
	t.Setenv("NSELF_DEPLOY_HOST_OPS", "deploy@ops.example.com:/opt/nself")
	rec := &opsSeams{}
	ob, op, od := opsBuildFn, opsPreflightFn, opsDeployViaSSHFn
	opsBuildFn = func(string, build.BuildOptions) (*build.BuildResult, error) {
		rec.order = append(rec.order, "build")
		return &build.BuildResult{ComposeFile: "/fake/docker-compose.yml"}, nil
	}
	opsPreflightFn = func(_ context.Context, _, host, _ string) error {
		rec.order = append(rec.order, "preflight")
		rec.host = host
		return rec.preflightErr
	}
	opsDeployViaSSHFn = func(context.Context, deploy.SSHConfig, string) error {
		rec.order = append(rec.order, "deploy")
		return nil
	}
	t.Cleanup(func() { opsBuildFn, opsPreflightFn, opsDeployViaSSHFn = ob, op, od })
	return rec
}

func newOpsTestCmd(dryRun bool) *cobra.Command {
	cmd := &cobra.Command{Use: "deploy"}
	cmd.Flags().Bool("dry-run", dryRun, "")
	cmd.Flags().Bool("follow", false, "")
	cmd.SetContext(context.Background())
	return cmd
}

func TestRunOpsDeploy_PreflightRunsBeforeDeploy(t *testing.T) {
	rec := stubOpsSeams(t)
	if err := runOpsDeploy(newOpsTestCmd(false), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(rec.order, ","); got != "build,preflight,deploy" {
		t.Fatalf("step order = %q, want build,preflight,deploy", got)
	}
	if rec.host != "deploy@ops.example.com:/opt/nself" {
		t.Errorf("preflight host = %q", rec.host)
	}
}

func TestRunOpsDeploy_PreflightErrorStopsDeploy(t *testing.T) {
	rec := stubOpsSeams(t)
	rec.preflightErr = errors.New("image arch mismatch")
	err := runOpsDeploy(newOpsTestCmd(false), nil)
	if err == nil || !strings.Contains(err.Error(), "image arch mismatch") {
		t.Fatalf("want preflight error returned, got %v", err)
	}
	if got := strings.Join(rec.order, ","); got != "build,preflight" {
		t.Fatalf("step order = %q, want deploy never called", got)
	}
}

func TestRunOpsDeploy_DryRunSkipsBuildAndPreflight(t *testing.T) {
	rec := stubOpsSeams(t)
	if err := runOpsDeploy(newOpsTestCmd(true), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.order) != 0 {
		t.Fatalf("dry-run ran steps: %v", rec.order)
	}
}

// TestOpsPreflight_ScopesOneEnvInventory checks the inventory handed to the
// arch guard: a single "ops" environment holding a synthetic app server for the
// parsed host, scoped to env "ops".
func TestOpsPreflight_ScopesOneEnvInventory(t *testing.T) {
	t.Chdir(t.TempDir())
	var gotInv *controlplane.Inventory
	var gotEnv string
	orig := opsControlplaneCheck
	opsControlplaneCheck = func(_ context.Context, inv *controlplane.Inventory, env string, _ []string) error {
		gotInv, gotEnv = inv, env
		return nil
	}
	t.Cleanup(func() { opsControlplaneCheck = orig })

	if err := opsPreflight(context.Background(), ".", "deploy@ops.example.com:/opt/nself", "docker-compose.yml"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotEnv != "ops" || gotInv == nil {
		t.Fatalf("env=%q inv=%v", gotEnv, gotInv)
	}
	env, ok := gotInv.Environments["ops"]
	if !ok || env.Kind != "remote" || len(env.Servers) != 1 {
		t.Fatalf("bad ops environment: %+v", env)
	}
	srv := env.Servers[0]
	if srv.Name != "ops-app" || srv.Role != controlplane.RoleApp || !srv.Primary || !strings.Contains(srv.Host, "ops.example.com") {
		t.Errorf("bad synthetic server: %+v", srv)
	}
}

func TestOpsPreflight_RejectsBadHost(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := opsPreflight(context.Background(), ".", "not a host spec; rm", "docker-compose.yml"); err == nil {
		t.Fatal("want error for an invalid host spec")
	}
}

// TestOpsPreflight_ChecksOnlyShippedCompose: the ops push ships the one built
// compose file, so plugin fragments listed in the project manifest are not
// handed to the arch guard (D-0614).
func TestOpsPreflight_ChecksOnlyShippedCompose(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join(dir, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".nself", "compose-files.txt"), []byte("docker-compose.yml\ndocker-compose.plugin-x.yml\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got []string
	orig := opsControlplaneCheck
	opsControlplaneCheck = func(_ context.Context, _ *controlplane.Inventory, _ string, files []string) error {
		got = files
		return nil
	}
	t.Cleanup(func() { opsControlplaneCheck = orig })
	compose := filepath.Join(dir, "docker-compose.yml")
	if err := opsPreflight(context.Background(), dir, "deploy@ops.example.com:/opt/nself", compose); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != compose {
		t.Fatalf("arch guard files = %v, want only %s", got, compose)
	}
}
