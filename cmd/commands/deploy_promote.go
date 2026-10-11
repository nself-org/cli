package commands

// Purpose: v1.5 arity dispatch and rendering for environment promotion.
// Inputs:  zero arguments for the existing canary flip, or source and target
//          inventory environment names; --dry-run, --yes and --json.
// Outputs: the deterministic plan or one promote result document.
// Constraints: v1.4 reaches the unchanged canary implementation for every
//              argument list. Dry-run never opens a remote or builds files.

import (
	"context"
	"fmt"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

func runPromoteRecordRollback(cmd *cobra.Command) error {
	workdir, err := projectRoot()
	if err != nil {
		return err
	}
	rec, err := cpromote.LoadRecord(workdir)
	if err != nil {
		return err
	}
	if rec == nil {
		return errs.New("E401", "no promotion record is available to roll back")
	}
	rec, err = cpromote.LoadLatestRecord(workdir, rec.Target)
	if err != nil {
		return err
	}
	if rec == nil {
		return errs.New("E401", "the promotion record is no longer the latest target change")
	}
	return rollbackPromoteRecord(cmd.Context(), workdir, rec)
}

func rollbackPromoteForTarget(ctx context.Context, workdir, target string) (bool, error) {
	rec, err := cpromote.LoadLatestRecord(workdir, target)
	if err != nil || rec == nil {
		return false, err
	}
	return true, rollbackPromoteRecord(ctx, workdir, rec)
}

func rollbackPromoteRecord(ctx context.Context, workdir string, rec *cpromote.Record) error {
	inv, err := controlplane.Load(workdir)
	if err != nil {
		return err
	}
	targets, err := controlplane.ResolveTargets(inv, controlplane.Selector{Env: rec.Target})
	if err != nil {
		return err
	}
	remotes, paths := map[string]deploy.Remote{}, map[string]string{}
	for _, target := range targets {
		r, openErr := promoteRemote(ctx, target.Server, rec.Target, workdir)
		if openErr != nil {
			return openErr
		}
		remotes[r.Label()], paths[r.Label()] = r, promotePath(target.Server, workdir)
	}
	if err := cpromote.RollbackRecord(ctx, workdir, rec, remotes, paths); err != nil {
		return errs.Newf("E489", "promote rollback failed for snapshot %s backup %s; needs operator: %v", rec.SnapshotTag, rec.BackupID, err)
	}
	return nil
}

func init() {
	deployPromoteCmd.Flags().Bool("dry-run", false, "Print the promotion plan without remote writes")
	deployPromoteCmd.Flags().Bool("yes", false, "Confirm promotion to a production-class target")
	deployPromoteCmd.Flags().Bool("json", false, "Emit JSON output")
}

// runDeployPromote preserves the zero-argument canary flip and adds the v1.5
// two-environment form.
func runDeployPromote(cmd *cobra.Command, args []string) error {
	// compat.V15(P7-DEPL-15): deploy promote ignored args as a canary flip -> two args promote environments
	if !compat.V15() || len(args) == 0 {
		return runDeployCanaryPromote(cmd, args)
	}
	if len(args) != 2 {
		return errs.New("E401", "deploy promote requires either no arguments or <source> <target>")
	}
	return runPromoteEnvironments(cmd, args[0], args[1])
}

func runPromoteEnvironments(cmd *cobra.Command, source, target string) error {
	workdir, err := projectRoot()
	if err != nil {
		return err
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	jsonOut, _ := cmd.Flags().GetBool("json")
	eng, st, err := preparePromoteEngine(cmd, workdir, source, target, dryRun, jsonOut)
	if err != nil {
		return err
	}
	var result cpromote.Result
	if dryRun {
		result = cpromote.Result{Status: "ok", Source: st.Source, Target: st.Target, Steps: eng.Plan(), DryRun: true}
	} else {
		result, err = eng.Run(cmd.Context(), st)
		if err != nil {
			return err
		}
	}
	if jsonOut {
		return output.EmitData(pilotWriter(), "deploy promote", result)
	}
	if dryRun {
		for _, step := range result.Steps {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-14s %s\n", step.Name, step.Detail)
		}
		return nil
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Promoted %s to %s\n", source, target)
	return nil
}
