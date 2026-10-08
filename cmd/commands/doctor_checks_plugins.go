package commands

// Purpose: doctor checks for installed-plugin compatibility and service port
// conflicts. Inputs are the project dir and a verbose flag; outputs are
// []doctorCheckResult values.
// Constraints: split out of doctor.go (CLI-R12) as a pure move, no behavior change.

import (
	"fmt"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/plugin"
)

// checkPluginCompatibility verifies installed plugins are compatible with the current CLI version.
// The plugin-command mount check (doctor_checks_mount.go) is prepended so it
// reports even when the compatibility scan exits early.
func checkPluginCompatibility(projectDir string, verbose bool) []doctorCheckResult {
	return append(checkPluginMount(RootCmd, verbose), checkPluginVersionCompat(projectDir, verbose)...)
}

// checkPluginVersionCompat is the version-compatibility scan of every
// installed plugin.
func checkPluginVersionCompat(projectDir string, verbose bool) []doctorCheckResult {
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

// checkServicePortConflicts probes configured service ports against enabled services.
// It catches conflicts between nSelf services (Grafana on 3000, Admin on 3021, etc.)
// and local dev servers that may already be listening.
func checkServicePortConflicts(projectDir string, verbose bool) []doctorCheckResult {
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

	var results []doctorCheckResult
	for _, sp := range ports {
		if sp.port == 0 {
			continue
		}
		name := fmt.Sprintf("Port %d (%s)", sp.port, sp.name)
		inUse, err := docker.CheckPort(sp.port)
		if err != nil {
			printCheck("warn", name, fmt.Sprintf("cannot check port: %v", err), verbose)
			results = append(results, doctorCheckResult{Name: name, Status: "warn", Message: fmt.Sprintf("cannot check port: %v", err)})
			continue
		}
		if inUse {
			msg := fmt.Sprintf("Warning: port %d (%s) is already in use by another process", sp.port, sp.name)
			printCheck("warn", name, msg, verbose)
			results = append(results, doctorCheckResult{Name: name, Status: "warn", Message: msg})
		} else {
			msg := fmt.Sprintf("port %d (%s) is available", sp.port, sp.name)
			printCheck("pass", name, msg, verbose)
			results = append(results, doctorCheckResult{Name: name, Status: "pass", Message: msg})
		}
	}
	return results
}
