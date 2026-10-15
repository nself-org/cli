package imageswitch

// state.go — .nself/image-switch/state.json, written atomically after every step.
//
// Purpose: a killed switch can be resumed or rolled back, and the invocation
// guard (E512) can tell that one is incomplete.
// Inputs: the project directory. Outputs: State values and the stage path.
// Constraints: the state holds names and phases only, never a password, a
// password hash or a row; the directory is 0700 and the file 0600.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// stateVersion is bumped when the state layout changes incompatibly.
const stateVersion = 1

// State is the persisted progress of one switch.
type State struct {
	Version int    `json:"version"`
	RunID   string `json:"run_id"`
	Phase   Phase  `json:"phase"`

	Container      string   `json:"container"`
	Network        string   `json:"network"`
	ComposeProject string   `json:"compose_project"`
	BootUser       string   `json:"boot_user"`
	Major          int      `json:"major"`
	OldImage       string   `json:"old_image"`
	NewImage       string   `json:"new_image"`
	OldVolume      string   `json:"old_volume"`
	NewVolume      string   `json:"new_volume"`
	Databases      []string `json:"databases"`
	// ReadOnlySet is true once ALTER DATABASE ... default_transaction_read_only
	// was set on the old cluster by this switch.
	ReadOnlySet bool `json:"read_only_set"`
	// StoppedServices are the services this switch stopped; abort and rollback restart them.
	StoppedServices []string `json:"stopped_services"`
	// NewVolumeCreated is true once this switch created the new volume (it may remove it).
	NewVolumeCreated bool `json:"new_volume_created"`
	// PrevEnv holds previous .env values; missing keys must be removed on rollback.
	// Legacy states contain both keys, including explicit empty values.
	PrevEnv   map[string]string `json:"prev_env"`
	Tolerated []string          `json:"tolerated,omitempty"`
	Tables    int               `json:"tables,omitempty"`
	Sequences int               `json:"sequences,omitempty"`
	DataBytes int64             `json:"data_bytes,omitempty"`

	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// dir is the switch's directory inside the project.
func dir(projectDir string) string { return filepath.Join(projectDir, ".nself", "image-switch") }

// StatePath is the state file of the project.
func StatePath(projectDir string) string { return filepath.Join(dir(projectDir), "state.json") }

// stagePath is the 0700 directory holding the dump and the captures.
func stagePath(projectDir string) string { return filepath.Join(dir(projectDir), "stage") }

// LoadState reads the state; (nil, nil) means there is none.
func LoadState(projectDir string) (*State, error) {
	data, err := os.ReadFile(StatePath(projectDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the image switch state: %w", err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("the image switch state %s is unreadable: %w (if no switch is in progress, check `docker volume ls` for a *_pgvector_* volume and remove the file)", StatePath(projectDir), err)
	}
	if st.Version != stateVersion {
		return nil, fmt.Errorf("the image switch state %s has version %d, this nself reads %d", StatePath(projectDir), st.Version, stateVersion)
	}
	return &st, nil
}

// Incomplete returns the state when a switch started in the project and has
// not finished. It is cheap: one stat when there is none. An unreadable state
// counts as incomplete (the guard must fail closed) and is returned as a
// State with an empty phase.
func Incomplete(projectDir string) (*State, bool) {
	if _, err := os.Stat(StatePath(projectDir)); err != nil {
		return nil, false
	}
	st, err := LoadState(projectDir)
	if err != nil {
		return &State{Phase: ""}, true
	}
	if st == nil || st.Phase.Complete() {
		return st, false
	}
	return st, true
}

// save writes the state atomically (temp file, fsync, rename) at 0600.
func (st *State) save(projectDir string, now time.Time) error {
	st.Version = stateVersion
	st.UpdatedAt = now.UTC()
	if err := os.MkdirAll(dir(projectDir), 0o700); err != nil {
		return fmt.Errorf("creating the image switch directory: %w", err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir(projectDir), ".state-*")
	if err != nil {
		return fmt.Errorf("writing the image switch state: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), StatePath(projectDir))
}

// removeState deletes the state file (an aborted switch leaves none).
func removeState(projectDir string) error {
	err := os.Remove(StatePath(projectDir))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
