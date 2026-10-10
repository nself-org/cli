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
// gated by kind: healthcheck declared -> healthy within 60s; restart "no"
// (one-shot) -> exited 0; otherwise running within 30s. The deploy halts on
// the first failure and names the service.
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

// rollingRestart runs plan: up each service, then gate it.
func rollingRestart(ctx context.Context, plan rollingPlan) ([]deployStep, error) {
	byName := make(map[string]docker.ServiceSpec, len(plan.specs))
	for _, sp := range plan.specs {
		byName[sp.Name] = sp
	}
	steps := []deployStep{}
	for _, svc := range plan.order {
		stepName := fmt.Sprintf("Restart %s", svc)
		if !plan.jsonOut {
			fmt.Printf("  [running] Restart %s (sequenced rolling)\n", svc)
		}
		if err := plan.compose.ComposeUpNoDeps(ctx, plan.workdir, svc); err != nil {
			steps = append(steps, deployStep{Name: stepName, Status: "failed"})
			return steps, fmt.Errorf("rolling restart: service %s restart failed: %w\nRun 'nself logs %s' for details", svc, err, svc)
		}
		spec, ok := byName[svc]
		if !ok {
			spec = docker.ServiceSpec{Name: svc}
		}
		if !plan.jsonOut {
			fmt.Printf("  [waiting] Waiting for %s (%s)\n", svc, gateKind(spec))
		}
		states := func(c context.Context) ([]docker.ContainerState, error) {
			return plan.compose.ComposeServiceStates(c, plan.workdir, svc)
		}
		if err := docker.HealthGate(ctx, spec, states, plan.clock); err != nil {
			return append(steps, deployStep{Name: stepName, Status: gateFailStatus(err)}), gateError(svc, err)
		}
		steps = append(steps, deployStep{Name: stepName, Status: "done"})
		if !plan.jsonOut {
			fmt.Printf("  [done] %s ready\n", svc)
		}
	}
	return steps, nil
}

// gateKind names what the gate waits for, for the progress line.
func gateKind(sp docker.ServiceSpec) string {
	switch {
	case sp.OneShot():
		return "one-shot: exit 0, max 60s"
	case sp.HasHealthcheck:
		return "healthy, max 60s"
	default:
		return "running, max 30s"
	}
}

// gateFailStatus is the step status for a gate error.
func gateFailStatus(err error) string {
	if errors.Is(err, docker.ErrGateTimeout) {
		return "unhealthy"
	}
	return "failed"
}

// gateError wraps a gate failure in its error code: E250 when the service is
// broken (a one-shot exited non-zero), E251 when it was too slow, E250 for
// anything else (a state read that failed is still an unhealthy answer).
func gateError(svc string, err error) error {
	code := "E250"
	if errors.Is(err, docker.ErrGateTimeout) {
		code = "E251"
	}
	return errs.Wrap(code, fmt.Sprintf("rolling restart: %v. Run 'nself logs %s' for details", err, svc), err)
}

// runDeployHealthCheck calls nself doctor and gates the deploy result.
// Returns an error with the failed service name when any service is unhealthy.
func runDeployHealthCheck(ctx context.Context, workdir string, jsonOut bool) (deployStep, error) {
	if !jsonOut {
		fmt.Println("  [running] Health checks (calling nself health)")
	}
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
