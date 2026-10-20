package commands

// Purpose: v1.5 arity dispatch and rendering for environment promotion.
// Inputs:  zero arguments for the existing canary flip, or source and target
//          inventory environment names; --dry-run, --yes and --json.
// Outputs: the deterministic plan or one promote result document.
// Constraints: v1.4 reaches the unchanged canary implementation for every
//              argument list. Dry-run never opens a remote or builds files.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

type lazyPromoteRemote struct {
	srv      controlplane.Server
	env, key string
	once     sync.Once
	r        deploy.Remote
	err      error
}

func (r *lazyPromoteRemote) open(ctx context.Context) (deploy.Remote, error) {
	r.once.Do(func() { r.r, r.err = openDeployRemote(ctx, r.srv, r.key, r.env) })
	return r.r, r.err
}
func (r *lazyPromoteRemote) Label() string {
	if r.r != nil {
		return r.r.Label()
	}
	return r.srv.Host
}
func (r *lazyPromoteRemote) Run(ctx context.Context, command string) ([]byte, error) {
	remote, err := r.open(ctx)
	if err != nil {
		return nil, err
	}
	return remote.Run(ctx, command)
}
func (r *lazyPromoteRemote) Rsync(ctx context.Context, flags []string, filesFrom, srcDir, destDir string) error {
	remote, err := r.open(ctx)
	if err != nil {
		return err
	}
	return remote.Rsync(ctx, flags, filesFrom, srcDir, destDir)
}

func runPromoteRecordRollback(cmd *cobra.Command) error {
	workdir, err := projectRoot()
	if err != nil {
		return err
	}
	ctx, release, err := cpromote.AcquireRecordLock(cmd.Context(), workdir)
	if err != nil {
		return err
	}
	defer release()
	originalContext := cmd.Context()
	cmd.SetContext(ctx)
	defer cmd.SetContext(originalContext)
	rec, err := cpromote.LoadRecord(workdir)
	if err != nil {
		return errs.Wrap("E485", "promotion record is unusable; nothing was changed", err)
	}
	attempt, err := cpromote.LoadAttempt(workdir)
	if err != nil {
		return errs.Wrap("E485", "promotion attempt is unusable; nothing was changed", err)
	}
	if attempt != nil && attempt.ApplyStarted && (attempt.Status == "in_progress" || attempt.Status == "rollback_failed") && (rec == nil || attempt.StartedAt.After(rec.StartedAt)) {
		rec = attempt
	}
	if rec == nil {
		state, stateErr := deploy.LoadDeployState(workdir)
		if stateErr != nil {
			return errs.Wrap("E485", "unusable deploy rollback record; nothing was changed", stateErr)
		}
		if handled, rollbackErr := rollbackRemoteV15(cmd, workdir, state.Target); handled {
			return rollbackErr
		}
		return errs.New("E485", "no usable v1.5 rollback record; nothing was changed")
	}
	rec, err = cpromote.LoadLatestRecord(workdir, rec.Target)
	if err != nil {
		return err
	}
	if rec == nil {
		return errs.New("E401", "the promotion record is no longer the latest target change")
	}
	return rollbackPromoteRecord(withPromoteRollbackConfirmation(cmd.Context(), cmd), workdir, rec)
}

func rollbackPromoteForTarget(ctx context.Context, workdir, target string) (bool, error) {
	ctx, release, err := cpromote.AcquireRecordLock(ctx, workdir)
	if err != nil {
		return true, err
	}
	defer release()
	rec, err := cpromote.LoadLatestRecord(workdir, target)
	if err != nil {
		return true, err
	}
	if rec == nil {
		return false, nil
	}
	return true, rollbackPromoteRecord(ctx, workdir, rec)
}

func rollbackPromoteRecord(ctx context.Context, workdir string, rec *cpromote.Record) error {
	if rec.Status == "rolled_back" {
		return errs.Newf("E485", "the last promote of %s was already rolled back; nothing was changed", rec.Target)
	}
	if done, _ := ctx.Value(promoteRollbackDBDoneKey{}).(bool); done {
		if err := cpromote.MarkDBRestoreDone(workdir, rec); err != nil {
			return err
		}
	}
	if err := cpromote.CheckDBRestoreCompletion(rec); err != nil {
		return err
	}
	inv, err := controlplane.Load(workdir)
	if err != nil {
		return err
	}
	if err := confirmPromoteRecordRollback(ctx, inv, rec); err != nil {
		return err
	}
	targets, err := controlplane.ResolveTargets(inv, controlplane.Selector{Env: rec.Target})
	if err != nil {
		return err
	}
	remotes, paths := map[string]deploy.Remote{}, map[string]string{}
	for _, target := range targets {
		var r deploy.Remote
		var openErr error
		if target.Server.Host == "" {
			r = localPromoteRemote{root: workdir}
		} else {
			r, openErr = openDeployRemote(ctx, target.Server, os.Getenv(target.Server.SSHKeyRef), rec.Target)
		}
		if openErr != nil {
			return openErr
		}
		remotes[r.Label()], paths[r.Label()] = r, promotePath(target.Server, workdir)
	}
	if err := cpromote.RollbackRecord(ctx, workdir, rec, remotes, paths); err != nil {
		var coded *errs.CLIError
		if errors.As(err, &coded) && (coded.Code == "E485" || coded.Code == "E489") {
			return err
		}
		return errs.Newf("E489", "promote rollback failed for snapshot %s backup %s; needs operator: %v", rec.SnapshotTag, rec.BackupID, err).
			WithFix(fmt.Sprintf("On %s run: nself backup restore %s --yes", rec.BackupHost, rec.BackupID))
	}
	return nil
}

func init() {
	deployRollbackCmd.Flags().Bool("db-restore-done", false, "Record operator verification that an uncertain database restore completed")
	deployPromoteCmd.Flags().Bool("dry-run", false, "Print the promotion plan without remote writes")
	deployPromoteCmd.Flags().Bool("yes", false, "Confirm promotion to a production-class target")
	deployPromoteCmd.Flags().Bool("json", false, "Emit JSON output")
}

// runDeployPromoteBody preserves the zero-argument canary flip and adds the
// v1.5 two-environment form. runDeployPromote (deploy_stream.go) wraps it for
// --stream.
func runDeployPromoteBody(cmd *cobra.Command, args []string) error {
	// compat.V15(P7-DEPL-15): deploy promote ignored args as a canary flip -> two args promote environments
	if !compat.V15() || len(args) == 0 {
		if release, _ := cmd.Flags().GetString("release"); release != "" {
			return errs.New("E401", "--release verifies an environment promotion: use deploy promote <source> <target> --release <dir>")
		}
		if streaming(cmd.Context()) {
			return errStreamNeedsEnvs()
		}
		return runDeployCanaryPromote(cmd, args)
	}
	if len(args) != 2 {
		return errs.New("E401", "deploy promote requires either no arguments or <source> <target>")
	}
	return runPromoteEnvironments(cmd, args[0], args[1])
}

// promoteEngineFn builds the promote engine. A variable so a test can run the
// stream and rollback path with fake steps instead of remote hosts.
var promoteEngineFn = preparePromoteEngine

func runPromoteEnvironments(cmd *cobra.Command, source, target string) error {
	workdir, err := projectRoot()
	if err != nil {
		return err
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	jsonOut, _ := cmd.Flags().GetBool("json")
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stop := deploy.WithInterrupt(ctx)
	defer stop()
	eng, st, err := promoteEngineFn(cmd, workdir, source, target, dryRun, jsonOut)
	if err != nil {
		return err
	}
	eng.Observer = promoteObserver(ctx, st.Target)
	if err := wirePromoteRelease(cmd, workdir, &eng, st); err != nil {
		return err
	}
	var result cpromote.Result
	if dryRun {
		result = cpromote.Result{Status: "ok", Source: st.Source, Target: st.Target, Steps: eng.Plan(), DryRun: true}
		streamPromoteResult(ctx, result)
	} else {
		result, err = eng.Run(ctx, st)
		streamPromoteResult(ctx, result)
		if err != nil {
			if result.FailedStep != "record" && st.SnapshotTag != "" {
				if wiring, ok := st.Values["wiring"].(*promoteWiring); ok {
					if saveErr := savePromoteRecord(wiring, st, result.Status); saveErr != nil {
						return errs.Wrap("E489", fmt.Sprintf("promote rollback evidence for snapshot %s backup %s could not be recorded", st.SnapshotTag, st.BackupID), errors.Join(err, saveErr)).
							WithFix(fmt.Sprintf("On %s run: nself backup restore %s --yes", st.BackupHost, st.BackupID))
					}
				}
			}
			return err
		}
	}
	if streaming(ctx) {
		return nil
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

// resolveModel isolates Compose interpolation from the operator's shell values.
func (r localPromoteRemote) resolveModel(ctx context.Context, command string) ([]byte, error) {
	c := exec.CommandContext(ctx, "sh", "-c", command) //nolint:gosec // fixed shell fragments; remote paths are validated and local paths are shell-quoted
	c.Dir = r.root
	c.Env = []string{"PATH=" + os.Getenv("PATH")}
	for _, key := range []string{"HOME", "DOCKER_CONFIG", "XDG_CONFIG_HOME"} {
		if value, ok := os.LookupEnv(key); ok {
			c.Env = append(c.Env, key+"="+value)
		}
	}
	return c.Output()
}
