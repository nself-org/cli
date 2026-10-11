package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// minimalConfig returns a *config.Config with sensible zero-value defaults
// suitable for unit tests that do not exercise real infrastructure.
func minimalConfig() *config.Config {
	return &config.Config{
		ProjectName: "testproject",
		Postgres: config.PostgresConfig{
			User:     "postgres",
			Password: "",
			Port:     5432,
			DB:       "nself",
		},
		Backup: config.BackupConfig{
			Dir: "./backups",
		},
	}
}

// ── buildPgURL ────────────────────────────────────────────────────────

func TestBuildPgURL_WithPassword(t *testing.T) {
	t.Parallel()
	cfg := minimalConfig()
	cfg.Postgres.User = "admin"
	cfg.Postgres.Password = "secret"
	cfg.Postgres.Port = 5432
	cfg.Postgres.DB = "mydb"

	got := buildPgURL(cfg)
	if !strings.Contains(got, "admin:secret@") {
		t.Errorf("expected admin:secret@..., got %s", got)
	}
	if !strings.Contains(got, "mydb") {
		t.Errorf("expected mydb in URL, got %s", got)
	}
}

func TestBuildPgURL_NoPassword(t *testing.T) {
	t.Parallel()
	cfg := minimalConfig()
	cfg.Postgres.User = "pguser"
	cfg.Postgres.Password = ""
	cfg.Postgres.Port = 5432
	cfg.Postgres.DB = "nself"

	got := buildPgURL(cfg)
	if strings.Contains(got, ":@") {
		t.Errorf("empty password should not produce :@ in URL, got %s", got)
	}
	if !strings.HasPrefix(got, "postgresql://pguser@") {
		t.Errorf("expected postgresql://pguser@..., got %s", got)
	}
}

func TestBuildPgURL_Defaults(t *testing.T) {
	t.Parallel()
	cfg := minimalConfig()
	// Zero-value fields should produce sensible defaults.
	got := buildPgURL(cfg)
	if !strings.Contains(got, "5432") {
		t.Errorf("expected default port 5432 in URL, got %s", got)
	}
	if !strings.Contains(got, "nself") {
		t.Errorf("expected default db 'nself' in URL, got %s", got)
	}
}

// ── redactURL ─────────────────────────────────────────────────────────

func TestRedactURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{
			"postgresql://user:hunter2@localhost:5432/db",
			"postgresql://user:***@localhost:5432/db",
		},
		{
			"postgresql://user@localhost:5432/db",
			"postgresql://user@localhost:5432/db",
		},
		{
			"postgresql://localhost/db",
			"postgresql://localhost/db",
		},
	}
	for _, tc := range cases {
		got := redactURL(tc.in)
		if got != tc.want {
			t.Errorf("redactURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ── cronToSystemd ─────────────────────────────────────────────────────

func TestCronToSystemd_Daily(t *testing.T) {
	t.Parallel()
	got := cronToSystemd("0 2 * * *")
	if got != "*-*-* 02:00:00 UTC" {
		t.Errorf("daily cron: got %q, want %q", got, "*-*-* 02:00:00 UTC")
	}
}

func TestCronToSystemd_Weekly(t *testing.T) {
	t.Parallel()
	got := cronToSystemd("30 3 * * 0")
	if !strings.Contains(got, "Sun") {
		t.Errorf("weekly cron: expected Sun in %q", got)
	}
	if !strings.Contains(got, "03:30") {
		t.Errorf("weekly cron: expected 03:30 in %q", got)
	}
}

func TestCronToSystemd_Monthly(t *testing.T) {
	t.Parallel()
	got := cronToSystemd("0 4 1 * *")
	if !strings.Contains(got, "01") {
		t.Errorf("monthly cron: expected dom 01 in %q", got)
	}
}

func TestCronToSystemd_InvalidField(t *testing.T) {
	t.Parallel()
	// Should not panic on malformed input; returns fallback.
	got := cronToSystemd("bad")
	if got != "bad" {
		t.Errorf("invalid cron: expected verbatim passthrough, got %q", got)
	}
}

// ── resume state ──────────────────────────────────────────────────────

func TestSaveLoadDeleteResumeState(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel — do not parallelise.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	state := ResumeState{
		BackupID:    "myproject_stream_20260423_020000.sql.age",
		Destination: "s3:mybucket",
		Key:         "myproject_stream_20260423_020000.sql.age",
		Recipients:  []string{"age1abc123"},
		StartedAt:   time.Now().UTC().Truncate(time.Second),
	}

	if err := saveResumeState(state); err != nil {
		t.Fatalf("saveResumeState: %v", err)
	}

	// Verify file on disk.
	stateFile := filepath.Join(tmpHome, ".nself", "backup-state", state.BackupID+".json")
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file not found: %v", err)
	}

	// Load and compare.
	loaded, err := loadResumeState(state.BackupID)
	if err != nil {
		t.Fatalf("loadResumeState: %v", err)
	}
	if loaded.BackupID != state.BackupID {
		t.Errorf("BackupID: got %q, want %q", loaded.BackupID, state.BackupID)
	}
	if loaded.Destination != state.Destination {
		t.Errorf("Destination: got %q, want %q", loaded.Destination, state.Destination)
	}
	if len(loaded.Recipients) != 1 || loaded.Recipients[0] != state.Recipients[0] {
		t.Errorf("Recipients: got %v, want %v", loaded.Recipients, state.Recipients)
	}

	// Delete.
	deleteResumeState(state.BackupID)
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Errorf("state file should be deleted after deleteResumeState")
	}
}

func TestLoadResumeState_Missing(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel — do not parallelise.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	_, err := loadResumeState("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for missing state file")
	}
	if !strings.Contains(err.Error(), "no resume state") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestListResumeStates(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel — do not parallelise.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	// Empty dir — should return nil, nil.
	states, err := listResumeStates()
	if err != nil {
		t.Fatalf("listResumeStates on empty dir: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("expected 0 states, got %d", len(states))
	}

	// Save two states.
	for i, id := range []string{"alpha.json.age", "beta.json.age"} {
		if err := saveResumeState(ResumeState{
			BackupID:    id,
			Destination: "r2:bucket",
			Key:         id,
			StartedAt:   time.Now().Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("saveResumeState: %v", err)
		}
	}

	states, err = listResumeStates()
	if err != nil {
		t.Fatalf("listResumeStates: %v", err)
	}
	if len(states) != 2 {
		t.Errorf("expected 2 states, got %d", len(states))
	}
}

// ── resolveRecipients — no network ───────────────────────────────────

func TestResolveRecipients_PlainKey(t *testing.T) {
	t.Parallel()
	keys := []string{"age1abc", "ssh-rsa AAAA..."}
	got, err := resolveRecipients(context.Background(), keys)
	if err != nil {
		t.Fatalf("resolveRecipients: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 keys, got %d", len(got))
	}
	if got[0] != "age1abc" {
		t.Errorf("first key: got %q", got[0])
	}
}

func TestResolveRecipients_Empty(t *testing.T) {
	t.Parallel()
	got, err := resolveRecipients(context.Background(), nil)
	if err != nil {
		t.Fatalf("resolveRecipients nil: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

// ── Stream dry-run ────────────────────────────────────────────────────

func TestStream_DryRun(t *testing.T) {
	t.Parallel()
	cfg := minimalConfig()
	cfg.ProjectName = "testproject"
	cfg.Backup.Remote = "s3:mybucket"

	// A recipient is supplied because streaming now fails closed without one.
	// This test is about the dry-run path and the destination, not about
	// encryption, so it exercises the ordinary encrypted case rather than
	// depending on the unencrypted default that used to be silent.
	result, err := Stream(context.Background(), cfg, StreamOptions{
		To:         "s3:mybucket",
		Recipients: []string{"age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsxxxxxx"},
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("Stream dry-run: %v", err)
	}
	if result == nil {
		t.Fatal("Stream dry-run: expected non-nil result")
	}
	if !strings.Contains(result.Destination, "s3:mybucket") {
		t.Errorf("destination: got %q, want s3:mybucket", result.Destination)
	}
}

func TestStream_DryRun_NoDestination(t *testing.T) {
	t.Parallel()
	cfg := minimalConfig()

	_, err := Stream(context.Background(), cfg, StreamOptions{DryRun: true})
	if err == nil {
		t.Fatal("expected error when no destination set")
	}
}

// ── StreamResult JSON serialisation ──────────────────────────────────

func TestStreamResult_JSON(t *testing.T) {
	t.Parallel()
	r := &StreamResult{
		BackupID:    "myproject_stream_20260423_020000.sql.age",
		Destination: "r2:bucket/myproject_stream_20260423_020000.sql.age",
		StartedAt:   time.Date(2026, 4, 23, 2, 0, 0, 0, time.UTC),
		Duration:    "1m23s",
		Encrypted:   true,
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back StreamResult
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.BackupID != r.BackupID {
		t.Errorf("BackupID round-trip: got %q, want %q", back.BackupID, r.BackupID)
	}
	if !back.Encrypted {
		t.Error("Encrypted should be true after round-trip")
	}
}

// ── zeroPad ───────────────────────────────────────────────────────────

func TestZeroPad(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"2":  "02",
		"02": "02",
		"10": "10",
		"*":  "*",
	}
	for in, want := range cases {
		got := zeroPad(in)
		if got != want {
			t.Errorf("zeroPad(%q) = %q, want %q", in, got, want)
		}
	}
}

// ── cronDOWName ───────────────────────────────────────────────────────

func TestCronDOWName(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"0": "Sun", "7": "Sun",
		"1": "Mon", "2": "Tue", "3": "Wed",
		"4": "Thu", "5": "Fri", "6": "Sat",
	}
	for in, want := range cases {
		got := cronDOWName(in)
		if got != want {
			t.Errorf("cronDOWName(%q) = %q, want %q", in, got, want)
		}
	}
}

// ── schedule ENV selector (D-0277) ────────────────────────────────────

// scheduleServiceFor renders the dry-run unit for a project dir whose .env is
// envFileBody ("" writes no .env) with the process ENV set to procEnv.
func scheduleServiceFor(t *testing.T, procEnv, envFileBody string) (string, error) {
	t.Helper()
	setScheduleProcessEnv(t, procEnv)
	dir := t.TempDir()
	if envFileBody != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envFileBody), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	err := ScheduleStream(minimalConfig(), ScheduleOptions{Cron: "0 3 * * *", To: "r2:b", ProjectDir: dir, BinaryPath: "/bin/sh", DryRun: true, Out: &out})
	return out.String(), err
}

// TestScheduleStreamEnvSelector: the unit carries exactly one
// Environment=ENV= line, directly after the PATH line, and its value is the
// selector that picks the cascade: process ENV, else .env, else dev. cfg.Env
// is not used (the loader rewrites it).
func TestScheduleStreamEnvSelector(t *testing.T) {
	skipUnitTestOnWindows(t)
	cases := []struct {
		name, procEnv, dotEnv, want string
	}{
		{"process ENV wins over .env", "prod", "ENV=dev\n", "prod"},
		{".env when the process has none", "", "PROJECT_NAME=x\nENV=staging\n", "staging"},
		{"dev when neither sets it", "", "PROJECT_NAME=x\n", "dev"},
		{"dev with no .env at all", "", "", "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scheduleServiceFor(t, tc.procEnv, tc.dotEnv)
			if err != nil {
				t.Fatalf("ScheduleStream: %v", err)
			}
			if n := strings.Count(got, "ENV="); n != 1 {
				t.Fatalf("want exactly one ENV= occurrence, got %d:\n%s", n, got)
			}
			pathThenEnv := "Environment=PATH=/usr/local/bin:/usr/bin:/bin\nEnvironment=ENV=" + tc.want + "\n"
			if !strings.Contains(got, pathThenEnv) {
				t.Errorf("want %q right after the PATH line:\n%s", "Environment=ENV="+tc.want, got)
			}
		})
	}
}

// TestScheduleStreamEnvRefusesUnsafe: a selector that could add a directive or
// expand in systemd is refused with nothing printed or written.
func TestScheduleStreamEnvRefusesUnsafe(t *testing.T) {
	skipUnitTestOnWindows(t)
	for _, env := range []string{"prod\nExecStartPre=/x", "prod\r", "pr$od", "a b", "a%h", "a/b", "a\"b"} {
		setScheduleProcessEnv(t, env)
		var out bytes.Buffer
		unitDir := t.TempDir()
		err := ScheduleStream(minimalConfig(), ScheduleOptions{Cron: "0 3 * * *", To: "r2:b", ProjectDir: t.TempDir(), BinaryPath: "/bin/sh", UnitDir: unitDir, Out: &out})
		if err == nil {
			t.Errorf("env %q must be refused", env)
		}
		if out.Len() != 0 {
			t.Errorf("env %q: nothing may be printed, got %q", env, out.String())
		}
		if entries, _ := os.ReadDir(unitDir); len(entries) != 0 {
			t.Errorf("env %q: nothing may be written, found %d files", env, len(entries))
		}
	}
	// The same applies to a bad value that comes from .env.
	if _, err := scheduleServiceFor(t, "", "ENV=a b\n"); err == nil {
		t.Error("an unsafe ENV in .env must be refused")
	}
}
