// Package stack executes the shared upgrade plan with a durable recovery journal.
package stack

import (
	"context"
	"time"

	"github.com/nself-org/cli/internal/upgrade/plan"
)

// Options selects the project and authorizes mutations.
// Inputs: caller-resolved environment and confirmation callback.
// Outputs: execution policy. Constraints: dry runs never call the backend.
type Options struct {
	ProjectDir string
	Prod       bool
	Yes        bool
	DryRun     bool
	Confirm    func(string) bool
}

// Backend connects ordered plan execution to existing project operations.
// Inputs: context and durable journal. Outputs: operation errors and backup proof.
// Constraints: operations must be idempotent; never delete backups or volumes.
type Backend interface {
	Backup(context.Context, *Journal) error
	VerifyBackup(context.Context, *Journal) error
	Run(context.Context, string, *Journal) error
	CheckPreviousImages(context.Context, *Journal) error
	StopServices(context.Context, *Journal) error
	Restore(context.Context, *Journal) error
	RestartPrevious(context.Context, *Journal) error
	CountRows(context.Context) (map[string]int64, error)
}

// Step records one execution result for human and JSON output.
// Inputs: plan name and measured execution. Outputs: resumable step state.
// Constraints: names and order are checked when loading a journal.
type Step struct {
	Name       string `json:"name"`
	Result     string `json:"result"`
	DurationMS int64  `json:"duration_ms"`
}

// Journal preserves every fact needed to resume or recover a stack update.
// Inputs: completed operation evidence. Outputs: atomic project state.
// Constraints: production classification cannot change during resume.
type Journal struct {
	RunID                 string           `json:"run_id"`
	StartedAt             time.Time        `json:"started_at"`
	Status                string           `json:"status"`
	Prod                  bool             `json:"prod"`
	BackupID              string           `json:"backup_id,omitempty"`
	BackupSHA256          string           `json:"backup_sha256,omitempty"`
	BackupCreatedAt       time.Time        `json:"backup_created_at,omitempty"`
	BackupVerifiedAt      time.Time        `json:"backup_verified_at,omitempty"`
	PreviousCompose       string           `json:"previous_compose,omitempty"`
	PreviousComposeSHA256 string           `json:"previous_compose_sha256,omitempty"`
	BeforeRows            map[string]int64 `json:"before_rows,omitempty"`
	AfterRows             map[string]int64 `json:"after_rows,omitempty"`
	Steps                 []Step           `json:"steps"`
}

// ExecutionPlan adapts the shared plan to stack execution after pins are updated.
// Inputs: shared DefaultSteps. Outputs: backup, pull, build, start and checks.
// Constraints: plugin and CLI pin application remain owned by update --yes.
func ExecutionPlan() []plan.Step {
	var out []plan.Step
	for _, s := range plan.DefaultSteps(nil) {
		switch s.Name {
		case plan.StepImages:
			s = plan.Step{Name: "pull", Kind: plan.StepKindStack, Pins: []plan.Pin{}, Command: "pull locked images"}
		case plan.StepPlugins, plan.StepCLI:
			continue
		}
		out = append(out, s)
	}
	return out
}
