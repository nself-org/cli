package commands

// Purpose: runDeploy, the RunE for the top-level "nself deploy" command. Inputs
// are the cobra command/args (target, strategy, flags); outputs are deploy step
// results printed as text or JSON, or a non-nil error on failure.
// Constraints: split out of deploy.go (CLI-R12) as a pure move, no behavior change.
// The blue/green canary path and the T05 control-plane pipeline path (both
// terminal, self-contained branches of runDeploy) were further extracted to
// deploy_run_bluegreen.go and deploy_run_pipeline.go for 300-line compliance
// (T-P6-E2-W1-S1-T3), superseding the prior CLI-R12 "cannot be split" note.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/maintenance"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// deployBuildStepFn is the build step. A variable so tests can prove a path
// never builds (or records that it did) without running docker.
var deployBuildStepFn = runDeployBuildStep

func runDeploy(cmd *cobra.Command, args []string) error {
	// Resolve target: --env flag takes priority over the positional argument.
	envFlag, _ := cmd.Flags().GetString("env")
	var rawTarget string
	switch {
	case envFlag != "":
		rawTarget = envFlag
	case len(args) == 1:
		rawTarget = args[0]
	default:
		return fmt.Errorf("target environment required: pass it as an argument (nself deploy staging) or via --env (nself deploy --env staging)")
	}
	target, err := resolveTarget(rawTarget)
	if err != nil {
		return err
	}

	// Load the env file cascade for the resolved target into the current process
	// so that downstream helpers (build, health checks, SSH env) pick up the
	// correct environment variables. NSELF_DEPLOY_ENV is also set for subprocesses.
	// workdir may not be known yet; use a best-effort lookup here.
	if wd, wdErr := projectRoot(); wdErr == nil {
		loadDeployEnvCascade(wd, target)
	}

	strategy, _ := cmd.Flags().GetString("strategy")
	if rolling, _ := cmd.Flags().GetBool("rolling"); rolling {
		strategy = "rolling"
	}
	if !deployStrategies[strategy] {
		return fmt.Errorf("invalid strategy %q (allowed: rolling, blue-green, canary, preview)", strategy)
	}

	// Strategies other than rolling are not yet implemented. Fall back to
	// rolling with an explicit warning so users know the flag was accepted but
	// has no effect yet.
	if notYetImplementedStrategies[strategy] {
		if !func() bool { v, _ := cmd.Flags().GetBool("json"); return v }() {
			ui.Warn(fmt.Sprintf("Strategy %q is not yet implemented in v1.0.9. Tracked for v1.1.0; falling back to rolling. See .claude/docs/operations/deploy-strategies.md", strategy))
		}
		strategy = "rolling"
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")
	yes, _ := cmd.Flags().GetBool("yes")
	if yes {
		force = true
	}
	follow, _ := cmd.Flags().GetBool("follow")
	skipHealth, _ := cmd.Flags().GetBool("skip-health")
	jsonOut, _ := cmd.Flags().GetBool("json")
	includeFrontends, _ := cmd.Flags().GetBool("include-frontends")
	excludeFrontends, _ := cmd.Flags().GetBool("exclude-frontends")
	canaryPct, _ := cmd.Flags().GetInt("canary")
	skipCanary, _ := cmd.Flags().GetBool("skip-canary")
	forceMigration, _ := cmd.Flags().GetBool("force-migration")
	serverFilter, _ := cmd.Flags().GetString("server")

	workdir, err := projectRoot()
	if err != nil {
		return err
	}

	// Production safety gate: every prod-class environment (name prod or
	// production today, tier prod once P7-DEPL-13 lands, or a custom name the
	// inventory marks prod-class) needs --force/--yes unless --dry-run. It runs
	// before every deploy path, blue/green included, so no path skips it.
	var gateInv *controlplane.Inventory
	if target != "local" {
		gateInv, _ = controlplane.Load(workdir)
	}
	if prodClassFn(gateInv, target) && !dryRun && !force {
		return fmt.Errorf("production deploy requires --force (or --dry-run). Re-run with --force once ready")
	}

	// Blue/green canary path (Y17 — blue_green_deploy feature flag) and the T05
	// control-plane pipeline path are extracted to deploy_run_bluegreen.go and
	// deploy_run_pipeline.go (T-P6-E2-W1-S1-T3) for 300-line compliance. Each
	// returns handled=false to fall through to the next path unchanged when it
	// doesn't apply, preserving the original branch order exactly.
	// Blue/green drives the local docker stack and ignores the named env, so a
	// remote env must be refused before it can touch anything.
	if target != "local" && (canaryPct > 0 || skipCanary) && os.Getenv("NSELF_FEATURE_BLUE_GREEN_DEPLOY") == "true" {
		return fmt.Errorf("blue/green deploy (--canary/--skip-canary) applies to the local target only; refusing env %q, nothing was changed", target)
	}
	if handled, bgErr := runDeployBlueGreenCanary(cmd, target, workdir, canaryPct, skipCanary, forceMigration, dryRun, force, jsonOut); handled {
		return bgErr
	}

	if !jsonOut {
		ui.CommandHeader(fmt.Sprintf("nself deploy %s", target), fmt.Sprintf("strategy=%s dry-run=%v include-frontends=%v exclude-frontends=%v", strategy, dryRun, includeFrontends, excludeFrontends))
	}

	if handled, pipeErr := runDeployControlPlanePipeline(cmd, workdir, target, strategy, serverFilter, dryRun, jsonOut); handled {
		return pipeErr
	}

	// A remote environment is only deployable through the single-host path when
	// it names a host. Refuse before building, staging and prod included: a
	// missing host must never fall back to deploying on this machine.
	if target != "local" && legacyDeployHost(target) == "" {
		return errs.New("E483", fmt.Sprintf("invalid target %q: no host for this environment (set %s or add it to .nself/control-plane.yaml)", target, deployHostEnvVar(target)))
	}

	steps := []deployStep{}
	start := time.Now()

	// Build. runDeployBuildStep also refuses to continue when the compose
	// file the rolling restart below is about to read was not actually
	// (re)generated by this build call — see deploy_build_root.go.
	if !dryRun {
		if !jsonOut {
			fmt.Println("  [running] Build images")
		}
		var buildErr error
		if steps, buildErr = deployBuildStepFn(cmd.Context(), workdir, steps); buildErr != nil {
			return finalize(jsonOut, target, strategy, start, steps, buildErr)
		}
	}
	steps = append(steps, deployStep{Name: "Build images", Status: stepStatus(dryRun, "done")})
	if !jsonOut && !dryRun {
		fmt.Println("  [done] Build images")
	}

	// Dry-run only: name, per custom service, which env vars it will
	// receive — see deploy_dry_run_env.go.
	if dryRun && !jsonOut {
		printCustomServiceEnvSummary(workdir)
	}

	// Target-specific action
	switch target {
	case "local":
		if dryRun {
			if !jsonOut {
				fmt.Printf("  [dry-run] Would: docker compose up -d (rolling: %v, frontends: include=%v exclude=%v)\n", strategy, includeFrontends, excludeFrontends)
			}
			steps = append(steps, deployStep{Name: "Start local stack", Status: "pending"})
		} else {
			if !jsonOut {
				fmt.Println("  [running] Start local stack (rolling sequenced restart)")
			}
			restartSteps, restartErr := runRollingRestart(cmd.Context(), workdir, jsonOut)
			steps = append(steps, restartSteps...)
			if restartErr != nil {
				return finalize(jsonOut, target, strategy, start, steps, restartErr)
			}
			if metaErr := applyLocalHasuraMetadataAfterDeploy(cmd.Context(), workdir, jsonOut); metaErr != nil {
				return finalize(jsonOut, target, strategy, start, steps, metaErr)
			}
		}

	default: // any remote environment: staging, prod, or a custom inventory env
		host := legacyDeployHost(target)

		if dryRun {
			if !jsonOut {
				fmt.Printf("  [dry-run] Would: ssh+rsync to %s then docker compose pull + rolling restart\n", host)
				fmt.Printf("  [dry-run] SSH key: %s\n", sshKeyPath())
				fmt.Printf("  [dry-run] Rolling restart order: resolved at run time from the project's compose file (core services %s first, then the rest, then any plugin services last)\n", strings.Join(composeCoreOrder, " → "))
				fmt.Printf("  [dry-run] Frontends: include=%v exclude=%v\n", includeFrontends, excludeFrontends)
			}
			steps = append(steps, deployStep{Name: fmt.Sprintf("Push artefacts to %s", host), Status: "pending"})
			steps = append(steps, deployStep{Name: "Rolling restart (sequenced)", Status: "pending"})
		} else {
			// Remote push: rsync compose file + env + migrations, then pull images
			// and run the rolling restart on the remote host via ssh.
			if !jsonOut {
				fmt.Printf("  [running] Remote push to %s\n", host)
			}
			pushErr := remoteDeployPushFn(cmd.Context(), workdir, host, target, jsonOut)
			if pushErr != nil {
				steps = append(steps, deployStep{Name: fmt.Sprintf("Push artefacts to %s", host), Status: "failed"})
				return finalize(jsonOut, target, strategy, start, steps, pushErr)
			}
			steps = append(steps, deployStep{Name: fmt.Sprintf("Push artefacts to %s", host), Status: "done"})
		}

		// Health gate (post-restart).
		if skipHealth {
			if !jsonOut {
				ui.Warn("Skipping health checks (--skip-health). Stack state unverified.")
			}
			steps = append(steps, deployStep{Name: "Health checks", Status: "skipped"})
		} else if !dryRun {
			healthStep, healthErr := runDeployHealthCheck(cmd.Context(), workdir, jsonOut)
			steps = append(steps, healthStep)
			if healthErr != nil {
				return finalize(jsonOut, target, strategy, start, steps, healthErr)
			}
		} else {
			steps = append(steps, deployStep{Name: "Health checks", Status: "pending"})
		}
	}

	// Auto-install daily disk-cleanup timer after successful staging/prod deploy (P98 T10.T07).
	// Non-fatal: a failure here warns but does not roll back the deploy.
	if !dryRun && target != "local" {
		if timerErr := maintenance.InstallDailyTimer(); timerErr != nil {
			ui.Warn(fmt.Sprintf("daily maintenance timer install failed (non-fatal): %v", timerErr))
			ui.Warn("Run `nself maintenance schedule --daily` manually to enable disk-cleanup cron")
		}
	}

	if err := finalize(jsonOut, target, strategy, start, steps, nil); err != nil {
		return err
	}

	// --follow: stream container logs until Ctrl-C (staging/prod only).
	// Not supported for dry-run or local (local already has foreground start).
	if follow && !dryRun && target != "local" {
		if !jsonOut {
			ui.Info("Following container logs (Ctrl-C to stop)...")
		}
		host := legacyDeployHost(target)
		if host != "" {
			// Remote follow: tail logs on the remote host via SSH.
			colonIdx := strings.LastIndex(host, ":")
			sshTarget := host
			remotePath := ""
			if colonIdx >= 0 {
				sshTarget = host[:colonIdx]
				remotePath = host[colonIdx+1:]
			}
			sshKey := sshKeyPath()
			logsCmd := "docker compose logs -f --tail=50"
			if remotePath != "" {
				logsCmd = fmt.Sprintf("cd %s && docker compose logs -f --tail=50", remotePath)
			}
			sc := exec.CommandContext(cmd.Context(), "ssh",
				"-i", sshKey,
				"-o", "StrictHostKeyChecking=accept-new",
				"-o", "ForwardAgent=no",
				sshTarget, logsCmd)
			sc.Stdout = os.Stdout
			sc.Stderr = os.Stderr
			_ = sc.Run() // Ctrl-C exits; non-zero exit is not an error from user perspective.
		} else {
			// Local host: stream logs directly.
			lc := exec.CommandContext(cmd.Context(), "docker", "compose", "logs", "-f", "--tail=50")
			lc.Dir = workdir
			lc.Stdout = os.Stdout
			lc.Stderr = os.Stderr
			_ = lc.Run()
		}
	}

	return nil
}
