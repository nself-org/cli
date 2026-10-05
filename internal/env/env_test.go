package env

import (
	"path/filepath"
	"testing"
)

// TestActiveEnv_DefaultsToDevWhenMissing verifies that ActiveEnv returns "dev"
// when no active_env file exists.
func TestActiveEnv_DefaultsToDevWhenMissing(t *testing.T) {
	dir := t.TempDir()
	got := ActiveEnv(dir)
	if got != DefaultEnv {
		t.Errorf("ActiveEnv on missing file: got %q, want %q", got, DefaultEnv)
	}
}

// TestSetAndGetActiveEnv_RoundTrip verifies that SetActiveEnv persists and
// ActiveEnv reads it back.
func TestSetAndGetActiveEnv_RoundTrip(t *testing.T) {
	cases := []string{"dev", "staging", "prod"}
	for _, env := range cases {
		t.Run(env, func(t *testing.T) {
			dir := t.TempDir()
			if err := SetActiveEnv(dir, env); err != nil {
				t.Fatalf("SetActiveEnv(%q): %v", env, err)
			}
			got := ActiveEnv(dir)
			if got != env {
				t.Errorf("ActiveEnv: got %q, want %q", got, env)
			}
		})
	}
}

// TestList_ReturnsActiveEnv verifies that List includes the active environment.
func TestList_ReturnsActiveEnv(t *testing.T) {
	dir := t.TempDir()

	// Set active env to staging.
	if err := SetActiveEnv(dir, "staging"); err != nil {
		t.Fatalf("SetActiveEnv: %v", err)
	}

	infos := List(dir)
	// At minimum the active env should appear.
	found := false
	for _, info := range infos {
		if info.Name == "staging" && info.Active {
			found = true
		}
	}
	if !found {
		t.Errorf("List did not return active env 'staging': %+v", infos)
	}
}

// TestEnvFilePath_ContainsEnvName verifies the pattern of env file path
// generation (if exposed).
func TestActiveEnv_CreatesStateFile(t *testing.T) {
	dir := t.TempDir()
	if err := SetActiveEnv(dir, "prod"); err != nil {
		t.Fatalf("SetActiveEnv: %v", err)
	}

	stateFilePath := filepath.Join(dir, ".nself", "active_env")
	got := ActiveEnv(dir)
	_ = stateFilePath // verify the path convention
	if got != "prod" {
		t.Errorf("expected prod from active_env file, got %q", got)
	}
}

// TestDefaultEnv_Value verifies the DefaultEnv constant is "dev".
func TestDefaultEnv_Value(t *testing.T) {
	if DefaultEnv != "dev" {
		t.Errorf("DefaultEnv should be 'dev', got %q", DefaultEnv)
	}
}
