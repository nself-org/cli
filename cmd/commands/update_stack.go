package commands

// Purpose: execute the D10 stack upgrade under an exclusive project lock.
// Inputs: project configuration and --dry-run/--plan/--yes/--json.
// Outputs: human step results through ui, or one typed JSON envelope.
// Constraints: consent precedes execution; previews perform no writes or probes.
// SPORT: cap:cli.stack-upgrade, P7-PLUG-31.

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/oplock"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/nself-org/cli/internal/ui"
	"github.com/nself-org/cli/internal/upgrade/stack"
	"github.com/spf13/cobra"
)

// StackUpdateResult is the public report, excluding recovery files and row data.
type StackUpdateResult struct {
	RunID      string       `json:"run_id"`
	Status     string       `json:"status"`
	DryRun     bool         `json:"dry_run"`
	ResumeStep string       `json:"resume_step,omitempty"`
	Steps      []stack.Step `json:"steps"`
}

// stackBackend is the runtime seam; constructing it must not mutate the project.
var stackBackend = func(dir string, cfg *config.Config, w io.Writer) stack.Backend {
	return stack.NewRuntime(dir, cfg, w)
}
var stackProjectDir = resolveProjectDir
var stackLoadConfig = config.Load
var stackLock = takeStackLock

// newUpdateStackCommand constructs the subcommand and its local flags.
func newUpdateStackCommand() *cobra.Command {
	c := &cobra.Command{Use: "stack", Short: "Upgrade the stack with a verified backup and resumable steps", Args: cobra.NoArgs, RunE: runUpdateStack}
	c.Flags().Bool("dry-run", false, "List the steps without changing anything")
	c.Flags().Bool("plan", false, "List the steps without changing anything (alias of --dry-run)")
	c.Flags().Bool("yes", false, "Confirm the upgrade and non-production migration rollback")
	return c
}

func init() {
	updateCmd.AddCommand(newUpdateStackCommand())
	registerJSONType("update stack", StackUpdateResult{})
}

// runUpdateStack validates consent before configuration loading or execution.
func runUpdateStack(cmd *cobra.Command, _ []string) (err error) {
	var journal *stack.Journal
	var jsonOn bool
	defer func() { err = finishStackInvocation(cmd, journal, jsonOn, err) }()
	dry, _ := cmd.Flags().GetBool("dry-run")
	preview, _ := cmd.Flags().GetBool("plan")
	jsonOn, err = jsonModeOf(cmd)
	if err != nil {
		return err
	}
	if dry || preview {
		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve stack preview directory: %w", err)
		}
		// A fresh plan also works outside a project; existing projects use
		// their root so previews read the same journal as an authorized run.
		if project, resolveErr := stackProjectDir(); resolveErr == nil {
			dir = project
		}
		j, err := stack.Execute(cmd.Context(), stack.Options{ProjectDir: dir, DryRun: true}, nil)
		if err != nil {
			return err
		}
		journal = j
		return nil
	}
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		confirm := interactiveConfirm(cmd)
		if confirm == nil || !confirm("Upgrade this stack, including automatic database rollback on a non-production migration failure?") {
			return errs.New("E403", "update stack requires terminal confirmation or --yes")
		}
	}
	dir, err := stackProjectDir()
	if err != nil {
		return fmt.Errorf("resolve update stack project: %w", err)
	}
	cfg, err := stackLoadConfig(dir)
	if err != nil {
		return fmt.Errorf("load update stack configuration: %w", err)
	}
	release, err := stackLock(cmd, dir)
	if err != nil {
		return err
	}
	defer func() { err = settleStackRelease(cmd, err, release()) }()
	j, err := stack.Execute(cmd.Context(), stack.Options{ProjectDir: dir, Prod: reconcile.EnvClass(cfg.Env) == reconcile.ClassProd, Yes: true}, stackBackend(dir, cfg, cmd.ErrOrStderr()))
	if err != nil {
		return err
	}
	journal = j
	return nil
}

// finishStackInvocation emits the success or failure after lock cleanup.
// Inputs: completed journal and final error. Outputs: one envelope. Constraints: no duplicate on release failure.
func finishStackInvocation(cmd *cobra.Command, journal *stack.Journal, jsonOn bool, err error) error {
	if err == nil && journal != nil {
		dry, _ := cmd.Flags().GetBool("dry-run")
		preview, _ := cmd.Flags().GetBool("plan")
		if emitErr := emitStackResult(journal, dry || preview, jsonOn); emitErr != nil {
			return reportStackEmitFailure(emitErr)
		}
	}
	return finishStackError(cmd, err)
}

// settleStackRelease folds a lock-release failure into the run outcome.
// Inputs: run error and release error. Outputs: the error to return.
// Constraints: after a successful run the stack is upgraded and the journal says
// complete, so a release failure is a warning; after a failed run it joins the error.
func settleStackRelease(cmd *cobra.Command, runErr, releaseErr error) error {
	if releaseErr == nil {
		return runErr
	}
	if runErr != nil {
		return errors.Join(runErr, releaseErr)
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: the stack update finished, but releasing the project lock failed: %v\n", releaseErr)
	return nil
}

// reportStackEmitFailure reports a failed result write on stderr only.
// Inputs: the write error. Outputs: a reported error. Constraints: a second
// JSON document would follow a partly written first one, so none is emitted.
func reportStackEmitFailure(emitErr error) error {
	err := stack.WithExitClass(fmt.Errorf("emit update stack result: %w", emitErr))
	output.RenderError(pilotWriter().Err, err)
	return stackReportedError{cause: err}
}

// stackReportedError preserves the error chain after command-owned reporting.
// Inputs: classified cause. Outputs: a silent error. Constraints: main prints no duplicate.
type stackReportedError struct{ cause error }

// Error returns the original classified message.
func (e stackReportedError) Error() string { return e.cause.Error() }

// Unwrap preserves structured code and exit-class discovery.
func (e stackReportedError) Unwrap() error { return e.cause }

// Silent marks the already emitted error in both compatibility modes.
func (stackReportedError) Silent() bool { return true }

// finishStackError emits one classified failure after lock release completes.
// Inputs: command and returned error. Outputs: reported error. Constraints: no compat gate.
func finishStackError(cmd *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	err = stack.WithExitClass(err)
	jsonOn, modeErr := jsonModeOf(cmd)
	if modeErr != nil {
		return stack.WithExitClass(errors.Join(err, fmt.Errorf("resolve update stack JSON mode: %w", modeErr)))
	}
	w := pilotWriter()
	if jsonOn {
		if writeErr := output.EmitError(w, "update stack", err); writeErr != nil {
			return stack.WithExitClass(errors.Join(err, fmt.Errorf("emit update stack failure: %w", writeErr)))
		}
	} else {
		output.RenderError(w.Err, err)
	}
	return stackReportedError{cause: err}
}

// takeStackLock refuses contention in both modes and supports nested build locks.
func takeStackLock(cmd *cobra.Command, dir string) (func() error, error) {
	l, err := oplock.Acquire(cmd.Context(), dir, oplock.Opts{Command: "nself update stack"})
	if err != nil {
		if errors.Is(err, oplock.ErrHeld) {
			return nil, errs.Wrap("E460", "another operation holds the project lock", err)
		}
		return nil, fmt.Errorf("lock update stack project: %w", err)
	}
	old, had := os.LookupEnv(oplock.EnvToken)
	if err := os.Setenv(oplock.EnvToken, l.Token()); err != nil {
		l.Release()
		return nil, fmt.Errorf("export operation lock token: %w", err)
	}
	return func() error {
		l.Release()
		if had {
			if err := os.Setenv(oplock.EnvToken, old); err != nil {
				return fmt.Errorf("restore operation lock token: %w", err)
			}
		} else if err := os.Unsetenv(oplock.EnvToken); err != nil {
			return fmt.Errorf("clear operation lock token: %w", err)
		}
		return nil
	}, nil
}

// emitStackResult emits only public facts, never recovery compose or credentials.
func emitStackResult(j *stack.Journal, dry, jsonOn bool) error {
	if jsonOn {
		result := StackUpdateResult{RunID: j.RunID, Status: j.Status, DryRun: dry, Steps: j.Steps}
		if dry {
			result.ResumeStep = stack.ResumeStep(j)
		}
		return output.EmitData(pilotWriter(), "update stack", result)
	}
	if dry {
		ui.Info(fmt.Sprintf("Journal: %s; resume step: %s", j.Status, stack.ResumeStep(j)))
	}
	for _, s := range j.Steps {
		ui.Info(fmt.Sprintf("%s: %s (%d ms)", s.Name, s.Result, s.DurationMS))
	}
	return nil
}
