package promote

// Purpose: Keep an unsynced attempt separate from the live recovery authority.
// Inputs: Project-local attempt and deploy identities. Outputs: Atomic attempt
// state or eligible interrupted recovery. Constraints: Failed empty attempts
// never hide a previous live record; uncertain writes refuse with the backup id.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
)

const AttemptFile = ".nself/state/promote-attempt.json"

// SaveAttempt persists provisional evidence without replacing promote.json.
func SaveAttempt(root string, rec Record) error {
	rec.SchemaVersion = 1
	file := filepath.Join(root, AttemptFile)
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(file+".tmp", raw, 0600); err != nil {
		return err
	}
	return os.Rename(file+".tmp", file)
}

// LoadAttempt reads isolated attempt history, including interrupted apply.
func LoadAttempt(root string) (*Record, error) {
	raw, err := os.ReadFile(filepath.Join(root, AttemptFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func loadRecoveryAttempt(root, target string, live *Record) (*Record, error) {
	attempt, err := LoadAttempt(root)
	if err != nil {
		return nil, errs.Wrap("E485", "unusable promotion attempt; nothing was changed", err)
	}
	if attempt == nil || attempt.Target != target || !attempt.ApplyStarted || (attempt.Status != "in_progress" && attempt.Status != "rollback_failed") {
		attempt = live
	} else if live != nil && !attempt.StartedAt.After(live.StartedAt) {
		attempt = live
	}
	if attempt == nil || !attempt.ApplyStarted || (attempt.Status != "in_progress" && attempt.Status != "rollback_failed") {
		return live, nil
	}
	if len(attempt.PreDeployTar) > 0 {
		return attempt, nil
	}
	state, err := deploy.LoadDeployState(root)
	if err != nil {
		return nil, err
	}
	if state.Target == target && attempt.DeployIdentity != nil && !attempt.DeployIdentity.StartedAt.Equal(state.StartedAt) {
		return live, nil
	}
	if state.Target != target || attempt.DeployIdentity == nil || !attempt.DeployIdentity.StartedAt.Equal(state.StartedAt) || len(state.PreDeployTars) == 0 {
		return nil, errs.Newf("E485", "interrupted apply has no host recovery evidence; backup %s needs operator; nothing was changed", attempt.BackupID).
			WithWhy("The interrupted promotion has no recorded host recovery tar.").
			WithFix(fmt.Sprintf("Verify the interrupted apply on the target and restore backup %s by hand if needed; preserve the attempt record until recovery is verified.", attempt.BackupID))
	}
	attempt.PreDeployTar, attempt.Changed, attempt.Absent = state.PreDeployTars, state.PreDeployChanged, state.PreDeployAbsent
	attempt.DeployIdentity = &DeployIdentity{StartedAt: state.StartedAt, PreDeployTar: state.PreDeployTar, PreDeployTars: state.PreDeployTars}
	return attempt, nil
}
