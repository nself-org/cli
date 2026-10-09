package commands

// Purpose: report generated-file drift and repair it through reconcile.
// Inputs: a doctor invocation and its current project directory.
// Outputs: one doctor check, or the plan for --fix --plan.
// Constraints: planning does not query Docker or write project files.
import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
)

type doctorOfflineRuntime struct{}

func (doctorOfflineRuntime) Running(context.Context, string) ([]reconcile.RunningContainer, error) {
	return nil, nil
}

func (doctorOfflineRuntime) ConfigHashes(context.Context, []string, []string, string) (map[string]string, error) {
	return map[string]string{}, nil
}

func doctorDriftRequest(projectDir string) reconcile.Request {
	req := reconcile.Request{
		ProjectDir: projectDir,
		Command:    reconcile.CmdDoctorFix,
		Trigger:    reconcile.Trigger{Kind: reconcile.TriggerDrift},
		Runtime:    doctorOfflineRuntime{},
	}
	if body, err := os.ReadFile(filepath.Join(projectDir, ".env.secrets")); err == nil && len(body) > 0 {
		sum := sha256.Sum256(body)
		req.Seed = sum[:]
	}
	return req
}

func doctorProjectRoot(cwd string) string {
	root, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return ""
	}
	return root
}

func checkGeneratedDrift(ctx context.Context, cwd string) (doctorCheckResult, *reconcile.Plan, error) {
	root := doctorProjectRoot(cwd)
	if root == "" {
		return doctorCheckResult{Name: "Generated files", Status: "skip", Message: "no project root found"}, nil, nil
	}
	state, err := reconcile.LoadGeneratedState(root)
	if err != nil {
		return doctorCheckResult{Name: "Generated files", Status: "fail", Message: err.Error()}, nil, err
	}
	req := doctorDriftRequest(root)
	req.HandEdited = state.HandEditedFn(root)
	p, err := reconcile.Compute(ctx, req)
	if err != nil {
		return doctorCheckResult{Name: "Generated files", Status: "fail", Message: err.Error()}, nil, err
	}
	return driftResult(p, false), p, nil
}

func driftResult(p *reconcile.Plan, repaired bool) doctorCheckResult {
	if p == nil || len(p.Artifacts) == 0 && len(p.Effects) == 0 {
		return doctorCheckResult{Name: "Generated files", Status: "pass", Message: "no generated-file drift"}
	}
	paths := make([]string, 0, len(p.Artifacts))
	for _, a := range p.Artifacts {
		name := a.Path
		if a.HandEdited {
			name += " (hand-edited)"
		}
		paths = append(paths, name)
	}
	status, verb := "fail", "drift"
	if repaired {
		status, verb = "pass", "repaired"
	}
	return doctorCheckResult{Name: "Generated files", Status: status, Message: fmt.Sprintf("%s: %s", verb, strings.Join(paths, ", "))}
}

func runDriftFix(ctx context.Context, cmd *cobra.Command, cwd string) (doctorCheckResult, error) {
	root := doctorProjectRoot(cwd)
	if root == "" {
		return doctorCheckResult{Name: "Generated files", Status: "skip", Message: "no project root found"}, nil
	}
	yes, _ := cmd.Flags().GetBool("yes")
	force, _ := cmd.Flags().GetBool("force")
	req := doctorDriftRequest(root)
	req.Stderr = cmd.ErrOrStderr()
	opt := reconcile.ApplyOptions{Yes: yes, Force: force, Interactive: interactiveConfirm(cmd)}
	_, missingSecrets := os.Stat(filepath.Join(root, ".env.secrets"))
	p, err := reconcile.Apply(ctx, req, opt)
	if err != nil {
		return doctorCheckResult{Name: "Generated files", Status: "fail", Message: err.Error()}, err
	}
	// On a fresh project the first build creates .env.secrets. The renderer
	// also generates defaults that are not persisted there, so settle those
	// defaults against the new private seed before reporting a repaired tree.
	if os.IsNotExist(missingSecrets) {
		if _, err := os.Stat(filepath.Join(root, ".env.secrets")); err == nil {
			settled := doctorDriftRequest(root)
			settled.Stderr = cmd.ErrOrStderr()
			if _, err = reconcile.Apply(ctx, settled, opt); err != nil {
				return doctorCheckResult{Name: "Generated files", Status: "fail", Message: err.Error()}, err
			}
		}
	}
	return driftResult(p, true), nil
}

func printDoctorDriftPlan(ctx context.Context, cmd *cobra.Command, cwd string) error {
	_, p, err := checkGeneratedDrift(ctx, cwd)
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("no nself project root found")
	}
	return reconcile.RenderHuman(cmd.OutOrStdout(), *p)
}

func runDoctorDriftCheck(ctx context.Context, cmd *cobra.Command, cwd string, fix, verbose bool) (doctorCheckResult, error) {
	var r doctorCheckResult
	var err error
	if fix {
		r, err = runDriftFix(ctx, cmd, cwd)
	} else {
		r, _, _ = checkGeneratedDrift(ctx, cwd)
	}
	if r.Status != "skip" {
		printCheck(r.Status, r.Name, r.Message, verbose)
	}
	return r, err
}
