package secrets

import (
	"strings"
	"testing"
)

// TestRotationLogPath_Format verifies the log path is under the expected secrets dir.
func TestRotationLogPath_Format(t *testing.T) {
	got := rotationLogPath("/tmp/project")
	if !strings.Contains(got, SecretsDir) {
		t.Errorf("rotationLogPath: %q does not contain SecretsDir %q", got, SecretsDir)
	}
	if !strings.HasSuffix(got, "rotation-log.json") {
		t.Errorf("rotationLogPath: %q does not end with rotation-log.json", got)
	}
}

// TestLoadRotationLog_NotExists returns empty log when file is missing.
func TestLoadRotationLog_NotExists(t *testing.T) {
	dir := t.TempDir()
	log, err := LoadRotationLog(dir)
	if err != nil {
		t.Fatalf("LoadRotationLog on empty dir: unexpected error: %v", err)
	}
	if log == nil {
		t.Fatal("LoadRotationLog: returned nil log")
	}
	if len(log.Events) != 0 {
		t.Errorf("LoadRotationLog on empty dir: len(Events) = %d, want 0", len(log.Events))
	}
}

// TestAppendRotationEvent_WritesAndReads verifies append creates log entries.
func TestAppendRotationEvent_WritesAndReads(t *testing.T) {
	dir := t.TempDir()

	if err := AppendRotationEvent(dir, "JWT_SIGNING_KEY", "success", "rotated via test"); err != nil {
		t.Fatalf("AppendRotationEvent: %v", err)
	}

	log, err := LoadRotationLog(dir)
	if err != nil {
		t.Fatalf("LoadRotationLog after append: %v", err)
	}
	if len(log.Events) != 1 {
		t.Fatalf("Events: got %d, want 1", len(log.Events))
	}
	e := log.Events[0]
	if e.SecretName != "JWT_SIGNING_KEY" {
		t.Errorf("SecretName: got %q, want JWT_SIGNING_KEY", e.SecretName)
	}
	if e.Status != "success" {
		t.Errorf("Status: got %q, want success", e.Status)
	}
	if e.Note != "rotated via test" {
		t.Errorf("Note: got %q, want 'rotated via test'", e.Note)
	}
	if e.RotatedAt == "" {
		t.Error("RotatedAt: empty, want RFC3339 timestamp")
	}
}

// TestAppendRotationEvent_MultipleEvents verifies accumulation across appends.
func TestAppendRotationEvent_MultipleEvents(t *testing.T) {
	dir := t.TempDir()

	secrets := []string{"HASURA_GRAPHQL_ADMIN_SECRET", "PLUGIN_INTERNAL_SECRET", "AGE_BACKUP_KEY"}
	for _, s := range secrets {
		if err := AppendRotationEvent(dir, s, "success", ""); err != nil {
			t.Fatalf("AppendRotationEvent(%q): %v", s, err)
		}
	}

	log, err := LoadRotationLog(dir)
	if err != nil {
		t.Fatalf("LoadRotationLog: %v", err)
	}
	if len(log.Events) != len(secrets) {
		t.Errorf("Events: got %d, want %d", len(log.Events), len(secrets))
	}
	if log.UpdatedAt == "" {
		t.Error("UpdatedAt: empty after append")
	}
}

// TestAddSchedule_AddsNewEntry verifies a new schedule entry is persisted.
func TestAddSchedule_AddsNewEntry(t *testing.T) {
	dir := t.TempDir()

	if err := AddSchedule(dir, "MY_CUSTOM_SECRET", 60, 3); err != nil {
		t.Fatalf("AddSchedule: %v", err)
	}

	state, err := LoadRotationState(dir)
	if err != nil {
		t.Fatalf("LoadRotationState: %v", err)
	}

	found := false
	for _, s := range state.Schedules {
		if s.SecretName == "MY_CUSTOM_SECRET" {
			found = true
			if s.CadenceDays != 60 {
				t.Errorf("CadenceDays: got %d, want 60", s.CadenceDays)
			}
			if s.WindowDays != 3 {
				t.Errorf("WindowDays: got %d, want 3", s.WindowDays)
			}
			if s.NextDue == "" {
				t.Error("NextDue: empty, want RFC3339")
			}
		}
	}
	if !found {
		t.Error("MY_CUSTOM_SECRET not found in schedules after AddSchedule")
	}
}

// TestAddSchedule_UpdatesExisting verifies updating an existing schedule entry.
func TestAddSchedule_UpdatesExisting(t *testing.T) {
	dir := t.TempDir()

	if err := AddSchedule(dir, "MY_SECRET", 30, 1); err != nil {
		t.Fatalf("AddSchedule (first): %v", err)
	}
	if err := AddSchedule(dir, "MY_SECRET", 90, 7); err != nil {
		t.Fatalf("AddSchedule (update): %v", err)
	}

	state, err := LoadRotationState(dir)
	if err != nil {
		t.Fatalf("LoadRotationState: %v", err)
	}

	count := 0
	for _, s := range state.Schedules {
		if s.SecretName == "MY_SECRET" {
			count++
			if s.CadenceDays != 90 {
				t.Errorf("CadenceDays after update: got %d, want 90", s.CadenceDays)
			}
		}
	}
	if count != 1 {
		t.Errorf("MY_SECRET appears %d times in schedules, want 1 (no duplicates)", count)
	}
}

// TestAddSchedule_OnDemand verifies cadence=0 leaves NextDue empty.
func TestAddSchedule_OnDemand(t *testing.T) {
	dir := t.TempDir()

	if err := AddSchedule(dir, "STRIPE_WEBHOOK_SECRET", 0, 0); err != nil {
		t.Fatalf("AddSchedule (on-demand): %v", err)
	}

	state, err := LoadRotationState(dir)
	if err != nil {
		t.Fatalf("LoadRotationState: %v", err)
	}

	for _, s := range state.Schedules {
		if s.SecretName == "STRIPE_WEBHOOK_SECRET" {
			if s.NextDue != "" {
				t.Errorf("NextDue: got %q, want empty for on-demand (cadence=0)", s.NextDue)
			}
			return
		}
	}
	t.Error("STRIPE_WEBHOOK_SECRET not found after AddSchedule")
}

// TestInitSchedules_PopulatesDefaults verifies all default schedules are written.
func TestInitSchedules_PopulatesDefaults(t *testing.T) {
	dir := t.TempDir()

	if err := InitSchedules(dir); err != nil {
		t.Fatalf("InitSchedules: %v", err)
	}

	state, err := LoadRotationState(dir)
	if err != nil {
		t.Fatalf("LoadRotationState after InitSchedules: %v", err)
	}

	defaults := DefaultSchedules()
	nameSet := make(map[string]bool, len(state.Schedules))
	for _, s := range state.Schedules {
		nameSet[s.SecretName] = true
	}
	for _, def := range defaults {
		if !nameSet[def.SecretName] {
			t.Errorf("default schedule %q missing after InitSchedules", def.SecretName)
		}
	}
}

// TestInitSchedules_Idempotent verifies calling twice does not duplicate entries.
func TestInitSchedules_Idempotent(t *testing.T) {
	dir := t.TempDir()

	if err := InitSchedules(dir); err != nil {
		t.Fatalf("InitSchedules (1st): %v", err)
	}
	if err := InitSchedules(dir); err != nil {
		t.Fatalf("InitSchedules (2nd): %v", err)
	}

	state, err := LoadRotationState(dir)
	if err != nil {
		t.Fatalf("LoadRotationState: %v", err)
	}

	counts := make(map[string]int)
	for _, s := range state.Schedules {
		counts[s.SecretName]++
	}
	for name, n := range counts {
		if n > 1 {
			t.Errorf("schedule %q duplicated %d times after double InitSchedules", name, n)
		}
	}
}

// TestGenerateRandomString_Length verifies the output matches the requested length.
func TestGenerateRandomString_Length(t *testing.T) {
	tests := []struct{ length int }{
		{16}, {32}, {64}, {1}, {0},
	}
	for _, tc := range tests {
		got := generateRandomString(tc.length)
		if len(got) != tc.length {
			t.Errorf("generateRandomString(%d): len = %d, want %d", tc.length, len(got), tc.length)
		}
	}
}

// TestGenerateRandomString_CharsetOnly verifies output contains only alphanumeric chars.
func TestGenerateRandomString_CharsetOnly(t *testing.T) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for i := 0; i < 10; i++ {
		s := generateRandomString(64)
		for _, c := range s {
			if !strings.ContainsRune(charset, c) {
				t.Errorf("generateRandomString: char %q not in charset", c)
			}
		}
	}
}

// TestGenerateRandomString_Entropy verifies two calls produce different output.
func TestGenerateRandomString_Entropy(t *testing.T) {
	a := generateRandomString(32)
	b := generateRandomString(32)
	if a == b {
		t.Error("generateRandomString: two calls returned identical strings (entropy failure)")
	}
}

// TestGenerateRotationValue_PasswordKey returns a non-empty value.
func TestGenerateRotationValue_PasswordKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantVal bool // true = non-empty value expected
	}{
		{"_PASSWORD suffix", "DB_PASSWORD", true},
		{"_PASS suffix", "ADMIN_PASS", true},
		{"JWT key", "JWT_SIGNING_KEY", true},
		{"SECRET key", "HASURA_GRAPHQL_ADMIN_SECRET", true},
		{"API_KEY key", "STRIPE_API_KEY", false}, // must rotate via provider
		{"TOKEN key", "GITHUB_TOKEN", false},
		{"default", "MY_CUSTOM_VAR", true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			value, hint := generateRotationValue(tc.key)
			if tc.wantVal && value == "" {
				t.Errorf("generateRotationValue(%q): got empty value, want non-empty", tc.key)
			}
			if !tc.wantVal && value != "" {
				t.Errorf("generateRotationValue(%q): got %q, want empty (provider-managed)", tc.key, value)
			}
			_ = hint
		})
	}
}

// TestGenerateRotationValue_HintPresent verifies provider-managed keys return a hint.
func TestGenerateRotationValue_HintPresent(t *testing.T) {
	_, hint := generateRotationValue("STRIPE_API_KEY")
	if hint == "" {
		t.Error("generateRotationValue(API_KEY): hint empty, want guidance message")
	}
}
