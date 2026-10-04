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
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/doctor"
	"github.com/nself-org/cli/internal/ports"
)

// Test seams for the doctor port check. Production values are the real
// probes; doctor_checks_config_test.go swaps them for a fake port owner.
var (
	// doctorPortsFiltered is the ownership-aware probe `nself start` uses.
	doctorPortsFiltered = docker.CheckAllPortsFiltered
	// doctorPortsAll is the plain probe, the fallback when ownership is unknown.
	doctorPortsAll = docker.CheckAllPorts
	// doctorPortHolder names the process holding a port for the message.
	doctorPortHolder = ports.WhoHoldsPort
	// doctorPortProject resolves the project directory the stack lives in.
	doctorPortProject = doctorPortProjectDir
)

// doctorPortOwnerTimeout bounds the `docker compose ps` ownership query.
const doctorPortOwnerTimeout = 30 * time.Second

// doctorPortProjectDir returns the project directory for the ownership query:
// the working directory, or its monorepo backend directory, the same
// resolution `doctor` applies to its own projectDir (doctor.go).
func doctorPortProjectDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolving project directory: %w", err)
	}
	if backendRoot := config.DetectMonorepoRoot(cwd); backendRoot != "" {
		return backendRoot, nil
	}
	return cwd, nil
}

// probeDoctorPorts probes docker.ReservedPorts and returns the conflicts.
//
// It asks the ownership-aware check `start` uses, so a port held by this
// project's own running stack is not a conflict while an unrelated holder
// still is. When ownership cannot be established (ownerErr != nil) it falls
// back to the unfiltered probe, which is the result doctor printed before
// D-0063, and reports ownerErr so the caller can say so. err is set only when
// the probe itself fails.
func probeDoctorPorts() (conflicts []docker.PortConflict, ownerErr, err error) {
	projectDir, ownerErr := doctorPortProject()
	if ownerErr == nil {
		ctx, cancel := context.WithTimeout(context.Background(), doctorPortOwnerTimeout)
		defer cancel()
		// Same resolution as start_ports.go: compose env files, then the
		// compose manifest (a manifest read failure leaves the file list
		// empty, as in start, and compose falls back to its own discovery).
		envFiles := build.ComposeEnvFiles(projectDir)
		composeFiles, _ := build.ReadComposeManifest(projectDir)
		conflicts, ownerErr = doctorPortsFiltered(ctx, docker.ReservedPorts, projectDir, envFiles, composeFiles...)
		if ownerErr == nil {
			return conflicts, nil, nil
		}
	}
	conflicts, err = doctorPortsAll(docker.ReservedPorts)
	return conflicts, ownerErr, err
}

// checkPorts probes all reserved ports and reports conflicts. Ports owned by
// the project's own running stack are not conflicts (D-0063).
func checkPorts(verbose bool) []doctorCheckResult {
	var results []doctorCheckResult
	conflicts, ownerErr, err := probeDoctorPorts()
	if err != nil {
		name := "Port check"
		msg := fmt.Sprintf("error checking ports: %v", err)
		printCheck("warn", name, msg, verbose)
		return []doctorCheckResult{{Name: name, Status: "warn", Message: msg}}
	}

	if len(conflicts) == 0 {
		name := "Reserved ports"
		msg := fmt.Sprintf("all %d reserved ports available", len(docker.ReservedPorts))
		printCheck("pass", name, msg, verbose)
		return []doctorCheckResult{{Name: name, Status: "pass", Message: msg}}
	}

	// The conflicts below are unfiltered when ownership could not be read, so
	// they may include the project's own containers: say so once.
	if ownerErr != nil {
		name := "Port ownership"
		msg := fmt.Sprintf("could not read this project's own ports (%v); conflicts may include its containers", ownerErr)
		printCheck("warn", name, msg, verbose)
		results = append(results, doctorCheckResult{Name: name, Status: "warn", Message: msg})
	}

	// Report each conflicting port individually, with holder info when available.
	for _, c := range conflicts {
		name := fmt.Sprintf("Port %d", c.Port)
		holder, _ := doctorPortHolder(c.Port)
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
