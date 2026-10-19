package secrets

// Purpose: dual-window key rotation, retirement, rotation-log persistence, and schedule/verify commands built on the rotation state defined in rotation.go.
// Inputs: a project root, environment name, and secret key/name.
// Outputs: rotated .env.secrets entries and appended rotation-log events.
// Constraints: split out of rotation.go as a pure move (CLI-R12); no behavior change.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RotateDualWindow generates a new key while keeping the old one as _PREVIOUS.
// The current value moves to KEY_PREVIOUS, and a new value is set as KEY_CURRENT.
func RotateDualWindow(projectRoot, env, key string) error {
	store, err := loadStore(projectRoot, env)
	if err != nil {
		return err
	}

	entry, ok := store.Secrets[key]
	if !ok {
		return fmt.Errorf("secret %q not found in %s environment", key, env)
	}

	oldValue := entry.Value
	newValue, hint := generateRotationValue(key)
	if newValue == "" {
		return fmt.Errorf("secret %q requires manual rotation: %s", key, hint)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Set _PREVIOUS to old value
	store.Secrets[key+"_PREVIOUS"] = SecretEntry{
		Value:     oldValue,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Set _CURRENT to new value
	store.Secrets[key+"_CURRENT"] = SecretEntry{
		Value:     newValue,
		CreatedAt: now,
		UpdatedAt: now,
		RotatedAt: now,
	}

	// Update the base key to point to new value
	entry.Value = newValue
	entry.UpdatedAt = now
	entry.RotatedAt = now
	store.Secrets[key] = entry

	if err := saveStore(projectRoot, env, store); err != nil {
		return err
	}

	// Update rotation state
	state, err := LoadRotationState(projectRoot)
	if err != nil {
		return err
	}
	for i, s := range state.Schedules {
		if s.SecretName == key {
			state.Schedules[i].LastRotated = now
			nextDue := time.Now().UTC().AddDate(0, 0, s.CadenceDays)
			state.Schedules[i].NextDue = nextDue.Format(time.RFC3339)
			break
		}
	}
	return SaveRotationState(projectRoot, state)
}

// RetireOldKey removes the _PREVIOUS variant of a secret after the dual-key window.
func RetireOldKey(projectRoot, env, key string) error {
	store, err := loadStore(projectRoot, env)
	if err != nil {
		return err
	}

	prevKey := key + "_PREVIOUS"
	if _, ok := store.Secrets[prevKey]; !ok {
		return fmt.Errorf("no previous key %q found — nothing to retire", prevKey)
	}

	delete(store.Secrets, prevKey)

	// Also clean up _CURRENT if present (collapse back to base key)
	if current, ok := store.Secrets[key+"_CURRENT"]; ok {
		entry := store.Secrets[key]
		entry.Value = current.Value
		store.Secrets[key] = entry
		delete(store.Secrets, key+"_CURRENT")
	}

	return saveStore(projectRoot, env, store)
}

// RotationLogEntry is a single event in the rotation event log.
type RotationLogEntry struct {
	SecretName string `json:"secret_name"`
	RotatedAt  string `json:"rotated_at"`
	Status     string `json:"status"` // ok|failed|rolled_back
	Note       string `json:"note,omitempty"`
}

// RotationLog represents all recorded rotation events.
type RotationLog struct {
	Events    []RotationLogEntry `json:"events"`
	UpdatedAt string             `json:"updated_at"`
}

// rotationLogPath returns the path to the rotation event log file.
func rotationLogPath(projectRoot string) string {
	return filepath.Join(projectRoot, SecretsDir, "rotation-log.json")
}

// LoadRotationLog reads the rotation event log from disk.
func LoadRotationLog(projectRoot string) (*RotationLog, error) {
	path := rotationLogPath(projectRoot)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &RotationLog{Events: []RotationLogEntry{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading rotation log: %w", err)
	}
	var log RotationLog
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, fmt.Errorf("parsing rotation log: %w", err)
	}
	return &log, nil
}

// AppendRotationEvent adds a rotation event to the log.
func AppendRotationEvent(projectRoot, secretName, status, note string) error {
	log, err := LoadRotationLog(projectRoot)
	if err != nil {
		return err
	}
	log.Events = append(log.Events, RotationLogEntry{
		SecretName: secretName,
		RotatedAt:  time.Now().UTC().Format(time.RFC3339),
		Status:     status,
		Note:       note,
	})
	log.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(rotationLogPath(projectRoot))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(rotationLogPath(projectRoot), data, 0600)
}

// AddSchedule adds or updates a named rotation schedule entry.
func AddSchedule(projectRoot, secretName string, cadenceDays, windowDays int) error {
	state, err := LoadRotationState(projectRoot)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	entry := RotationSchedule{
		SecretName:  secretName,
		CadenceDays: cadenceDays,
		WindowDays:  windowDays,
		LastRotated: now.Format(time.RFC3339),
	}
	if cadenceDays > 0 {
		entry.NextDue = now.AddDate(0, 0, cadenceDays).Format(time.RFC3339)
	}

	found := false
	for i, s := range state.Schedules {
		if s.SecretName == secretName {
			state.Schedules[i] = entry
			found = true
			break
		}
	}
	if !found {
		state.Schedules = append(state.Schedules, entry)
	}
	return SaveRotationState(projectRoot, state)
}

// VerifySecretExists checks whether a named secret is present in the store for an environment.
// It returns nil when the secret is found; an error when it is missing or the store cannot be read.
// This provides a lightweight "verify" surface (value check without decrypting to stdout).
func VerifySecretExists(projectRoot, env, secretName string) error {
	store, err := loadStore(projectRoot, env)
	if err != nil {
		return err
	}
	if _, ok := store.Secrets[secretName]; !ok {
		return fmt.Errorf("secret %q not found in %s environment", secretName, env)
	}
	return nil
}

// InitSchedules ensures all default schedules are present in the rotation state,
// computing NextDue from LastRotated or setting to now if never rotated.
func InitSchedules(projectRoot string) error {
	state, err := LoadRotationState(projectRoot)
	if err != nil {
		return err
	}

	existing := make(map[string]bool)
	for _, s := range state.Schedules {
		existing[s.SecretName] = true
	}

	now := time.Now().UTC()
	for _, def := range DefaultSchedules() {
		if existing[def.SecretName] {
			continue
		}
		def.LastRotated = now.Format(time.RFC3339)
		if def.CadenceDays > 0 {
			def.NextDue = now.AddDate(0, 0, def.CadenceDays).Format(time.RFC3339)
		}
		state.Schedules = append(state.Schedules, def)
	}

	return SaveRotationState(projectRoot, state)
}
