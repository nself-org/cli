package commands

// build_plan.go — `nself build --plan | --yes | --plan-id | --diff` (P7-LIVE-03).
//
// Purpose: one path for "what would build change" and "apply it". --plan runs
// reconcile.Compute (the real pipeline in plan mode, diffed against the disk
// and the running containers) and prints the plan; a plain `nself build` runs
// reconcile.Apply, which computes the same plan, checks --plan-id, asks for the
// confirmation a prod-class env needs, and then writes.
// Inputs: the build flags and the resolved project directory.
// Outputs: --plan prints the human plan (or, with --json, the v1 envelope whose
// data is the plan) on stdout and writes nothing. An apply prints the plan
// summary on stderr when it needs a confirmation, and with --json the applied
// plan as the envelope data. --diff prints unified diffs to stderr.
// Constraints: --plan, --yes, --plan-id and --diff are additive in both compat
// modes. The refusal of a non-interactive prod-class build (E403, exit 4) is
// v1.5 only; v1.4 prints the summary and a notice, then proceeds
// (reconcile.Confirm). The plugin lifecycle step (expired-plugin removal) never
// runs under --plan; an apply runs it after the confirmation.

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func init() {
	f := buildCmd.Flags()
	f.Bool("plan", false, "Show what build would change (files, host effects, containers) and write nothing")
	f.Bool("yes", false, "Confirm a change on a prod-class env (prod, staging) without a prompt")
	f.String("plan-id", "", "Apply only if the change still matches this plan_id (from nself build --plan --json)")
	f.Bool("diff", false, "Print unified diffs of the planned changes to stderr (env files show key names only)")
}

// buildPlanFlags are the flags of the plan path.
type buildPlanFlags struct {
	plan, yes, diff, json bool
	planID                string
}

func readBuildPlanFlags(cmd *cobra.Command) buildPlanFlags {
	var pf buildPlanFlags
	pf.plan, _ = cmd.Flags().GetBool("plan")
	pf.yes, _ = cmd.Flags().GetBool("yes")
	pf.diff, _ = cmd.Flags().GetBool("diff")
	pf.json, _ = cmd.Flags().GetBool("json")
	pf.planID, _ = cmd.Flags().GetString("plan-id")
	return pf
}

// buildRequest is the reconcile request for this invocation.
func buildRequest(cmd *cobra.Command, workdir string, opts build.BuildOptions, pf buildPlanFlags, removeOrphans bool) reconcile.Request {
	req := reconcile.Request{
		ProjectDir:    workdir,
		Command:       reconcile.CmdBuild,
		Trigger:       reconcile.Trigger{Kind: reconcile.TriggerBuild},
		Build:         opts,
		Containers:    true,
		RemoveOrphans: removeOrphans,
		Extra:         lifecycleEffects(),
		Stderr:        cmd.ErrOrStderr(),
	}
	if pf.diff {
		req.DiffOut = cmd.ErrOrStderr()
	}
	return req
}

// runBuildPlan is `nself build --plan`: compute, print, write nothing.
func runBuildPlan(cmd *cobra.Command, workdir string, opts build.BuildOptions, pf buildPlanFlags, removeOrphans bool) error {
	p, err := reconcile.Compute(context.Background(), buildRequest(cmd, workdir, opts, pf, removeOrphans))
	if err != nil {
		return fmt.Errorf("planning build: %w", err)
	}
	if pf.json {
		return output.EmitData(pilotWriter(), "build", p)
	}
	return reconcile.RenderHuman(cmd.OutOrStdout(), *p)
}

// runBuildApply is a plain `nself build`: plan, check, confirm, write.
func runBuildApply(cmd *cobra.Command, workdir string, opts build.BuildOptions, pf buildPlanFlags, force, removeOrphans, quiet bool) (*build.BuildResult, error) {
	opt := reconcile.ApplyOptions{
		Yes:    pf.yes,
		Force:  force,
		PlanID: strings.TrimSpace(pf.planID),
		BeforeWrite: func() error {
			// The expired-plugin removal is part of the plan (plugin-remove
			// effect) and runs only once it is confirmed.
			runPluginLifecycleCheck(quiet)
			return nil
		},
	}
	if !pf.json {
		opt.Interactive = interactiveConfirm(cmd)
	}
	p, res, err := reconcile.ApplyBuild(context.Background(), buildRequest(cmd, workdir, opts, pf, removeOrphans), opt)
	if err != nil {
		return nil, err
	}
	if pf.json {
		if err := output.EmitData(pilotWriter(), "build", p); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// interactiveConfirm returns a yes/no prompt on the terminal, or nil when the
// session is not interactive (stdin and stderr must both be terminals).
func interactiveConfirm(cmd *cobra.Command) func(string) bool {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stderr.Fd())) {
		return nil
	}
	return func(prompt string) bool {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s [y/N] ", prompt)
		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && line == "" {
			return false
		}
		a := strings.ToLower(strings.TrimSpace(line))
		return a == "y" || a == "yes"
	}
}

// lifecycleEffects lists the plugin removals the lifecycle step would perform,
// read only: the store is loaded and checked in memory and never saved. An
// unreadable store yields none (the step itself treats it as advisory).
func lifecycleEffects() []reconcile.Effect {
	store, err := plugin.LoadLifecycleStore()
	if err != nil {
		return nil
	}
	_, remove := store.CheckExpiry(time.Now())
	sort.Strings(remove)
	out := make([]reconcile.Effect, 0, len(remove))
	for _, name := range remove {
		out = append(out, reconcile.Effect{Kind: reconcile.EffectPluginRemove, Target: name, Detail: "license grace period exhausted"})
	}
	return out
}
