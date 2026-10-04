package bluegreen

// bluegreen_traffic.go — Nginx traffic-shifting helpers.
//
// Purpose: set Nginx upstream weights, reload Nginx, soak the new weighting and measure the resulting error rate, used by Deploy and Promote in bluegreen.go, split out for file size.
// Inputs: the desired blue/green weight split and a soak duration.
// Outputs: an applied Nginx config plus the measured error rate for the soak window.
// Constraints: the upstream conf is written to the served nginx's conf.d
// (P7-LIVE-02, D-0045); for a project that runs its own nginx that is the
// directory it always was.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/internal/nginxtopo"
)

// setNginxWeights writes the Nginx upstream block with the given canary percent
// and reloads Nginx atomically (nginx -s reload).
func setNginxWeights(cfg DeployConfig, canaryPercent int) error {
	servedRoot, err := writeUpstreamConf(cfg, canaryPercent)
	if err != nil {
		return err
	}

	// Reload nginx (atomic — no dropped connections). The nginx container
	// belongs to the served stack, which is the fronting stack's compose
	// project for a fronted project.
	// Try docker exec into the nginx container first; fall back to system nginx.
	if err := reloadNginx(servedRoot); err != nil {
		return fmt.Errorf("nginx reload failed: %w", err)
	}

	return nil
}

// writeUpstreamConf writes the blue/green upstream conf into the SERVED nginx
// conf.d directory and returns the served stack root (D-0045).
//
// A project with NGINX_FRONTED_BY set has no nginx of its own, so a conf
// written under its own nginx/conf.d is never read: the weights would shift
// on disk and nowhere else. internal/nginxtopo decides where the running nginx
// reads from; everything else stays where it was.
func writeUpstreamConf(cfg DeployConfig, canaryPercent int) (servedRoot string, err error) {
	frontedBy := frontedByFor(cfg.ProjectRoot)
	servedRoot, err = nginxtopo.ServedRoot(cfg.ProjectRoot, frontedBy)
	if err != nil {
		return "", fmt.Errorf("locating the served nginx stack: %w", err)
	}
	nginxDir, err := nginxtopo.ServedNginxDir(cfg.ProjectRoot, frontedBy)
	if err != nil {
		return "", fmt.Errorf("locating the served nginx stack: %w", err)
	}

	upstream := GenerateNginxUpstream(cfg, canaryPercent)

	// Write to the generated upstream conf file.
	upstreamPath := filepath.Join(nginxDir, "conf.d", "bluegreen-upstream.conf")
	if err := os.MkdirAll(filepath.Dir(upstreamPath), 0755); err != nil {
		return "", fmt.Errorf("creating nginx conf.d dir: %w", err)
	}

	if err := os.WriteFile(upstreamPath, []byte(upstream), 0644); err != nil {
		return "", fmt.Errorf("writing bluegreen upstream config: %w", err)
	}
	return servedRoot, nil
}

// frontedByFor returns the project's NGINX_FRONTED_BY ("" when it runs its own
// nginx). DeployConfig carries no such field, so it is read the way the config
// loader would resolve it, without loading the whole config or touching the
// process environment: the env cascade files in precedence order, a file value
// winning over the process environment exactly as config.Load's Overload does.
func frontedByFor(projectRoot string) string {
	value := os.Getenv("NGINX_FRONTED_BY")
	env, _ := config.ResolveEnv(projectRoot)
	for _, name := range config.EnvCascadeOrder(env, config.LegacyOrderActive()) {
		m, err := godotenv.Read(filepath.Join(projectRoot, name))
		if err != nil {
			continue
		}
		if v, ok := m["NGINX_FRONTED_BY"]; ok {
			value = v
		}
	}
	return strings.TrimSpace(value)
}

// reloadNginx sends a reload signal to the nginx container or the system nginx.
func reloadNginx(projectRoot string) error {
	// Try docker compose exec nginx nginx -s reload.
	cmd := exec.Command("docker", "compose", "exec", "-T", "nginx", "nginx", "-s", "reload")
	cmd.Dir = projectRoot
	cmd.Env = os.Environ()
	if err := cmd.Run(); err == nil {
		return nil
	}

	// Fall back to system nginx reload.
	cmd = exec.Command("nginx", "-s", "reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("both docker compose exec nginx reload and system nginx reload failed: %w", err)
	}
	return nil
}

// soak monitors the green environment during the soak period.
// Returns (rolledBack=true, err) when error rate exceeds threshold, or when
// the green environment cannot be observed at all (see measureErrorRate) —
// an unobservable environment must never be promoted to production.
func soak(ctx context.Context, cfg DeployConfig, duration time.Duration) (bool, error) {
	deadline := time.Now().Add(duration)
	pollInterval := 30 * time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(pollInterval):
		}

		errorRate, err := measureErrorRate(ctx, cfg)
		if err != nil {
			return true, fmt.Errorf("soak measurement failed, rolling back: %w", err)
		}
		if errorRate > cfg.ErrorThresholdPct {
			return true, fmt.Errorf("error rate %.2f%% exceeded threshold %.1f%%", errorRate, cfg.ErrorThresholdPct)
		}
	}
	return false, nil
}

// measureErrorRate estimates the current error rate from green containers.
// It checks docker compose ps for containers that are not running, or that
// declare a failing healthcheck, and returns the resulting percentage. When
// Prometheus is available, it queries the metrics endpoint instead.
//
// It returns an error when the green environment cannot be observed at all:
// the docker command failing, or reporting zero containers. Both used to
// return 0.0 ("no errors"), which made an unobservable environment
// indistinguishable from a healthy one. soak() now treats any error from
// this function as a soak failure and rolls back, so "couldn't measure" and
// "measured and it's broken" both stop the promotion — the only difference
// is which is reported.
func measureErrorRate(ctx context.Context, cfg DeployConfig) (float64, error) {
	project := composeProjectName(EnvGreen)
	out, err := exec.CommandContext(ctx,
		"docker", "compose",
		"-p", project,
		"ps", "--format", containerStatusFormat,
	).Output()
	if err != nil {
		return 0.0, fmt.Errorf("querying green container status for project %s: %w", project, err)
	}

	errorRate, total := parseContainerStatusOutput(string(out))
	if total == 0 {
		return 0.0, fmt.Errorf("no containers reported for green project %s: cannot measure error rate", project)
	}
	return errorRate, nil
}

// containerStatus is one parsed row of docker compose ps output: the
// State (running, exited, restarting, ...) and the optional Health value
// (healthy, unhealthy, starting, or empty when no healthcheck is declared).
type containerStatus struct {
	state  string
	health string
}

// containerStatusFormat is the shared docker compose ps --format used by
// both measureErrorRate (soak gate) and waitForHealth (readiness wait), so
// the two call sites read identical fields and can share one parser and
// one accept-predicate (containerHealthy) instead of drifting apart the
// way #270 (fail-open on Health alone) and its readiness-side counterpart
// (fail-closed on the same missing Health) did.
const containerStatusFormat = "{{.Name}}\t{{.State}}\t{{.Health}}"

// parseContainerStatusLines parses tab-separated
// "{{.Name}}\t{{.State}}\t{{.Health}}" lines from `docker compose ps` into
// one containerStatus per non-blank line. Isolated from its callers so it
// can be unit tested without shelling out to Docker.
func parseContainerStatusLines(out string) []containerStatus {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	statuses := make([]containerStatus, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		state := ""
		dockerHealth := ""
		if len(fields) >= 2 {
			state = strings.TrimSpace(fields[1])
		}
		if len(fields) >= 3 {
			dockerHealth = strings.TrimSpace(fields[2])
		}
		statuses = append(statuses, containerStatus{state: state, health: dockerHealth})
	}
	return statuses
}

// parseContainerStatusOutput returns the error rate percentage together
// with the number of containers counted (the denominator).
//
// A container counts as healthy only when containerHealthy (below) accepts
// it. Every parsed line is counted in total regardless of whether it has a
// Health value — the original bug excluded healthcheck-less services (e.g.
// nginx, auth) from the denominator entirely because it looked at Health
// alone, which meant a crash-looping nginx was invisible to this gate.
func parseContainerStatusOutput(out string) (errorRatePct float64, total int) {
	statuses := parseContainerStatusLines(out)
	total = len(statuses)
	if total == 0 {
		return 0.0, 0
	}

	unhealthy := 0
	for _, cs := range statuses {
		if !containerHealthy(cs.state, cs.health) {
			unhealthy++
		}
	}
	return float64(unhealthy) / float64(total) * 100.0, total
}

// allContainersReady reports whether every container in out is ready
// (containerHealthy accepts it) together with the number of containers
// observed. Used by waitForHealth. total == 0 is never ready: no
// containers reported means the environment could not be observed, which
// must never be indistinguishable from "everything is healthy" (same
// principle as the total == 0 case in parseContainerStatusOutput).
func allContainersReady(out string) (ready bool, total int) {
	statuses := parseContainerStatusLines(out)
	total = len(statuses)
	if total == 0 {
		return false, 0
	}

	for _, cs := range statuses {
		if !containerHealthy(cs.state, cs.health) {
			return false, total
		}
	}
	return true, total
}

// containerHealthy applies the same accept-predicate internal/health uses
// for HealthResult.OK(): a container with no Docker healthcheck reports an
// empty Health field and is judged on State alone ("running" counts as OK,
// matching buildComposeHealthMap's "no healthcheck configured" convention
// in internal/health/checker.go). A container whose State is not "running"
// (exited, restarting, dead, ...) is never OK regardless of Health — that
// is precisely the failure {{.Health}}-only sampling could not see. A
// "starting" Health value (healthcheck running but not yet passed) is also
// never OK, since it is neither "healthy" nor "running". waitForHealth
// relies on this to keep waiting rather than declaring readiness prematurely.
func containerHealthy(state, dockerHealth string) bool {
	status := dockerHealth
	if status == "" {
		status = state
	}
	return health.HealthResult{Status: status}.OK()
}
