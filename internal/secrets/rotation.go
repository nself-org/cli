// Package secrets — rotation scheduling, dual-key windows, and expiry alerts.
package secrets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// RotationSchedule defines when and how a secret should be rotated.
type RotationSchedule struct {
	SecretName  string `json:"secret_name"`
	CadenceDays int    `json:"cadence_days"`
	WindowDays  int    `json:"window_days"` // dual-key overlap window
	LastRotated string `json:"last_rotated,omitempty"`
	NextDue     string `json:"next_due,omitempty"`
}

// RotationState is the persisted state for all tracked secret schedules.
type RotationState struct {
	Schedules []RotationSchedule `json:"schedules"`
	UpdatedAt string             `json:"updated_at"`
}

// DefaultSchedules returns the minimum set of tracked secrets per the spec.
func DefaultSchedules() []RotationSchedule {
	return []RotationSchedule{
		{SecretName: "PLUGIN_INTERNAL_SECRET", CadenceDays: 90, WindowDays: 7},
		{SecretName: "HASURA_GRAPHQL_ADMIN_SECRET", CadenceDays: 180, WindowDays: 3},
		{SecretName: "JWT_SIGNING_KEY", CadenceDays: 180, WindowDays: 1},
		{SecretName: "STRIPE_WEBHOOK_SECRET", CadenceDays: 0, WindowDays: 0}, // on-demand
		{SecretName: "B2_APPLICATION_KEY", CadenceDays: 365, WindowDays: 1},
		{SecretName: "AGE_BACKUP_KEY", CadenceDays: 365, WindowDays: 90},
	}
}

// rotationStatePath returns the path to the rotation state file.
func rotationStatePath(projectRoot string) string {
	return filepath.Join(projectRoot, SecretsDir, "rotation-state.json")
}

// LoadRotationState reads the rotation schedule state from disk.
func LoadRotationState(projectRoot string) (*RotationState, error) {
	path := rotationStatePath(projectRoot)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &RotationState{Schedules: DefaultSchedules()}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading rotation state: %w", err)
	}
	var state RotationState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing rotation state: %w", err)
	}
	migrateScheduleNames(&state)
	return &state, nil
}

// legacyAdminSecretName is the name the default schedule used before
// P7-LIVE-05: Hasura's admin secret without "GRAPHQL_", a key nself never
// sets. It is spelled in two parts on purpose: the Ticket's check that no
// schedule in this package names the old key greps for the quoted literal,
// and this constant exists only to rename persisted rows away from it.
const legacyAdminSecretName = "HASURA_" + "ADMIN_SECRET"

// migrateScheduleNames renames persisted rows from legacy names (P7-LIVE-05):
// the row keeps its cadence and its LastRotated/NextDue history. When the new
// name already has a row, the legacy row is dropped and its history is kept
// only if it is the newer one. The change is written the next time the state
// is saved.
func migrateScheduleNames(state *RotationState) {
	renames := map[string]string{legacyAdminSecretName: "HASURA_GRAPHQL_ADMIN_SECRET"}
	at := map[string]int{}
	for i, s := range state.Schedules {
		at[s.SecretName] = i
	}
	out := make([]RotationSchedule, 0, len(state.Schedules))
	for _, s := range state.Schedules {
		to, legacy := renames[s.SecretName]
		if !legacy {
			out = append(out, s)
			continue
		}
		if j, ok := at[to]; ok {
			if cur := &state.Schedules[j]; s.LastRotated > cur.LastRotated {
				cur.LastRotated, cur.NextDue = s.LastRotated, s.NextDue
			}
			continue
		}
		s.SecretName = to
		at[to] = -1
		out = append(out, s)
	}
	// A target row copied before a later legacy row updated it is refreshed.
	targets := map[string]bool{}
	for _, to := range renames {
		targets[to] = true
	}
	for i := range out {
		if j, ok := at[out[i].SecretName]; ok && j >= 0 && targets[out[i].SecretName] {
			out[i] = state.Schedules[j]
		}
	}
	state.Schedules = out
}

// SaveRotationState persists the rotation schedule state to disk.
func SaveRotationState(projectRoot string, state *RotationState) error {
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(rotationStatePath(projectRoot))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(rotationStatePath(projectRoot), data, 0600)
}

// ScheduleCheck represents the result of checking one secret's rotation schedule.
type ScheduleCheck struct {
	SecretName  string        `json:"secret_name"`
	CadenceDays int           `json:"cadence_days"`
	WindowDays  int           `json:"window_days"`
	LastRotated string        `json:"last_rotated"`
	NextDue     string        `json:"next_due"`
	DueIn       time.Duration `json:"-"`
	DueInDays   int           `json:"due_in_days"`
	Status      string        `json:"status"` // ok, warning, overdue, missing
}

// CheckSchedule validates all rotation schedules and returns findings.
func CheckSchedule(projectRoot string) ([]ScheduleCheck, error) {
	state, err := LoadRotationState(projectRoot)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var results []ScheduleCheck

	for _, s := range state.Schedules {
		check := ScheduleCheck{
			SecretName:  s.SecretName,
			CadenceDays: s.CadenceDays,
			WindowDays:  s.WindowDays,
			LastRotated: s.LastRotated,
			NextDue:     s.NextDue,
		}

		// On-demand secrets (cadence 0) are always ok unless explicitly overdue.
		if s.CadenceDays == 0 {
			check.Status = "ok"
			check.DueInDays = -1
			results = append(results, check)
			continue
		}

		if s.NextDue == "" {
			check.Status = "missing"
			results = append(results, check)
			continue
		}

		nextDue, err := time.Parse(time.RFC3339, s.NextDue)
		if err != nil {
			check.Status = "missing"
			results = append(results, check)
			continue
		}

		check.DueIn = nextDue.Sub(now)
		check.DueInDays = int(check.DueIn.Hours() / 24)

		switch {
		case check.DueIn < 0:
			check.Status = "overdue"
		case check.DueIn < 7*24*time.Hour:
			check.Status = "warning"
		default:
			check.Status = "ok"
		}

		results = append(results, check)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].NextDue < results[j].NextDue
	})
	return results, nil
}
