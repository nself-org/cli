package commands

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
)

func TestDeployProfileOps(t *testing.T) {
	defer resetFlags(deployCmd)
	deployCmd.SetContext(context.Background())
	if deployCmd.Flags().Lookup("profile") == nil {
		t.Fatal("--profile missing")
	}
	_ = deployCmd.Flags().Set("profile", "ops")
	profile, err := selectedDeployProfile(deployCmd)
	if err != nil || profile != "ops" {
		t.Fatalf("profile=%q err=%v", profile, err)
	}
	args := deployBuildArgsForProfile(t.TempDir(), true, profile)
	if !reflect.DeepEqual(args, []string{"build", "--profile", "ops", "--deploy-remote"}) {
		t.Fatalf("ops build argv=%q", args)
	}
	if got := deployBuildArgsForProfile(t.TempDir(), false, "full"); !reflect.DeepEqual(got, []string{"build", "--profile", "app"}) {
		t.Fatalf("full build argv=%q", got)
	}
	_ = deployCmd.Flags().Set("profile", "invalid")
	if _, err := selectedDeployProfile(deployCmd); err == nil {
		t.Fatal("invalid profile accepted")
	}
	root, cleanup := newTestRoot(t)
	defer cleanup()
	t.Setenv("NSELF_DEPLOY_HOST_OPS", "")
	t.Setenv("OPS_DEPLOY_HOST", "u@legacy.test:/srv/ops")
	if target, err := resolveDeployProfileTarget("ops", "ops"); err != nil || target != "ops" {
		t.Fatalf("legacy ops target=%q err=%v", target, err)
	}
	plan := captureAccessStdout(t, func() error { printDeployProfilePlan(root, "ops", "ops"); return nil })
	if !strings.Contains(plan, "host=u@legacy.test remote_path=/srv/ops") {
		t.Fatalf("legacy ops plan=%q", plan)
	}
	oldProber := newDeployProber
	newDeployProber = func(string) controlplane.Prober { return &scopeProber{unreachable: true} }
	defer func() { newDeployProber = oldProber }()
	resetFlags(deployCmd)
	_ = deployCmd.Flags().Set("profile", "ops")
	_ = deployCmd.Flags().Set("dry-run", "true")
	output := captureAccessStdout(t, func() error { return runDeploy(deployCmd, nil) })
	if !strings.Contains(output, "Topology plan for target \"ops\"") || !strings.Contains(output, "host=u@legacy.test remote_path=/srv/ops") {
		t.Fatalf("synthesized ops host missed pipeline: %q", output)
	}
	if os.Getenv("NSELF_DEPLOY_HOST_OPS") != "" {
		t.Fatal("legacy ops host env leaked after deploy")
	}
}

func TestOpsDelegationEquivalence(t *testing.T) {
	root, cleanup := newTestRoot(t)
	defer cleanup()
	t.Setenv("NSELF_V15", "1")
	writeInventory(t, root, &controlplane.Inventory{SchemaVersion: 2, Project: "test", Environments: map[string]controlplane.Environment{
		"local": {Name: "local", Kind: "local", Tier: controlplane.TierLocal},
		"ops": {Name: "ops", Kind: "remote", Tier: controlplane.TierLocalServers, Servers: []controlplane.Server{
			{Name: "ops-app", Role: controlplane.RoleApp, Host: "u@ops.test:2222", RemotePath: "/srv/ops", Primary: true},
		}},
	}})
	oldProber := newDeployProber
	newDeployProber = func(string) controlplane.Prober { return &scopeProber{unreachable: true} }
	defer func() { newDeployProber = oldProber }()
	defer resetFlags(deployCmd)
	defer resetFlags(opsDeployCmd)
	deployCmd.SetContext(context.Background())
	opsDeployCmd.SetContext(context.Background())
	_ = deployCmd.Flags().Set("dry-run", "true")
	_ = deployCmd.Flags().Set("profile", "ops")
	direct := captureAccessStdout(t, func() error { return runDeploy(deployCmd, nil) })
	resetFlags(deployCmd)
	_ = opsDeployCmd.Flags().Set("dry-run", "true")
	canonical := captureAccessStdout(t, func() error { return runOpsDeploy(opsDeployCmd, nil) })
	args, _, err := rewriteCanonArgsWith(&canonTable, RootCmd, []string{"ops", "deploy", "--dry-run"}, true)
	if err != nil || !reflect.DeepEqual(args, []string{"deploy", "ops", "--dry-run"}) {
		t.Fatalf("legacy canonical rewrite: args=%q err=%v", args, err)
	}
	legacy := captureAccessStdout(t, func() error { return runOpsDeploy(opsDeployCmd, nil) })
	if direct != canonical || canonical != legacy || !strings.Contains(direct, "Build profile: ops; target env: ops") || !strings.Contains(direct, "host=u@ops.test:2222 remote_path=/srv/ops") || !strings.Contains(direct, "ops/ops-app") {
		t.Fatalf("plans differ\ndirect=%q\ncanonical=%q\nlegacy=%q", direct, canonical, legacy)
	}
	// A non-dry pipeline call must carry the same profile to its build step.
	oldBuild := deployBuildStepFn
	seenProfile := ""
	deployBuildStepFn = func(ctx context.Context, _ string, _ bool, steps []deployStep) ([]deployStep, error) {
		seenProfile, _ = ctx.Value(deployProfileContextKey{}).(string)
		return steps, nil
	}
	defer func() { deployBuildStepFn = oldBuild }()
	resetFlags(deployCmd)
	_ = deployCmd.Flags().Set("profile", "ops")
	if err := runDeploy(deployCmd, nil); err == nil || seenProfile != "ops" {
		t.Fatalf("pipeline build profile=%q err=%v", seenProfile, err)
	}
}
