package commands

// Purpose: doctor checks for route consistency and port-range sanity across the
// project's configured services. Inputs are the project dir and a verbose flag;
// outputs are doctorCheckResult values.
// Constraints: split out of doctor.go (CLI-R12) as a pure move, no behavior change.

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

// checkRouteConsistency verifies that ROUTE values for enabled services
// do not contain uppercase letters, spaces, or leading slashes. A valid route
// is a bare subdomain label like "api" or "auth-service".
func checkRouteConsistency(projectDir string, verbose bool) []doctorCheckResult {
	cfg, err := config.Load(projectDir)
	if err != nil {
		name := "Route consistency"
		msg := fmt.Sprintf("cannot load config: %v", err)
		printCheck("warn", name, msg, verbose)
		return []doctorCheckResult{{Name: name, Status: "warn", Message: msg}}
	}

	type namedRoute struct {
		label string
		value string
	}

	// Collect all configured route values with their label.
	routes := []namedRoute{
		{"HASURA_ROUTE", cfg.Hasura.Route},
		{"AUTH_ROUTE", cfg.Auth.Route},
	}
	// D-0683: the active store decides which routes exist (SeaweedFS has no console).
	store := compose.CurrentStore(cfg)
	if store != "none" {
		routes = append(routes, namedRoute{"STORAGE_ROUTE", cfg.Minio.StorageRoute})
	}
	if store == "minio-core" || store == "minio-plugin" {
		routes = append(routes, namedRoute{"STORAGE_CONSOLE_ROUTE", cfg.Minio.ConsoleRoute})
	}
	if cfg.Admin.Enabled {
		routes = append(routes, namedRoute{"NSELF_ADMIN_ROUTE", cfg.Admin.Route})
	}
	if cfg.Search.Enabled {
		routes = append(routes, namedRoute{"SEARCH_ROUTE", cfg.Search.Route})
	}
	if cfg.Mailpit.Enabled {
		routes = append(routes, namedRoute{"MAILPIT_ROUTE", cfg.Mailpit.Route})
	}
	if cfg.Functions.Enabled {
		routes = append(routes, namedRoute{"FUNCTIONS_ROUTE", cfg.Functions.Route})
	}
	if cfg.MLflow.Enabled {
		routes = append(routes, namedRoute{"MLFLOW_ROUTE", cfg.MLflow.Route})
	}
	for _, cs := range cfg.CustomServices {
		if cs.Route != "" {
			routes = append(routes, namedRoute{fmt.Sprintf("CS_%d_ROUTE", cs.Index), cs.Route})
		}
	}
	for _, fa := range cfg.FrontendApps {
		if fa.Route != "" {
			routes = append(routes, namedRoute{fmt.Sprintf("FRONTEND_APP_%d_ROUTE", fa.Index), fa.Route})
		}
	}

	var results []doctorCheckResult
	for _, r := range routes {
		if r.value == "" {
			continue
		}
		name := fmt.Sprintf("Route: %s", r.label)
		corrected := routeCorrect(r.value)
		if corrected != r.value {
			msg := fmt.Sprintf("%q has invalid format — suggested value: %q", r.value, corrected)
			printCheck("warn", name, msg, verbose)
			results = append(results, doctorCheckResult{Name: name, Status: "warn", Message: msg})
		} else {
			msg := fmt.Sprintf("%q is valid", r.value)
			printCheck("pass", name, msg, verbose)
			results = append(results, doctorCheckResult{Name: name, Status: "pass", Message: msg})
		}
	}

	if len(results) == 0 {
		name := "Route consistency"
		msg := "no routes configured"
		printCheck("pass", name, msg, verbose)
		return []doctorCheckResult{{Name: name, Status: "pass", Message: msg}}
	}
	return results
}

// routeCorrect returns a corrected form of a ROUTE value: lowercase, no
// leading slashes, no spaces. This mirrors what RouteToFQDN would expect.
func routeCorrect(route string) string {
	corrected := strings.TrimLeft(route, "/")
	corrected = strings.TrimSpace(corrected)
	corrected = strings.ToLower(corrected)
	corrected = strings.ReplaceAll(corrected, " ", "-")
	return corrected
}

// checkPortRangeSanity warns when any configured port is below 1024
// (privileged port) when the process is not running as root.
func checkPortRangeSanity(projectDir string, verbose bool) []doctorCheckResult {
	cfg, err := config.Load(projectDir)
	if err != nil {
		name := "Port range sanity"
		msg := fmt.Sprintf("cannot load config: %v", err)
		printCheck("warn", name, msg, verbose)
		return []doctorCheckResult{{Name: name, Status: "warn", Message: msg}}
	}

	// Only warn when not running as root (uid != 0).
	isRoot := os.Getuid() == 0

	type namedPort struct {
		label string
		port  int
	}

	// Well-known ports 80 and 443 are expected privileged ports (Nginx) — skip them.
	ports := []namedPort{
		{"POSTGRES_PORT", cfg.Postgres.Port},
		{"HASURA_PORT", cfg.Hasura.Port},
		{"AUTH_PORT", cfg.Auth.Port},
	}
	if cfg.Redis.Enabled {
		ports = append(ports, namedPort{"REDIS_PORT", cfg.Redis.Port})
	}
	if store := compose.CurrentStore(cfg); store == "minio-core" || store == "minio-plugin" {
		ports = append(ports, namedPort{"MINIO_PORT", cfg.Minio.Port})
		ports = append(ports, namedPort{"MINIO_CONSOLE_PORT", cfg.Minio.ConsolePort})
	}
	if cfg.Search.Enabled {
		ports = append(ports, namedPort{"SEARCH_PORT", cfg.Search.Port})
	}
	if cfg.Functions.Enabled {
		ports = append(ports, namedPort{"FUNCTIONS_PORT", cfg.Functions.Port})
	}
	if cfg.MLflow.Enabled {
		ports = append(ports, namedPort{"MLFLOW_PORT", cfg.MLflow.Port})
	}
	if cfg.Admin.Enabled {
		ports = append(ports, namedPort{"NSELF_ADMIN_PORT", cfg.Admin.Port})
	}
	for _, cs := range cfg.CustomServices {
		if cs.Port != 0 {
			ports = append(ports, namedPort{fmt.Sprintf("CS_%d_PORT", cs.Index), cs.Port})
		}
	}

	var results []doctorCheckResult
	for _, p := range ports {
		if p.port == 0 || p.port == 80 || p.port == 443 {
			continue
		}
		name := fmt.Sprintf("Port range: %s", p.label)
		if p.port < 1024 && !isRoot {
			msg := fmt.Sprintf("port %d is privileged (<1024) and may require root to bind", p.port)
			printCheck("warn", name, msg, verbose)
			results = append(results, doctorCheckResult{Name: name, Status: "warn", Message: msg})
		} else {
			msg := fmt.Sprintf("port %d OK", p.port)
			printCheck("pass", name, msg, verbose)
			results = append(results, doctorCheckResult{Name: name, Status: "pass", Message: msg})
		}
	}

	if len(results) == 0 {
		name := "Port range sanity"
		msg := "all configured ports are in the unprivileged range"
		printCheck("pass", name, msg, verbose)
		return []doctorCheckResult{{Name: name, Status: "pass", Message: msg}}
	}
	return results
}
