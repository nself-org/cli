// Package deploy provides deployment management for the nSelf CLI.
// This file manages the unified deploy state file at .nself/state/deploy.json.
// It tracks which strategy was last used, its outcome, and provides the
// rollback target for `nself deploy rollback`.
package deploy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// DeployStateFile is the path relative to project root where deploy state is persisted.
// All deploy strategies (rolling, blue-green, canary) write to this file.
const DeployStateFile = ".nself/state/deploy.json"

// Strategy is the deploy strategy used.
type Strategy string

const (
	StrategyRolling   Strategy = "rolling"
	StrategyBlueGreen Strategy = "blue-green"
	StrategyCanary    Strategy = "canary"
)

// StrategyState records the outcome of the last deploy for a given strategy.
type StrategyState struct {
	// SchemaVersion is the version of the StrategyState schema.
	// Set to 1 on all writes. Readers treat 0 (missing field) as the legacy default.
	SchemaVersion int `json:"schema_version"`

	// Strategy is the deploy strategy used.
	Strategy Strategy `json:"strategy"`

	// Target is the deploy target: local, staging, prod.
	Target string `json:"target"`

	// StartedAt is the timestamp when the deploy started.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is the timestamp when the deploy completed (success or failure).
	CompletedAt time.Time `json:"completed_at"`

	// Success reports whether the deploy succeeded.
	Success bool `json:"success"`

	// CanaryPercent is the final canary traffic split at completion (0 or 100 for non-canary).
	CanaryPercent int `json:"canary_percent"`

	// RolledBack is true when the deploy triggered an automatic rollback.
	RolledBack bool `json:"rolled_back"`

	// Error is the failure reason when Success is false.
	Error string `json:"error,omitempty"`

	// BlueGreenStateFile is the path to the underlying blue/green state file,
	// populated when strategy is blue-green or canary.
	BlueGreenStateFile string `json:"blue_green_state_file,omitempty"`

	// BackupID is the verified database backup taken before a v1.5 remote apply
	// on a production-class target; BackupHost is the host it lives on, and
	// PreDeployTar the in-place file backup (every tier). `deploy rollback`
	// restores from them. Empty on every other deploy.
	BackupID     string `json:"backup_id,omitempty"`
	BackupHost   string `json:"backup_host,omitempty"`
	PreDeployTar string `json:"pre_deploy_tar,omitempty"`
	// PreDeployTars maps every host the deploy synced to its pre-deploy tar, so
	// `deploy rollback` restores each server of the env, not just the primary.
	PreDeployTars map[string]string `json:"pre_deploy_tars,omitempty"`
	// PreDeployChanged and PreDeployAbsent are, per host, the relative paths the
	// deploy shipped or pruned and the subset the host did not have before. A
	// rollback restores and removes exactly these, never anything read from the host.
	PreDeployChanged map[string][]string `json:"pre_deploy_changed,omitempty"`
	PreDeployAbsent  map[string][]string `json:"pre_deploy_absent,omitempty"`
	// FrontTars, FrontChanged and FrontAbsent are the same per host for the
	// fronting stack's root of a fronted project (D-0425): the front tar and the
	// nginx/sites paths shipped or pruned there. Absent on every other deploy, so
	// a record without them rolls back as before.
	FrontTars    map[string]string   `json:"front_tars,omitempty"`
	FrontChanged map[string][]string `json:"front_changed,omitempty"`
	FrontAbsent  map[string][]string `json:"front_absent,omitempty"`
	// FrontShipped is, per host, the front files that deploy shipped (not the pruned
	// ones): the only paths the next deploy may prune there. Without it nothing is.
	FrontShipped map[string][]string `json:"front_shipped,omitempty"`
	// FrontNames is, per host, the server_name set of each shipped front file: the
	// next deploy replaces or prunes a recorded path only while the host file still has it.
	FrontNames map[string]map[string][]string `json:"front_names,omitempty"`
	// DBRestoredAt is set (RFC 3339) right after `nself backup restore` succeeded,
	// before the re-apply, so a retried rollback never restores the database twice.
	DBRestoredAt string `json:"db_restored_at,omitempty"`
	// HostOutcomes is, per server name, what the deploy did there: "ok", "failed:
	// <err>" (changes kept) or "not changed: <err>" (refused or restored). Success
	// is true only when every server of the env is "ok".
	HostOutcomes map[string]string `json:"host_outcomes,omitempty"`
}

// LoadDeployState reads the deploy state from .nself/state/deploy.json.
// Returns a zero-value StrategyState and nil when no state file exists (fresh
// install). Any other read failure and a corrupt file are returned as errors,
// never as an empty state (D-0144).
func LoadDeployState(projectRoot string) (StrategyState, error) {
	path := filepath.Join(projectRoot, DeployStateFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return StrategyState{}, nil
	}
	if err != nil {
		return StrategyState{}, fmt.Errorf("reading deploy state: %w", err)
	}
	var s StrategyState
	if err := json.Unmarshal(data, &s); err != nil {
		return StrategyState{}, fmt.Errorf("parsing deploy state: %w", err)
	}
	return s, nil
}

// SaveDeployState writes the deploy state to .nself/state/deploy.json.
// The file is written atomically via a temp file + rename to avoid partial reads.
// Permissions are 0600 because deploy state contains hostnames and error context.
func SaveDeployState(projectRoot string, s StrategyState) error {
	// Always stamp the schema version on write.
	s.SchemaVersion = 1

	path := filepath.Join(projectRoot, DeployStateFile)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating deploy state directory: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling deploy state: %w", err)
	}

	// Write to a sibling temp file, then rename atomically.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("writing deploy state (temp): %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup
		return fmt.Errorf("committing deploy state: %w", err)
	}
	return nil
}
