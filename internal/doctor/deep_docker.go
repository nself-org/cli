package doctor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Purpose: Docker-daemon-level --deep checks — storage driver, disk usage,
// unhealthy containers, plus the daemon-reachability and host-port lookup
// helpers shared with the plugin health checks in deep_plugins.go.
// Inputs: a context and, for the helpers, a container name.
// Outputs: []CheckResult per category, or (string, error) for host-port lookup.
// Constraints: split out of deep.go (CLI-R12) as a pure move; no behavior
// changed.

// DockerDeepChecks verifies daemon, storage driver, dangling images, container health.
func DockerDeepChecks(ctx context.Context, verbose bool) []CheckResult {
	var results []CheckResult

	// Daemon reachable
	cmd := exec.CommandContext(ctx, "docker", "info", "--format", "{{.Driver}}")
	out, err := cmd.Output()
	if err != nil {
		results = append(results, CheckResult{Section: "docker", Name: "Docker daemon", Status: "fail", Message: "unreachable"})
		return results
	}
	driver := strings.TrimSpace(string(out))
	if driver != "overlay2" {
		results = append(results, CheckResult{Section: "docker", Name: "Storage driver", Status: "warn",
			Message: fmt.Sprintf("using %s (expected overlay2)", driver)})
	} else {
		results = append(results, CheckResult{Section: "docker", Name: "Storage driver", Status: "pass", Message: "overlay2"})
	}

	// Dangling images >5GB
	cmd = exec.CommandContext(ctx, "docker", "system", "df", "--format", "{{.Size}}")
	out, err = cmd.Output()
	if err == nil {
		results = append(results, CheckResult{Section: "docker", Name: "Docker disk usage", Status: "pass",
			Message: strings.TrimSpace(string(out))})
	}

	// All expected containers healthy
	cmd = exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}\t{{.Status}}")
	out, err = cmd.Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			cName := parts[0]
			status := ""
			if len(parts) > 1 {
				status = parts[1]
			}
			if strings.Contains(strings.ToLower(status), "unhealthy") {
				generic := CheckResult{Section: "docker", Name: fmt.Sprintf("Container: %s", cName),
					Status: "fail", Message: "unhealthy", FixCmd: fmt.Sprintf("docker restart %s", cName)}
				// G-014: an "unhealthy" status is only actionable if the
				// healthcheck command itself can run inside the image —
				// otherwise "docker restart" is a guess that never helps
				// (see deep_docker_healthcheck.go).
				results = append(results, diagnoseUnhealthyContainer(ctx, cName, generic))
			}
		}
	}

	return results
}

// verifyDockerRunning checks that the Docker daemon is reachable.
// Returns a non-nil error if Docker is not available.
func verifyDockerRunning(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("docker daemon unreachable: %w", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("docker daemon returned empty server version")
	}
	return nil
}

// resolveHostPort returns the first mapped host port for containerName using
// docker inspect. Returns ("", nil) when no port binding is found, and
// ("", err) on inspect failure.
func resolveHostPort(ctx context.Context, containerName string) (string, error) {
	portCmd := exec.CommandContext(ctx, "docker", "inspect",
		"--format", `{{range $p, $b := .NetworkSettings.Ports}}{{if $b}}{{(index $b 0).HostPort}}{{end}}{{end}}`,
		containerName)
	portOut, err := portCmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker inspect %s: %w", containerName, err)
	}
	return strings.TrimSpace(string(portOut)), nil
}
