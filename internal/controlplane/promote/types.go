// Package promote runs an environment promotion as an injected transaction.
package promote

import "github.com/nself-org/cli/internal/controlplane"

// State is the mutable data shared by promotion steps.
//
// Purpose: carry resolved promotion inputs and rollback evidence between steps.
// Inputs: command wiring initializes environment and tier fields; steps fill evidence.
// Outputs: Engine.Run copies public outcome fields into Result.
// Constraints: Values is opaque wiring state; this package does not interpret it.
type State struct {
	Source           string
	Target           string
	SourceTier       controlplane.Tier
	TargetTier       controlplane.Tier
	SnapshotTag      string
	BackupID         string
	BackupHost       string
	MigrateStarted   bool
	Digests          []DigestRow
	PreDeployTar     map[string]string
	PreDeployChanged map[string][]string
	PreDeployAbsent  map[string][]string
	Values           map[string]any
	DryRun           bool
}

// DigestRow records the per-platform release identity comparison.
type DigestRow struct {
	Service string `json:"service"`
	Source  string `json:"source"`
	Target  string `json:"target"`
	Status  string `json:"status"`
}

// StepResult is one deterministic plan or execution result row.
type StepResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Result is the complete promotion outcome returned to human and JSON callers.
type Result struct {
	Status      string       `json:"status"`
	Source      string       `json:"source"`
	Target      string       `json:"target"`
	Steps       []StepResult `json:"steps"`
	Digests     []DigestRow  `json:"digests,omitempty"`
	SnapshotTag string       `json:"snapshot_tag,omitempty"`
	BackupID    string       `json:"backup_id,omitempty"`
	FailedStep  string       `json:"failed_step,omitempty"`
	DryRun      bool         `json:"dry_run,omitempty"`
}
