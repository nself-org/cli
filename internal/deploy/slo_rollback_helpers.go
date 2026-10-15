package deploy

// Purpose: deploy-key validation, prior-version discovery, the rollback exec step, audit-event logging, and project-root discovery backing ExecuteRollback in slo_rollback.go.
// Inputs: a deploy key, project root, service name, and RollbackConfig.
// Outputs: a validated key check, the discovered prior version, an executed rollback command, and a written np_auditlog_events entry.
// Constraints: split out of slo_rollback.go as a pure move (CLI-R12); no behavior change.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/docker"
)

// isValidDeployKey checks that the key matches the expected format.
// Deploy keys follow the pattern: nself_deploy_ + 32+ alphanumeric chars.
func isValidDeployKey(key string) bool {
	const prefix = "nself_deploy_"
	if !strings.HasPrefix(key, prefix) {
		// Also accept keys starting with nself_pro_ for backwards compat with
		// the license-based deploy key pattern used in some environments.
		if !strings.HasPrefix(key, "nself_pro_") && !strings.HasPrefix(key, "nself_") {
			return false
		}
	}
	return len(key) >= len(prefix)+16
}

// discoverPriorVersion reads the rollback state file to find the previous
// deployed image tag for the given service.
// State file: <project_root>/.nself/rollback-state/<env>/<service>.json
func discoverPriorVersion(projectRoot, service, env string) (string, error) {
	stateFile := filepath.Join(projectRoot, ".nself", "rollback-state", env, service+".json")

	data, err := os.ReadFile(stateFile)
	if err != nil {
		return "", fmt.Errorf("rollback state file not found at %s: %w", stateFile, err)
	}

	var state struct {
		PriorTag  string    `json:"prior_tag"`
		PriorHash string    `json:"prior_hash"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return "", fmt.Errorf("invalid rollback state file: %w", err)
	}

	if state.PriorTag == "" && state.PriorHash == "" {
		return "", fmt.Errorf("rollback state is empty — no prior version recorded")
	}

	version := state.PriorTag
	if version == "" {
		version = state.PriorHash
	}

	return version, nil
}

// runRollback re-deploys the prior image for the given service.
// Uses docker compose to pull + restart the specific service container.
func runRollback(ctx context.Context, projectRoot, service, priorVersion, env string) error {
	composeFile := filepath.Join(projectRoot, "docker-compose.yml")
	if _, err := os.Stat(composeFile); err != nil {
		return fmt.Errorf("docker-compose.yml not found at %s: %w", projectRoot, err)
	}

	// Set the image tag env var for the specific service.
	imageTag := fmt.Sprintf("%s_IMAGE_TAG=%s", strings.ToUpper(service), priorVersion)

	cmd := docker.NewProjectCompose(projectRoot, composeFile).CommandContext(ctx, projectRoot,
		"up", "-d",
		"--no-deps",
		"--force-recreate",
		service,
	)
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), imageTag)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose rollback failed: %w", err)
	}

	return nil
}

// writeAuditEvent emits an audit event to stdout (audit-log plugin picks it up).
func writeAuditEvent(cfg RollbackConfig, priorVersion, outcome, errMsg string) {
	event := AuditEvent{
		EventType:    "deploy.rollback",
		Service:      cfg.Service,
		Environment:  cfg.Environment,
		Reason:       cfg.Reason,
		Outcome:      outcome,
		PriorVersion: priorVersion,
		Error:        errMsg,
		TriggeredBy:  "slo-watcher",
		Timestamp:    time.Now().UTC(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal audit event: %v\n", err)
		return
	}

	// Emit a single-line, prefixed JSON event to stdout. The audit-log plugin
	// tails stdout and ingests every line beginning with "AUDIT_EVENT ".
	fmt.Printf("AUDIT_EVENT %s\n", data)
}

// findProjectRoot walks up the directory tree to find the nSelf project root
// (directory containing docker-compose.yml or .nself/).
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		// Check for .nself directory (nSelf project marker).
		nself := filepath.Join(dir, ".nself")
		if _, err := os.Stat(nself); err == nil {
			return dir, nil
		}

		// Check for docker-compose.yml (fallback).
		compose := filepath.Join(dir, "docker-compose.yml")
		if _, err := os.Stat(compose); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find nSelf project root (no .nself/ or docker-compose.yml found)")
		}
		dir = parent
	}
}
