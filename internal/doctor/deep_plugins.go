package doctor

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Purpose: per-plugin health probing for --deep mode — HTTP /health and
// /healthz probes with a docker-healthcheck fallback, plus the top-level
// PluginHealthChecks entry point that enumerates running plugin containers.
// Inputs: a context, project dir, and verbose flag (or, for the HTTP probe
// helper, an http.Client, plugin name, and port).
// Outputs: []CheckResult, or a single CheckResult from the HTTP probe helper.
// Constraints: split out of deep.go (CLI-R12) as a pure move; no behavior
// changed. Depends on verifyDockerRunning/resolveHostPort in deep_docker.go.

// probePluginHTTP attempts GET /health and, on 404, GET /healthz against
// http://127.0.0.1:<port>. Returns the CheckResult for this plugin.
func probePluginHTTP(client *http.Client, pluginName string, port int) CheckResult {
	checkID := fmt.Sprintf("PLUGIN-HEALTH-%s", pluginName)
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	for _, path := range []string{"/health", "/healthz"} {
		resp, err := client.Get(base + path)
		if err != nil {
			// connection refused or timeout
			return CheckResult{
				Section: "plugins",
				Name:    checkID,
				Status:  "fail",
				Message: fmt.Sprintf("plugin %s: not running (%v)", pluginName, err),
				FixCmd:  fmt.Sprintf("nself start --plugin %s", pluginName),
			}
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return CheckResult{
				Section: "plugins",
				Name:    checkID,
				Status:  "pass",
				Message: fmt.Sprintf("plugin %s: healthy (HTTP 200 %s)", pluginName, path),
			}
		}
		if resp.StatusCode == http.StatusNotFound && path == "/health" {
			// Try /healthz next iteration.
			continue
		}
		return CheckResult{
			Section: "plugins",
			Name:    checkID,
			Status:  "fail",
			Message: fmt.Sprintf("plugin %s: HTTP %d on %s", pluginName, resp.StatusCode, path),
			FixCmd:  fmt.Sprintf("docker restart %s", pluginName),
		}
	}

	return CheckResult{
		Section: "plugins",
		Name:    checkID,
		Status:  "warn",
		Message: fmt.Sprintf("plugin %s: /health and /healthz both returned 404", pluginName),
	}
}

// PluginHealthChecks verifies every installed plugin's health endpoint, then
// appends the checks registered through RegisterExternal (P7-CI-53: the
// installed plugins' own doctors). The external checks run even when the
// container probe returns early (no Docker, no containers): a CLI plugin has
// no container.
func PluginHealthChecks(ctx context.Context, projectDir string, verbose bool) []CheckResult {
	return append(pluginContainerChecks(ctx, projectDir, verbose), ExternalChecks(ctx, projectDir)...)
}

// pluginContainerChecks probes every running plugin container.
func pluginContainerChecks(ctx context.Context, projectDir string, verbose bool) []CheckResult {
	var results []CheckResult

	// Step 1: verify Docker daemon is reachable.
	if err := verifyDockerRunning(ctx); err != nil {
		fixCmd := "sudo systemctl start docker"
		if runtime.GOOS == "darwin" {
			fixCmd = "open -a Docker"
		}
		results = append(results, CheckResult{
			Section: "plugins",
			Name:    "PLUGIN-HEALTH-docker",
			Status:  "fail",
			Message: fmt.Sprintf("Docker not running: %v", err),
			FixCmd:  fixCmd,
		})
		return results
	}

	// Step 2: list running nself plugin containers.
	cmd := exec.CommandContext(ctx, "docker", "ps", "--filter", "label=nself.plugin", "--format", "{{.Names}}\t{{.Ports}}")
	out, err := cmd.Output()
	running := strings.TrimSpace(string(out))

	if err != nil || running == "" {
		// Step 3: distinguish installed-but-stopped from not-installed.
		listCmd := exec.CommandContext(ctx, "nself", "plugin", "list", "--installed", "--json")
		listOut, listErr := listCmd.Output()
		expectedCount := 0
		if listErr == nil && strings.TrimSpace(string(listOut)) != "" && strings.TrimSpace(string(listOut)) != "[]" {
			// Count newlines as a rough plugin count; each JSON line = one plugin.
			for _, ln := range strings.Split(strings.TrimSpace(string(listOut)), "\n") {
				if strings.TrimSpace(ln) != "" && strings.TrimSpace(ln) != "[" && strings.TrimSpace(ln) != "]" {
					expectedCount++
				}
			}
		}
		if expectedCount > 0 {
			results = append(results, CheckResult{
				Section: "plugins",
				Name:    "PLUGIN-HEALTH-containers",
				Status:  "fail",
				Message: fmt.Sprintf("%d plugin(s) installed but no containers running", expectedCount),
				FixCmd:  "nself start",
			})
		} else {
			results = append(results, CheckResult{
				Section: "plugins",
				Name:    "PLUGIN-HEALTH-containers",
				Status:  "pass",
				Message: "no plugins installed",
			})
		}
		return results
	}

	// Step 4: probe each running plugin container.
	client := &http.Client{Timeout: 5 * time.Second}
	for _, line := range strings.Split(running, "\n") {
		parts := strings.SplitN(line, "\t", 2)
		containerName := strings.TrimSpace(parts[0])
		if containerName == "" {
			continue
		}

		// Strip "nself_" prefix to derive the plugin name for the check ID.
		pluginName := strings.TrimPrefix(containerName, "nself_")

		hostPort, portErr := resolveHostPort(ctx, containerName)
		if portErr != nil || hostPort == "" || hostPort == "0" {
			// No host port exposed: emit warn, fall through to docker healthcheck.
			results = append(results, CheckResult{
				Section: "plugins",
				Name:    fmt.Sprintf("PLUGIN-HEALTH-%s", pluginName),
				Status:  "warn",
				Message: fmt.Sprintf("plugin %s: cannot probe health (no host port exposed)", pluginName),
			})
			// Still fall through to docker healthcheck below.
		} else {
			portNum, convErr := strconv.Atoi(hostPort)
			if convErr != nil || portNum < 1024 || portNum > 65535 {
				results = append(results, CheckResult{
					Section: "plugins",
					Name:    fmt.Sprintf("PLUGIN-HEALTH-%s", pluginName),
					Status:  "warn",
					Message: fmt.Sprintf("plugin %s: invalid host port %s", pluginName, hostPort),
				})
			} else {
				results = append(results, probePluginHTTP(client, pluginName, portNum))
				continue // HTTP probe result is authoritative; skip docker healthcheck.
			}
		}

		// Fallback: docker healthcheck status (when no valid host port).
		hcCmd := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Health.Status}}", containerName)
		hcOut, hcErr := hcCmd.Output()
		if hcErr != nil {
			results = append(results, CheckResult{
				Section: "plugins",
				Name:    fmt.Sprintf("PLUGIN-HEALTH-%s", pluginName),
				Status:  "warn",
				Message: fmt.Sprintf("plugin %s: cannot inspect docker health", pluginName),
			})
			continue
		}
		hStatus := strings.TrimSpace(string(hcOut))
		// Deliberately NOT health.HealthResult.OK(). hStatus here is the RAW
		// docker health field, where "" means "no healthcheck configured", not
		// the resolved status OK() operates on. Same words, different vocabulary.
		if hStatus == "healthy" || hStatus == "" {
			results = append(results, CheckResult{
				Section: "plugins",
				Name:    fmt.Sprintf("PLUGIN-HEALTH-%s", pluginName),
				Status:  "pass",
				Message: fmt.Sprintf("plugin %s: healthy (docker healthcheck)", pluginName),
			})
		} else {
			results = append(results, CheckResult{
				Section: "plugins",
				Name:    fmt.Sprintf("PLUGIN-HEALTH-%s", pluginName),
				Status:  "fail",
				Message: fmt.Sprintf("plugin %s: docker health status: %s", pluginName, hStatus),
				FixCmd:  fmt.Sprintf("docker restart %s", containerName),
			})
		}
	}

	return results
}
