package commands

// deploy_stream.go: `--stream` for `nself deploy` and
// `nself deploy promote <source> <target>` (contract:cli.deploy-progress v1).
//
// Purpose: with --json --stream stdout carries NDJSON only: one compact v1
//          envelope per progress event, then one last envelope with the
//          outcome (data.type "result"), also when the run failed.
// Inputs:  the --stream and --json flags; step records from the deploy code;
//          the promote engine's lifecycle events and result.
// Outputs: the lines (internal/deploy/progress); for a failed run the
//          human error text on stderr and a silent error that keeps the
//          exit status, so main prints no second error envelope on stdout.
// Constraints: nothing else reaches stdout while streaming: stdout is
//          redirected to stderr for the run (output.IsolateStdout), so the
//          existing human progress lines land on stderr. Without --stream
//          every helper here does nothing and the legacy output is untouched.
//          deploy promote with no arguments (the canary flip) has no stream
//          and is refused with E401.

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	dprog "github.com/nself-org/cli/internal/deploy/progress"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

// Compile-time proof that the progress adapter fits the promote engine.
var _ cpromote.Observer = (*dprog.PromoteObserver)(nil)

func init() {
	const usage = "Print NDJSON progress events and a final result line (requires --json)"
	deployCmd.Flags().Bool("stream", false, usage)
	deployPromoteCmd.Flags().Bool("stream", false, usage+"; needs <source> <target>")
}

// runDeploy is `nself deploy`: runDeployBody, streamed when --stream is set.
func runDeploy(cmd *cobra.Command, args []string) error {
	env, _ := cmd.Flags().GetString("env")
	if env == "" && len(args) == 1 {
		env = args[0]
	}
	return runStreamed(cmd, "deploy", env, func() error { return runDeployBody(cmd, args) })
}

// runDeployPromote is `nself deploy promote`: the canary flip or the two
// environment form, streamed when --stream is set. Only the two environment
// form (v1.5) has a progress stream.
func runDeployPromote(cmd *cobra.Command, args []string) error {
	run := func() error { return runDeployPromoteBody(cmd, args) }
	if !streamRequested(cmd) {
		return run()
	}
	if !jsonRequested(cmd) {
		return errStreamNeedsJSON("deploy promote")
	}
	if len(args) != 2 {
		return errStreamNeedsEnvs()
	}
	return runStreamed(cmd, "deploy promote", args[1], run)
}

func streamRequested(cmd *cobra.Command) bool {
	on, _ := cmd.Flags().GetBool("stream")
	return on
}

func jsonRequested(cmd *cobra.Command) bool {
	on, _ := cmd.Flags().GetBool("json")
	return on
}

// errStreamNeedsEnvs is the E401 for --stream on a promote that has no stream:
// the canary flip, or the two environment form in v1.4 mode (which does not
// exist there).
func errStreamNeedsEnvs() error {
	return errs.New("E401", "deploy promote --stream needs <source> <target> in v1.5 mode (NSELF_V15=1); the canary flip has no progress stream").
		WithFix("Run: nself deploy promote <source> <target> --json --stream")
}

func errStreamNeedsJSON(command string) error {
	return errs.New("E401", "--stream requires --json").WithFix("Run: nself " + command + " ... --json --stream")
}

// streamedError is a failure whose text was already written to stderr. It is
// silent, so main prints nothing (and no JSON error envelope on stdout), and it
// unwraps to the real error, so the exit status is the one the run would have
// had without --stream.
type streamedError struct{ err error }

func (e *streamedError) Error() string { return e.err.Error() }
func (e *streamedError) Unwrap() error { return e.err }
func (e *streamedError) Silent() bool  { return true }

// runStreamed runs run, and when --stream is set wraps it in the stream:
// validation, a Writer on the process stdout in the context, stdout
// redirected to stderr, and the last line. env labels the events.
func runStreamed(cmd *cobra.Command, command, env string, run func() error) error {
	if !streamRequested(cmd) {
		return run()
	}
	if !jsonRequested(cmd) {
		return errStreamNeedsJSON(command)
	}
	sw := dprog.NewWriter(pilotWriter().Out, command).At(env, "")
	defer output.IsolateStdout()()
	old := cmd.Context()
	if old == nil {
		old = context.Background()
	}
	cmd.SetContext(dprog.WithWriter(old, sw))
	defer cmd.SetContext(old)

	err := run()
	result, steps, ok := sw.Outcome()
	switch {
	case ok:
	case err == nil:
		result = dprog.ResultOK
	default:
		result = dprog.ResultFailed
	}
	if err != nil {
		reportStreamError(err)
		err = &streamedError{err: err}
	}
	if finishErr := sw.Finish(result, steps); finishErr != nil && err == nil {
		err = finishErr
	}
	return err
}

// reportStreamError writes err to stderr as main would in v1.5 mode: the coded
// block for an E-code, else "Error: <message>". --stream is new, so v1.4 mode
// has no older rendering to keep.
func reportStreamError(err error) { output.RenderError(os.Stderr, err) }

// streaming reports whether ctx carries a progress stream.
func streaming(ctx context.Context) bool { return dprog.FromContext(ctx) != nil }

// streamStart reports that step began. A no-op without --stream.
func streamStart(ctx context.Context, step string) {
	dprog.FromContext(ctx).Event(step, dprog.PhaseStart, "")
}

// announce prints the human progress line unless JSON output is on, then
// reports the start of step to the stream.
func announce(ctx context.Context, jsonOut bool, step, line string) {
	if !jsonOut {
		fmt.Println(line)
	}
	streamStart(ctx, step)
}

// addStep appends s to steps and reports its outcome to the stream. A pending
// step is a dry-run plan row and is reported as skipped.
func addStep(ctx context.Context, steps []deployStep, s deployStep) []deployStep {
	detail := ""
	if s.Status == "pending" {
		detail = "planned, not run (dry-run)"
	}
	dprog.FromContext(ctx).Status(s.Name, s.Status, detail)
	return append(steps, s)
}

// streamStepFailed reports a step that failed after it was recorded as done.
func streamStepFailed(ctx context.Context, step, detail string) {
	dprog.FromContext(ctx).Status(step, "failed", detail)
}

// streamTopology reports a dry-run pipeline plan: one skipped event per
// visible server.
func streamTopology(ctx context.Context, statuses []controlplane.TargetStatus) {
	w := dprog.FromContext(ctx)
	for _, ts := range statuses {
		if ts.Capability == controlplane.CapHidden {
			continue
		}
		w.At(ts.Env, ts.Server).Status("Deploy "+ts.Server, "planned", "dry-run: capability "+string(ts.Capability))
	}
}

// streamServers reports the outcome of each server of a pipeline run, then the
// pipeline step itself.
func streamServers(ctx context.Context, res *controlplane.DeployResult, runErr error) {
	w := dprog.FromContext(ctx)
	if res != nil {
		for _, sr := range res.Servers {
			status, detail := "done", ""
			switch sr.Status {
			case "skipped":
				status, detail = "skipped", "read-only capability"
			case "failed":
				status = "failed"
				if sr.Err != nil {
					detail = sr.Err.Error()
				}
			}
			w.At(sr.Env, sr.Server).Status("Deploy "+sr.Server, status, detail)
		}
	}
	switch {
	case runErr != nil:
		w.Status("Deploy pipeline", "failed", runErr.Error())
	case res != nil && (res.PrimarySkipped || failedServersError(res) != nil):
		w.Status("Deploy pipeline", "failed", "not every server was deployed")
	default:
		w.Status("Deploy pipeline", "done", "")
	}
}

// promoteObserver returns the engine observer for the stream in ctx, or nil
// (an untyped nil, so the engine's nil check holds) without --stream.
func promoteObserver(ctx context.Context, env string) cpromote.Observer {
	if w := dprog.FromContext(ctx); w != nil {
		return dprog.NewPromoteObserver(w, env)
	}
	return nil
}

// streamPromoteResult hands the engine's result to the stream: it becomes the
// last line. A dry run has no engine events, so its plan is reported as
// skipped events first.
func streamPromoteResult(ctx context.Context, r cpromote.Result) {
	w := dprog.FromContext(ctx)
	if w == nil {
		return
	}
	steps := make([]dprog.Step, 0, len(r.Steps))
	for _, s := range r.Steps {
		if r.DryRun {
			w.At(r.Target, "").Status(s.Name, "planned", s.Detail)
		}
		steps = append(steps, dprog.Step{Step: s.Name, Status: s.Status})
	}
	w.SetOutcome(dprog.ResultForPromote(r.Status), steps)
}

// finalizeFor is finalize with the legacy JSON document switched off while
// streaming: the run prints its human summary (to stderr) and the stream's last
// line carries the outcome.
func finalizeFor(ctx context.Context, jsonOut bool, target, strategy string, start time.Time, steps []deployStep, err error) error {
	return finalize(jsonOut && !streaming(ctx), target, strategy, start, steps, err)
}

// streamStepDone reports that step finished, for a step that has no row in the
// legacy step list.
func streamStepDone(ctx context.Context, step string) {
	dprog.FromContext(ctx).Status(step, "done", "")
}
