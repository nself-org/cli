package backup

// stream_resume.go — resumable-stream checkpoint state.
//
// Purpose: persist and reload ResumeState so an interrupted Stream run can be resumed instead of restarted, split out of stream.go for file size.
// Inputs: a state directory and the resume key identifying a stream run.
// Outputs: ResumeState read from or written to disk; Resume drives the actual restart.
// Constraints: pure move from stream.go (CLI-R12 Batch E); no behaviour change.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// ResumeState holds the persisted state for a resumable streaming backup.
type ResumeState struct {
	BackupID    string    `json:"backup_id"`
	Destination string    `json:"destination"`
	Key         string    `json:"key"`
	Recipients  []string  `json:"recipients"`
	StartedAt   time.Time `json:"started_at"`
	// For rclone-based multipart, we store the partial object key prefix.
	// rclone handles actual part tracking internally; we record enough to
	// invoke rclone copyto from a local temp file on resume.
	PartialPath string `json:"partial_path,omitempty"`
}

// stateDir returns the directory for resume state files.
func stateDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nself", "backup-state")
}

// saveResumeState persists state to ~/.nself/backup-state/<id>.json.
func saveResumeState(state ResumeState) error {
	dir := stateDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	path := filepath.Join(dir, state.BackupID+".json")
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

// loadResumeState reads ~/.nself/backup-state/<id>.json.
func loadResumeState(backupID string) (*ResumeState, error) {
	path := filepath.Join(stateDir(), backupID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no resume state found for backup ID %q", backupID)
		}
		return nil, fmt.Errorf("read state: %w", err)
	}
	var state ResumeState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &state, nil
}

// deleteResumeState removes the state file on successful completion.
func deleteResumeState(backupID string) {
	path := filepath.Join(stateDir(), backupID+".json")
	_ = os.Remove(path)
}

// listResumeStates returns all IDs with saved resume state.
func listResumeStates() ([]ResumeState, error) {
	dir := stateDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP. No state dir means no interrupted transfer to resume,
			// which is the ordinary case. ENOENT only — an unreadable state
			// dir errors rather than silently restarting a resumable upload
			// from byte zero.
			return nil, nil
		}
		return nil, fmt.Errorf("read state dir: %w", err)
	}
	var states []ResumeState
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		state, err := loadResumeState(id)
		if err != nil {
			continue
		}
		states = append(states, *state)
	}
	return states, nil
}

// ResumeOptions holds flags for `nself backup resume`.
type ResumeOptions struct {
	BackupID string
}

// Resume attempts to complete a previously interrupted streaming backup.
// Since rclone rcat cannot resume a partial upload, Resume re-streams from
// pg_dump through the full pipeline. The previous partial upload is
// overwritten at the same destination key.
func Resume(ctx context.Context, cfg *config.Config, opts ResumeOptions) (*StreamResult, error) {
	state, err := loadResumeState(opts.BackupID)
	if err != nil {
		return nil, err
	}

	slog.Info("resuming backup",
		"backup_id", state.BackupID,
		"destination", state.Destination,
		"started_at", state.StartedAt,
	)

	pgURL := buildPgURL(cfg)
	result, err := runStreamPipeline(ctx, cfg, pgURL, state.Destination, state.Key, state.Recipients)
	if err != nil {
		return nil, fmt.Errorf("resume stream: %w", err)
	}

	deleteResumeState(opts.BackupID)
	return result, nil
}
