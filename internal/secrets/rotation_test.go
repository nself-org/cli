package secrets

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultSchedulesContainsSixSecrets(t *testing.T) {
	schedules := DefaultSchedules()
	if len(schedules) != 6 {
		t.Fatalf("expected 6 default schedules, got %d", len(schedules))
	}
	names := map[string]bool{
		"PLUGIN_INTERNAL_SECRET":      false,
		"HASURA_GRAPHQL_ADMIN_SECRET": false,
		"JWT_SIGNING_KEY":             false,
		"STRIPE_WEBHOOK_SECRET":       false,
		"B2_APPLICATION_KEY":          false,
		"AGE_BACKUP_KEY":              false,
	}
	for _, s := range schedules {
		if _, ok := names[s.SecretName]; !ok {
			t.Errorf("unexpected schedule: %s", s.SecretName)
		}
		names[s.SecretName] = true
	}
	for name, found := range names {
		if !found {
			t.Errorf("missing schedule: %s", name)
		}
	}
}

func TestLoadRotationStateDefault(t *testing.T) {
	dir := t.TempDir()
	state, err := LoadRotationState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Schedules) != 6 {
		t.Fatalf("expected 6 default schedules, got %d", len(state.Schedules))
	}
}

func TestSaveAndLoadRotationState(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, SecretsDir), 0700)

	state := &RotationState{
		Schedules: []RotationSchedule{
			{SecretName: "TEST_SECRET", CadenceDays: 30, WindowDays: 3,
				LastRotated: time.Now().UTC().Format(time.RFC3339),
				NextDue:     time.Now().UTC().AddDate(0, 0, 30).Format(time.RFC3339)},
		},
	}
	if err := SaveRotationState(dir, state); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadRotationState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Schedules) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(loaded.Schedules))
	}
	if loaded.Schedules[0].SecretName != "TEST_SECRET" {
		t.Errorf("expected TEST_SECRET, got %s", loaded.Schedules[0].SecretName)
	}
}

func TestCheckScheduleOverdue(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, SecretsDir), 0700)

	pastDue := time.Now().UTC().AddDate(0, 0, -10).Format(time.RFC3339)
	state := &RotationState{
		Schedules: []RotationSchedule{
			{SecretName: "OVERDUE_SECRET", CadenceDays: 90, WindowDays: 7,
				LastRotated: pastDue, NextDue: pastDue},
		},
	}
	if err := SaveRotationState(dir, state); err != nil {
		t.Fatal(err)
	}

	checks, err := CheckSchedule(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Status != "overdue" {
		t.Errorf("expected overdue, got %s", checks[0].Status)
	}
}

func TestCheckScheduleWarning(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, SecretsDir), 0700)

	// Due in 3 days — should trigger warning (<7d)
	nearDue := time.Now().UTC().AddDate(0, 0, 3).Format(time.RFC3339)
	state := &RotationState{
		Schedules: []RotationSchedule{
			{SecretName: "WARN_SECRET", CadenceDays: 90, WindowDays: 7,
				LastRotated: time.Now().UTC().Format(time.RFC3339), NextDue: nearDue},
		},
	}
	if err := SaveRotationState(dir, state); err != nil {
		t.Fatal(err)
	}

	checks, err := CheckSchedule(dir)
	if err != nil {
		t.Fatal(err)
	}
	if checks[0].Status != "warning" {
		t.Errorf("expected warning, got %s", checks[0].Status)
	}
}

func TestRetireOldKeyMissing(t *testing.T) {
	dir := t.TempDir()
	err := RetireOldKey(dir, "dev", "NONEXISTENT")
	if err == nil {
		t.Error("expected error retiring nonexistent key")
	}
}

// TestDefaultSchedulesTrackHasuraGraphQLAdminSecret pins the P7-LIVE-05 fix:
// the schedule tracks the key the stack reads (HASURA_GRAPHQL_ADMIN_SECRET),
// with the cadence and window the old, never-set name had.
func TestDefaultSchedulesTrackHasuraGraphQLAdminSecret(t *testing.T) {
	for _, s := range DefaultSchedules() {
		if s.SecretName == "HASURA_GRAPHQL_ADMIN_SECRET" {
			if s.CadenceDays != 180 || s.WindowDays != 3 {
				t.Fatalf("HASURA_GRAPHQL_ADMIN_SECRET schedule = %d/%d days, want 180/3", s.CadenceDays, s.WindowDays)
			}
			return
		}
	}
	t.Fatal("DefaultSchedules does not track HASURA_GRAPHQL_ADMIN_SECRET")
}

// TestLoadRotationStateMigratesLegacyName: a state saved before P7-LIVE-05
// tracks the admin secret under the old name; loading renames the row and
// keeps its history, and drops it when the new name already has a row.
func TestLoadRotationStateMigratesLegacyName(t *testing.T) {
	for _, tc := range []struct {
		name      string
		schedules []RotationSchedule
		wantLast  string
	}{
		{"rename", []RotationSchedule{{SecretName: legacyAdminSecretName, CadenceDays: 180, WindowDays: 3, LastRotated: "2026-01-02T00:00:00Z", NextDue: "2026-07-01T00:00:00Z"}}, "2026-01-02T00:00:00Z"},
		{"both present", []RotationSchedule{
			{SecretName: legacyAdminSecretName, CadenceDays: 180, LastRotated: "2025-01-01T00:00:00Z"},
			{SecretName: "HASURA_GRAPHQL_ADMIN_SECRET", CadenceDays: 180, LastRotated: "2026-03-03T00:00:00Z"},
		}, "2026-03-03T00:00:00Z"},
		{"two legacy rows", []RotationSchedule{
			{SecretName: legacyAdminSecretName, CadenceDays: 180, LastRotated: "2025-05-05T00:00:00Z"},
			{SecretName: legacyAdminSecretName, CadenceDays: 180, LastRotated: "2026-06-06T00:00:00Z"},
		}, "2026-06-06T00:00:00Z"},
		{"legacy row newer", []RotationSchedule{
			{SecretName: "HASURA_GRAPHQL_ADMIN_SECRET", CadenceDays: 180, LastRotated: "2025-01-01T00:00:00Z"},
			{SecretName: legacyAdminSecretName, CadenceDays: 180, LastRotated: "2026-04-04T00:00:00Z"},
		}, "2026-04-04T00:00:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := SaveRotationState(dir, &RotationState{Schedules: tc.schedules}); err != nil {
				t.Fatal(err)
			}
			state, err := LoadRotationState(dir)
			if err != nil {
				t.Fatal(err)
			}
			var found int
			for _, s := range state.Schedules {
				if s.SecretName == legacyAdminSecretName {
					t.Fatal("the legacy name survived the load")
				}
				if s.SecretName == "HASURA_GRAPHQL_ADMIN_SECRET" {
					found++
					if s.LastRotated != tc.wantLast {
						t.Fatalf("LastRotated = %q, want %q", s.LastRotated, tc.wantLast)
					}
				}
			}
			if found != 1 {
				t.Fatalf("HASURA_GRAPHQL_ADMIN_SECRET rows = %d, want 1", found)
			}
		})
	}
}
