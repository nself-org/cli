package commands

// Purpose: shared deploy-strategy tables and the rolling-restart and health-check
// helpers used by runDeploy. Inputs are the target workdir and strategy name;
// outputs are per-step results consumed by the deploy command's summary.
// Constraints: every docker call goes through internal/docker (docker funnel);
// the rolling restart reads the compose manifest and env files like restart.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/hasura"
)

// notYetImplementedStrategies lists strategies that fall back to rolling with
// an explicit warning. Tracked for v1.1.0.
// Note: blue-green and canary are now implemented via --canary N when the
// blue_green_deploy feature flag (Y17) is ON. The --strategy=blue-green/canary
// path still falls back to rolling for backwards compat with existing scripts.
var notYetImplementedStrategies = map[string]bool{
	"blue-green": true,
	"canary":     true,
	"preview":    true,
}

// deployServiceOrder is the illustrative fallback order shown by --dry-run
// when the project's actual compose services cannot be queried (dry-run
// never runs docker). It is NOT used by the real rolling restart — that
// derives its order from the project's own resolved compose file via
// projectServiceOrder (deploy_service_order.go), because a fixed list
// cannot know a given project's real service names (see that file's header
// for the production incident this fixes).
var deployServiceOrder = []string{
	"postgres",
	"hasura",
	"auth",
	"storage",
	"plugins",
}

// rollingPlan is everything one rolling restart needs. compose carries the
// manifest files and env files; clock is injectable so the gate's 30 s and
// 60 s limits are testable without waiting.
type rollingPlan struct {
	compose *docker.Compose
	workdir string
	order   []string
	specs   []docker.ServiceSpec
	clock   docker.Clock
	jsonOut bool
}

// runRollingRestart performs a per-service sequenced restart with a health
// gate between each service (D17). Everything goes through docker.NewCompose
// over .nself/compose-files.txt with build.ComposeEnvFiles, as restart does,
// so plugin fragments and the variables computed for them are included. The
// order is `docker compose config --services` over that manifest (plugin
// services last), never a hardcoded list: a fixed list restarted a service
// that did not exist and aborted a real deploy half way (production incident
// 2026-09-20). Each service is brought up with `up -d --no-deps` and then
// gated by kind: healthcheck declared -> healthy within 60s (longer when the
// healthcheck's own timing needs it, up to 10m); restart "no"
// without a healthcheck (one-shot) -> a new run that exited 0; otherwise
// running within 30s and still the same run a poll later. Any kind fails at
// once on a container that exited non-zero or keeps restarting. The deploy
// halts on the first failure and names the service.
func runRollingRestart(ctx context.Context, workdir string, jsonOut bool) ([]deployStep, error) {
	compose, files, err := deployCompose(workdir)
	if err != nil {
		return nil, fmt.Errorf("rolling restart: %w", err)
	}
	order, err := projectServiceOrder(ctx, compose, workdir, files)
	if err != nil {
		return nil, fmt.Errorf("rolling restart: resolving project service order: %w", err)
	}
	specs, err := docker.ComposeServiceSpecs(ctx, compose, workdir)
	if err != nil {
		return nil, fmt.Errorf("rolling restart: reading resolved service config: %w", err)
	}
	return rollingRestart(ctx, rollingPlan{compose: compose, workdir: workdir, order: order, specs: specs, jsonOut: jsonOut})
}

// rollingRestart runs plan. Every service's spec is checked first, so E252
// fires before anything restarts. Then each service is brought up and gated.
// After the last one, every gated service is read once more: a crash loop
// slower than the gate's two polls shows up as a restart since its gate passed.
func rollingRestart(ctx context.Context, plan rollingPlan) ([]deployStep, error) {
	byName := make(map[string]docker.ServiceSpec, len(plan.specs))
	for _, sp := range plan.specs {
		byName[sp.Name] = sp
	}
	for _, svc := range plan.order {
		if _, ok := byName[svc]; !ok {
			// config --services and config --format json read the same manifest; a
			// miss means they disagree, and a guessed gate could skip a wait.
			err := fmt.Errorf("service %s is in the restart order but missing from the resolved compose config", svc)
			return addStep(ctx, nil, deployStep{Name: "Restart " + svc, Status: "failed"}), errs.Wrap("E252", "rolling restart: "+err.Error(), err)
		}
	}
	steps := []deployStep{}
	var gated []gatedService
	for _, svc := range plan.order {
		stepName := fmt.Sprintf("Restart %s", svc)
		announce(ctx, plan.jsonOut, stepName, fmt.Sprintf("  [running] Restart %s (sequenced rolling)", svc))
		spec := byName[svc]
		marks, err := plan.compose.StartMarks(ctx, plan.workdir, svc)
		if err != nil {
			return addStep(ctx, steps, deployStep{Name: stepName, Status: "failed"}), gateError(svc, markErr(ctx, err))
		}
		if err := rollingUp(ctx, plan, svc); err != nil {
			return addStep(ctx, steps, deployStep{Name: stepName, Status: "failed"}), err
		}
		if !plan.jsonOut {
			fmt.Printf("  [waiting] Waiting for %s (%s)\n", svc, gateKind(spec))
		}
		var seen []docker.ContainerState // the last read, i.e. the one that passed the gate
		states := func(c context.Context) ([]docker.ContainerState, error) {
			cs, err := plan.compose.FreshStates(c, plan.workdir, svc, marks)
			if err == nil {
				seen = cs
			}
			return cs, err
		}
		if err := docker.HealthGate(ctx, spec, states, plan.clock); err != nil {
			return addStep(ctx, steps, deployStep{Name: stepName, Status: gateFailStatus(err)}), gateError(svc, err)
		}
		steps = addStep(ctx, steps, deployStep{Name: stepName, Status: "done"})
		gated = append(gated, gatedService{name: svc, step: len(steps) - 1, seen: seen})
		if !plan.jsonOut {
			fmt.Printf("  [done] %s ready\n", svc)
		}
	}
	return recheckGated(ctx, plan, steps, gated)
}

// gatedService is a service whose gate passed: its step index and the
// containers as the passing read saw them.
type gatedService struct {
	name string
	step int
	seen []docker.ContainerState
}

// recheckGated reads every gated service once more and fails E250 on the first
// that restarted or exited non-zero since its gate passed. It costs one ps and
// inspect per service and no sleep. A crash after this read is not detected.
func recheckGated(ctx context.Context, plan rollingPlan, steps []deployStep, gated []gatedService) ([]deployStep, error) {
	if !plan.jsonOut && len(gated) > 0 {
		fmt.Printf("  [waiting] Re-checking %d services\n", len(gated))
	}
	for _, g := range gated {
		cs, err := plan.compose.FreshStates(ctx, plan.workdir, g.name, nil)
		if err == nil {
			err = docker.Regressed(g.seen, cs)
		} else {
			err = markErr(ctx, err)
		}
		if err != nil {
			steps[g.step].Status = "failed"
			streamStepFailed(ctx, steps[g.step].Name, "failed the final re-check: "+err.Error())
			return steps, gateError(g.name, err)
		}
	}
	return steps, nil
}

// markErr classes a failed pre-restart state read: the user's interrupt as
// such, anything else as an unreadable state.
func markErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("%w: %w", docker.ErrStateRead, err)
}

// gateKind names what the gate waits for, for the progress line, with the
// limit HealthGate really applies (a slow healthcheck waits longer than 60s).
func gateKind(sp docker.ServiceSpec) string {
	limit := fmt.Sprintf("max %ds", int(docker.GateLimit(sp)/time.Second))
	switch {
	case sp.OneShot():
		return "one-shot: exit 0, " + limit
	case sp.HasHealthcheck:
		return "healthy, " + limit
	default:
		return "running, " + limit
	}
}

// gateFailStatus is the step status for a gate error.
func gateFailStatus(err error) string {
	if errors.Is(err, docker.ErrGateTimeout) {
		return "unhealthy"
	}
	return "failed"
}

// gateError turns a gate failure into the error the operator sees, by what
// went wrong: E250 when the service is broken (a container exited non-zero or
// keeps restarting), E251 when it was too slow, and no service code when the
// cause is outside the service (docker could not be read, or the user
// interrupted), so the message never blames a healthy service.
func gateError(svc string, err error) error {
	hint := fmt.Sprintf("Run 'nself logs %s' for details", svc)
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("rolling restart interrupted at service %s: %w", svc, err)
	case errors.Is(err, docker.ErrStateRead):
		return fmt.Errorf("rolling restart: cannot read the state of service %s, so it was not judged: %w", svc, err)
	case errors.Is(err, docker.ErrOneShotFailed), errors.Is(err, docker.ErrServiceCrashed):
		return errs.Wrap("E250", fmt.Sprintf("rolling restart: %v. %s", err, hint), err)
	case errors.Is(err, docker.ErrGateTimeout):
		return errs.Wrap("E251", fmt.Sprintf("rolling restart: %v. %s", err, hint), err)
	}
	return fmt.Errorf("rolling restart: service %s: %w", svc, err)
}

// runDeployHealthCheck calls nself doctor and gates the deploy result.
// Returns an error with the failed service name when any service is unhealthy.
func runDeployHealthCheck(ctx context.Context, workdir string, jsonOut bool) (deployStep, error) {
	announce(ctx, jsonOut, "Health checks", "  [running] Health checks (calling nself health)")
	bin, err := os.Executable()
	if err != nil || bin == "" {
		bin, _ = exec.LookPath("nself")
	}
	if bin == "" {
		return deployStep{Name: "Health checks", Status: "failed"}, fmt.Errorf("unable to locate nself binary for health check")
	}
	c := exec.CommandContext(ctx, bin, "doctor")
	c.Dir = workdir
	c.Env = os.Environ()
	out, err := c.CombinedOutput()
	if err != nil {
		output := strings.TrimSpace(string(out))
		// Try to extract the failing service from doctor output.
		failedSvc := "unknown"
		for _, line := range strings.Split(output, "\n") {
			if strings.Contains(line, "unhealthy") || strings.Contains(line, "failed") {
				parts := strings.Fields(line)
				if len(parts) > 0 {
					failedSvc = parts[0]
					break
				}
			}
		}
		return deployStep{Name: "Health checks", Status: "failed"},
			fmt.Errorf("health check failed (service: %s). Run 'nself doctor --verbose' for details", failedSvc)
	}
	if !jsonOut {
		fmt.Println("  [done] Health checks passed")
	}
	return deployStep{Name: "Health checks", Status: "done"}, nil
}

// applyLocalHasuraMetadataAfterDeploy applies hasura/metadata/ (if present)
// against the Hasura instance the rolling restart just brought up on THIS
// host — used by the "local" target and the "no host configured" staging/prod
// fallback (both cases run entirely on the current machine, so a plain
// config.Load + local API call is correct; the remote-host case is handled
// separately, over SSH, at the end of remoteDeployPush in deploy_remote.go).
func applyLocalHasuraMetadataAfterDeploy(ctx context.Context, workdir string, jsonOut bool) error {
	cfg, err := config.Load(workdir)
	if err != nil {
		return fmt.Errorf("hasura metadata apply: loading config: %w", err)
	}
	if !jsonOut {
		fmt.Println("  [running] hasura metadata apply")
	}
	return hasura.ApplyIfPresent(ctx, cfg, workdir)
}
