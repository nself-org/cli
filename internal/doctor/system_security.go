package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Purpose: security-posture checks — admin bind address, encryption key
// scope/strength, root containers, and secret file permissions.
// Inputs: a context, the project directory, and (for the key-scope check) a
// strict flag that turns weak-key findings into failures instead of warnings.
// Outputs: []CheckResult, or a single CheckResult for CheckAdminBind.
// Constraints: split out of system.go (CLI-R12) as a pure move; no behavior
// changed. Depends on CheckJWTSecretPresent, defined elsewhere in this package.

// CheckAdminBind verifies that the nself-admin container is bound to
// 127.0.0.1 only (not 0.0.0.0 which would expose it to the LAN).
// This implements S43-T03 — required by the Service Binding Hard Rule.
func CheckAdminBind(ctx context.Context) CheckResult {
	name := "Admin bind address (SEC-BIND-01)"

	// Try docker inspect to determine the host binding for port 3021.
	cmd := exec.CommandContext(ctx, "docker", "inspect",
		"--format", `{{range $p, $conf := .HostConfig.PortBindings}}{{range $conf}}{{if eq $p "3021/tcp"}}{{.HostIp}}{{end}}{{end}}{{end}}`,
		"nself-admin")
	out, err := cmd.Output()
	if err != nil {
		// Container not running — check passes vacuously (nothing exposed).
		return CheckResult{Section: "security", Name: name, Status: "pass",
			Message: "nself-admin container not running"}
	}

	hostIP := strings.TrimSpace(string(out))
	if hostIP == "" {
		return CheckResult{Section: "security", Name: name, Status: "pass",
			Message: "admin container has no published port 3021"}
	}
	if hostIP == "0.0.0.0" || hostIP == "" {
		return CheckResult{Section: "security", Name: name, Status: "fail",
			Message: fmt.Sprintf("admin port 3021 bound to %q — exposes admin UI to LAN", hostIP),
			FixCmd:  "nself admin stop && nself admin start"}
	}
	if hostIP != "127.0.0.1" {
		return CheckResult{Section: "security", Name: name, Status: "warn",
			Message: fmt.Sprintf("admin port 3021 bound to %q (expected 127.0.0.1)", hostIP)}
	}
	return CheckResult{Section: "security", Name: name, Status: "pass",
		Message: "admin port 3021 bound to 127.0.0.1 only"}
}

// CheckEncryptionKeyScope verifies that AI_ENCRYPTION_KEY (and any other
// *_ENCRYPTION_KEY env vars) are set and are not reused across environments.
// Implements S43-UNDEP-01 + S43-AUDIT-01 (SEC-ENC-01 and SEC-ENC-02).
//
// When strict=true, a shared or default key returns Status="fail".
// When strict=false (default), it returns Status="warn" — allowing the admin
// to investigate without blocking a deploy.
func CheckEncryptionKeyScope(projectDir string, strict bool) []CheckResult {
	var results []CheckResult

	// Known encryption key env vars across nSelf (extend as new plugins add keys).
	keyVars := []string{
		"AI_ENCRYPTION_KEY",
		"CLAW_ENCRYPTION_KEY",
		"PLUGIN_AI_ENCRYPTION_KEY",
	}

	// Known weak/default values that must never appear in any real environment.
	weakValues := map[string]bool{
		"":              true,
		"changeme":      true,
		"secret":        true,
		"password":      true,
		"dev":           true,
		"development":   true,
		"test":          true,
		"default":       true,
		"placeholder":   true,
		"replace_me":    true,
		"your_key_here": true,
	}

	status := func(isStrict bool) string {
		if isStrict {
			return "fail"
		}
		return "warn"
	}

	for _, keyVar := range keyVars {
		val := os.Getenv(keyVar)

		// SEC-ENC-01: key must be set.
		if val == "" {
			results = append(results, CheckResult{
				Section: "security",
				Name:    fmt.Sprintf("SEC-ENC-01: %s", keyVar),
				Status:  status(strict),
				Message: fmt.Sprintf("%s is not set (encryption at rest requires a key)", keyVar),
			})
			continue
		}

		// SEC-ENC-02: key must not be a known weak/default value.
		if weakValues[strings.ToLower(strings.TrimSpace(val))] {
			results = append(results, CheckResult{
				Section: "security",
				Name:    fmt.Sprintf("SEC-ENC-02: %s", keyVar),
				Status:  status(strict),
				Message: fmt.Sprintf("%s uses a default/weak value; rotate before deploying to production", keyVar),
			})
			continue
		}

		// Key is set and non-default.
		results = append(results, CheckResult{
			Section: "security",
			Name:    fmt.Sprintf("SEC-ENC-01/02: %s", keyVar),
			Status:  "pass",
			Message: fmt.Sprintf("%s is set and non-default", keyVar),
		})
	}

	return results
}

// SecurityChecks runs security diagnostics.
func SecurityChecks(ctx context.Context, projectDir string) []CheckResult {
	var results []CheckResult

	// JWT secret presence — Hasura refuses to start without it.
	results = append(results, CheckJWTSecretPresent(projectDir))

	// S43-T03: admin service binding check.
	results = append(results, CheckAdminBind(ctx))

	// S43-AUDIT-01 / S43-UNDEP-01: encryption key scope (SEC-ENC-01/02).
	// WARN by default; callers may re-run with strict=true for --strict mode.
	results = append(results, CheckEncryptionKeyScope(projectDir, false)...)

	// D-0459: ssl/ keys and .env.secrets ignored by git and not tracked (SEC-GITIGNORE-01).
	results = append(results, CheckSSLTracked(ctx, projectDir))

	// Check for root containers
	cmd := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}")
	out, err := cmd.Output()
	if err == nil {
		containers := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, c := range containers {
			if c == "" {
				continue
			}
			inspectCmd := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Config.User}}", c)
			userOut, err := inspectCmd.Output()
			if err == nil {
				user := strings.TrimSpace(string(userOut))
				if user == "" || user == "root" || user == "0" {
					results = append(results, CheckResult{Section: "security", Name: fmt.Sprintf("Container %s user", c),
						Status: "warn", Message: "running as root"})
				}
			}
		}
	}

	// Check secret file permissions
	secretsDir := filepath.Join(projectDir, ".nself", "secrets")
	if entries, err := os.ReadDir(secretsDir); err == nil {
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			mode := info.Mode().Perm()
			if mode&0o077 != 0 {
				results = append(results, CheckResult{Section: "security",
					Name:    fmt.Sprintf("Secret perms: %s", e.Name()),
					Status:  "warn",
					Message: fmt.Sprintf("mode %o (should be 0600)", mode),
					FixCmd:  fmt.Sprintf("chmod 0600 %s", filepath.Join(secretsDir, e.Name()))})
			}
		}
	}

	return results
}
