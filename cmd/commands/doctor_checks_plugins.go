package commands

// Purpose: doctor checks for installed-plugin compatibility and service port
// conflicts. Inputs are the project dir and a verbose flag; outputs are
// []doctorCheckResult values.
// Constraints: split out of doctor.go (CLI-R12) as a pure move, no behavior change.

import (
	"context"
	"fmt"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/plugin"
)

// checkPluginCompatibility verifies installed plugins are compatible with the current CLI version.
func checkPluginCompatibility(projectDir string, verbose bool) []doctorCheckResult {
	pluginDir := resolvePluginDir()
	plugins, err := plugin.ListInstalled(pluginDir)
	if err != nil {
		name := "Plugin compatibility"
		msg := fmt.Sprintf("cannot list plugins: %v", err)
		printCheck("warn", name, msg, verbose)
		return []doctorCheckResult{{Name: name, Status: "warn", Message: msg}}
	}

	if len(plugins) == 0 {
		name := "Plugin compatibility"
		msg := "no plugins installed"
		printCheck("pass", name, msg, verbose)
		return []doctorCheckResult{{Name: name, Status: "pass", Message: msg}}
	}

	var results []doctorCheckResult
	for _, p := range plugins {
		name := fmt.Sprintf("Plugin: %s", p.Name)
		msg := fmt.Sprintf("v%s (%s)", p.Version, p.Status)
		if p.Status == "error" || p.Status == "failed" {
			printCheck("fail", name, msg, verbose)
			results = append(results, doctorCheckResult{Name: name, Status: "fail", Message: msg})
		} else {
			printCheck("pass", name, msg, verbose)
			results = append(results, doctorCheckResult{Name: name, Status: "pass", Message: msg})
		}
	}
	return results
}

// checkServicePortConflicts probes configured service ports against enabled
// services. It catches conflicts between nSelf services (Grafana on 3000,
// Admin on 3021, etc.) and local dev servers that may already be listening.
//
// Ports held by this project's own running containers are excluded, via the
// same ownership filter `nself start` uses. Without it, a healthy stack's own
// Admin container made doctor report "port 3021 (nSelf Admin) is already in
// use by another process" — pointing at itself.
func checkServicePortConflicts(ctx context.Context, projectDir string, verbose bool) []doctorCheckResult {
	cfg, err := config.Load(projectDir)
	if err != nil {
		name := "Service port conflicts"
		msg := fmt.Sprintf("cannot load config: %v", err)
		printCheck("warn", name, msg, verbose)
		return []doctorCheckResult{{Name: name, Status: "warn", Message: msg}}
	}

	type servicePort struct {
		name string
		port int
	}
	var ports []servicePort

	if cfg.Monitoring.GrafanaEnabled {
		ports = append(ports, servicePort{"Grafana", cfg.Monitoring.GrafanaPort})
	}
	if cfg.Admin.Enabled {
		ports = append(ports, servicePort{"nSelf Admin", cfg.Admin.Port})
	}
	if cfg.Mailpit.Enabled {
		ports = append(ports, servicePort{"Mailpit UI", cfg.Mailpit.UIPort})
	}
	if cfg.Functions.Enabled {
		ports = append(ports, servicePort{"Functions", cfg.Functions.Port})
	}
	if cfg.MLflow.Enabled {
		ports = append(ports, servicePort{"MLflow", cfg.MLflow.Port})
	}

	if len(ports) == 0 {
		name := "Service port conflicts"
		msg := "no services with dev-port conflict risk enabled"
		printCheck("pass", name, msg, verbose)
		return []doctorCheckResult{{Name: name, Status: "pass", Message: msg}}
	}

	// Resolve which of these ports this project already owns, so its own
	// containers are not reported as foreign processes.
	portNums := make([]int, 0, len(ports))
	for _, sp := range ports {
		if sp.port != 0 {
			portNums = append(portNums, sp.port)
		}
	}

	envFiles, composeFiles, hasProject := projectPortInputs(projectDir)
	var conflicts []docker.PortConflict
	var probeErr error
	if hasProject {
		conflicts, probeErr = docker.CheckAllPortsFiltered(ctx, portNums, projectDir, envFiles, composeFiles...)
	} else {
		// Nothing built here, so nothing of ours can hold these ports.
		conflicts, probeErr = docker.CheckPortsUnowned(portNums)
	}
	if probeErr != nil {
		// One honest "could not check" beats a conflict per port that we
		// cannot attribute to a foreign process.
		name := "Service port conflicts"
		msg := fmt.Sprintf("cannot check service ports: %v", probeErr)
		printCheck("warn", name, msg, verbose)
		return []doctorCheckResult{{Name: name, Status: "warn", Message: msg}}
	}

	conflicting := make(map[int]bool, len(conflicts))
	for _, c := range conflicts {
		conflicting[c.Port] = true
	}

	var results []doctorCheckResult
	for _, sp := range ports {
		if sp.port == 0 {
			continue
		}
		name := fmt.Sprintf("Port %d (%s)", sp.port, sp.name)
		if conflicting[sp.port] {
			msg := fmt.Sprintf("Warning: port %d (%s) is already in use by another process", sp.port, sp.name)
			printCheck("warn", name, msg, verbose)
			results = append(results, doctorCheckResult{Name: name, Status: "warn", Message: msg})
			continue
		}
		msg := fmt.Sprintf("port %d (%s) is free or held by this project", sp.port, sp.name)
		printCheck("pass", name, msg, verbose)
		results = append(results, doctorCheckResult{Name: name, Status: "pass", Message: msg})
	}
	return results
}
