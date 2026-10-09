package commands

// Purpose: carry the deploy compose profile into the build subprocess.
// Inputs: --profile full|ops, or the v1.5 deploy ops delegation.
// Outputs: a profile on the deploy context and its build argv.
// Constraints: full is the default; ops without an explicit env targets ops.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/ui"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/spf13/cobra"
)

type deployProfileContextKey struct{}

func init() {
	deployCmd.Flags().String("profile", "full", "Compose build profile: full or ops")
}

func selectedDeployProfile(cmd *cobra.Command) (string, error) {
	if profile, ok := cmd.Context().Value(deployProfileContextKey{}).(string); ok && profile != "" {
		return profile, nil
	}
	profile, _ := cmd.Flags().GetString("profile")
	if profile == "" {
		profile = "full"
	}
	if profile != "full" && profile != "ops" {
		return "", fmt.Errorf("invalid --profile %q (allowed: full, ops)", profile)
	}
	return profile, nil
}

func withDeployProfile(ctx context.Context, profile string) context.Context {
	return context.WithValue(ctx, deployProfileContextKey{}, profile)
}

func resolveDeployProfileTarget(raw, profile string) (string, error) {
	if profile == "ops" && strings.EqualFold(raw, "ops") && os.Getenv("NSELF_DEPLOY_HOST_OPS") == "" && os.Getenv("OPS_DEPLOY_HOST") != "" {
		return "ops", nil
	}
	return resolveTarget(raw)
}

func warnLegacyOpsDeployHost(profile string) {
	// compat.V15(P7-DEPL-14): silent legacy host fallback -> deprecation warning.
	if profile == "ops" && compat.V15() && os.Getenv("NSELF_DEPLOY_HOST_OPS") == "" && os.Getenv("OPS_DEPLOY_HOST") != "" {
		ui.Warn("OPS_DEPLOY_HOST is deprecated; use NSELF_DEPLOY_HOST_OPS")
	}
}

func synthesizeLegacyOpsHost(profile, target string) func() {
	if profile != "ops" || target != "ops" || os.Getenv("NSELF_DEPLOY_HOST_OPS") != "" {
		return func() {}
	}
	old := os.Getenv("OPS_DEPLOY_HOST")
	if old == "" {
		return func() {}
	}
	_ = os.Setenv("NSELF_DEPLOY_HOST_OPS", old)
	return func() { _ = os.Unsetenv("NSELF_DEPLOY_HOST_OPS") }
}

func opsPipelineServerFilter(workdir, target, profile, filter string) string {
	if profile != "ops" || target == "local" || filter != "" {
		return filter
	}
	if _, err := os.Stat(filepath.Join(workdir, ".nself", "control-plane.yaml")); err == nil {
		return filter
	}
	return target + "-app"
}

func printDeployProfilePlan(workdir, target, profile string) {
	if profile != "ops" {
		return
	}
	fmt.Printf("  [dry-run] Build profile: %s; target env: %s\n", profile, target)
	inv, err := controlplane.Load(workdir)
	if err == nil {
		if targets, err := controlplane.ResolveTargets(inv, controlplane.Selector{Env: target}); err == nil {
			for _, item := range targets {
				fmt.Printf("  [dry-run] host=%s remote_path=%s\n", item.Server.Host, item.Server.RemotePath)
			}
			return
		}
	}
	if host := legacyDeployHost(target); host != "" {
		path := os.Getenv("NSELF_REMOTE_PATH_" + strings.ToUpper(target))
		if spec, err := remote.ParseHostSpec(host); err == nil {
			if path == "" && spec.LegacyPath != "" {
				path = spec.LegacyPath
			}
			host = spec.String()
		}
		if path == "" {
			path = "/opt/nself"
		}
		fmt.Printf("  [dry-run] host=%s remote_path=%s\n", host, path)
	}
}
