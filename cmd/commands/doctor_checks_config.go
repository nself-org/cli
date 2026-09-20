package commands

// Purpose: doctor checks for project config: listening ports, .env presence,
// password strength, and JWT secret presence. Inputs are the project dir and a
// verbose/fix flag; outputs are doctorCheckResult values.
// Constraints: split out of doctor.go (CLI-R12) as a pure move, no behavior change.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/doctor"
	"github.com/nself-org/cli/internal/ports"
)

// checkPorts probes the host ports this project cares about and reports only
// the ones held by a FOREIGN process.
//
// It routes through the same resolve-and-filter step as `nself start`. Before
// that, doctor probed a fixed default list with no ownership filter, so a
// healthy stack produced six warnings naming its own running containers
// (80, 443, 5432, 8080, 4000 plus admin on 3021). Warnings that fire on a
// green run are worse than no warnings: they teach people to skip the output
// that a real conflict would appear in.
func checkPorts(ctx context.Context, projectDir string, verbose bool) []doctorCheckResult {
	envFiles, composeFiles, hasProject := projectPortInputs(projectDir)

	var (
		portList  []int
		names     map[int]string
		conflicts []docker.PortConflict
	)

	if hasProject {
		report := projectPortConflicts(ctx, projectDir, envFiles, composeFiles...)
		portList, names = report.Ports, report.ServiceNames

		if report.OwnershipUnknown() {
			// Without knowing which ports the project already binds, every one
			// of them reads as foreign. Say so once instead of emitting a
			// conflict per port that we cannot stand behind.
			name := "Port check"
			msg := fmt.Sprintf("could not determine which ports this project owns (%v); conflict check skipped", report.OwnershipErr)
			printCheck("warn", name, msg, verbose)
			return []doctorCheckResult{{Name: name, Status: "warn", Message: msg}}
		}
		conflicts = report.Conflicts
	} else {
		// Nothing has been built here, so no container of ours can hold a
		// port: the unfiltered probe of the defaults is the correct answer and
		// keeps doctor useful before `nself build`.
		portList, names = docker.ReservedPorts, docker.DefaultPortServiceNames()

		var err error
		conflicts, err = docker.CheckPortsUnowned(portList)
		if err != nil {
			name := "Port check"
			msg := fmt.Sprintf("error checking ports: %v", err)
			printCheck("warn", name, msg, verbose)
			return []doctorCheckResult{{Name: name, Status: "warn", Message: msg}}
		}
	}

	if len(conflicts) == 0 {
		name := "Reserved ports"
		msg := fmt.Sprintf("all %d project ports are free or already held by this project", len(portList))
		printCheck("pass", name, msg, verbose)
		return []doctorCheckResult{{Name: name, Status: "pass", Message: msg}}
	}

	// Report each conflicting port individually, with holder info when available.
	var results []doctorCheckResult
	for _, c := range conflicts {
		name := fmt.Sprintf("Port %d", c.Port)
		if svc := names[c.Port]; svc != "" {
			name = fmt.Sprintf("Port %d (%s)", c.Port, svc)
		}
		holder, _ := ports.WhoHoldsPort(c.Port)
		msg := ports.FormatConflictMessage(c.Port, holder)
		printCheck("warn", name, msg, verbose)
		results = append(results, doctorCheckResult{Name: name, Status: "warn", Message: msg})
	}
	return results
}

// checkEnvExists verifies that a .env file (or .env.dev) exists in the project directory.
func checkEnvExists(projectDir string, verbose bool) doctorCheckResult {
	name := ".env exists"
	envFiles := []string{".env", ".env.dev"}
	for _, f := range envFiles {
		path := filepath.Join(projectDir, f)
		if _, err := os.Stat(path); err == nil {
			msg := fmt.Sprintf("%s found", f)
			if verbose {
				printCheck("pass", name, msg, true)
			} else {
				printCheck("pass", name, msg, false)
			}
			return doctorCheckResult{Name: name, Status: "pass", Message: msg, Detail: path}
		}
	}
	printCheck("fail", name, "no .env or .env.dev found (run 'nself init')", verbose)
	return doctorCheckResult{Name: name, Status: "fail", Message: "no .env or .env.dev found (run 'nself init')"}
}

// checkPasswordStrength loads config and checks password fields for weakness.
func checkPasswordStrength(projectDir string, verbose, fix bool) []doctorCheckResult {
	var results []doctorCheckResult
	cfg, err := config.Load(projectDir)
	if err != nil {
		// Config load failed — cannot check passwords.
		name := "Password strength"
		msg := fmt.Sprintf("cannot load config: %v", err)
		printCheck("warn", name, msg, verbose)
		return []doctorCheckResult{{Name: name, Status: "warn", Message: msg}}
	}

	// Check each critical password field
	type pwField struct {
		Name   string
		Value  string
		MinLen int
	}

	fields := []pwField{
		{"POSTGRES_PASSWORD", cfg.Postgres.Password, 16},
		{"HASURA_GRAPHQL_ADMIN_SECRET", cfg.Hasura.AdminSecret, 32},
	}
	if cfg.Redis.Enabled {
		fields = append(fields, pwField{"REDIS_PASSWORD", cfg.Redis.Password, 16})
	}
	if cfg.Minio.Enabled {
		fields = append(fields, pwField{"MINIO_ROOT_PASSWORD", cfg.Minio.RootPassword, 16})
	}

	for _, f := range fields {
		name := fmt.Sprintf("Password: %s", f.Name)
		if f.Value == "" {
			printCheck("warn", name, "not set", verbose)
			results = append(results, doctorCheckResult{Name: name, Status: "warn", Message: "not set"})
			continue
		}
		if len(f.Value) < f.MinLen {
			msg := fmt.Sprintf("too short (%d chars, need %d+)", len(f.Value), f.MinLen)
			printCheck("warn", name, msg, verbose)
			results = append(results, doctorCheckResult{Name: name, Status: "warn", Message: msg})
			continue
		}
		if isWeakPassword(f.Value) {
			msg := "contains insecure pattern"
			if fix {
				msg += " (use 'nself init' to regenerate)"
			}
			printCheck("warn", name, msg, verbose)
			results = append(results, doctorCheckResult{Name: name, Status: "warn", Message: msg})
			continue
		}
		printCheck("pass", name, "strong", verbose)
		results = append(results, doctorCheckResult{Name: name, Status: "pass", Message: "strong"})
	}

	// Warn when POSTGRES_USER is the default 'postgres' value in prod/staging.
	// The default is correct for dev; in production it is a predictable attack
	// surface. We do NOT change the default — only surface a warning.
	if cfg.Postgres.User == "postgres" {
		env := cfg.Env
		if env == "prod" || env == "staging" {
			name := "Postgres default credentials"
			msg := fmt.Sprintf("POSTGRES_USER is 'postgres' (the default) in %s — set a unique username to reduce predictable-credential risk", env)
			printCheck("warn", name, msg, verbose)
			results = append(results, doctorCheckResult{Name: name, Status: "warn", Message: msg})
		}
	}

	return results
}

// isWeakPassword checks if a password contains common insecure substrings.
func isWeakPassword(value string) bool {
	insecure := []string{
		"password", "changeme", "secret", "admin",
		"12345", "qwerty", "default", "test",
		"postgres", "minioadmin", "hasura",
	}
	lower := strings.ToLower(value)
	for _, p := range insecure {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// checkJWTSecretPresent reports whether HASURA_GRAPHQL_JWT_SECRET is defined
// in the project's env files. Fails the command if absent everywhere.
// Hasura is always a core service in nSelf, so this check runs unconditionally.
func checkJWTSecretPresent(projectDir string, verbose bool) doctorCheckResult {
	r := doctor.CheckJWTSecretPresent(projectDir)
	printCheck(r.Status, r.Name, r.Message, verbose)
	return doctorCheckResult{Name: r.Name, Status: r.Status, Message: r.Message, Detail: r.FixCmd}
}
