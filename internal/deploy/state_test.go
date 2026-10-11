package deploy

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSaveAndLoadDeployState(t *testing.T) {
	tmp := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)

	state := StrategyState{
		Strategy:      StrategyBlueGreen,
		Target:        "prod",
		StartedAt:     now,
		CompletedAt:   now.Add(30 * time.Second),
		Success:       true,
		CanaryPercent: 100,
		RolledBack:    false,
	}

	if err := SaveDeployState(tmp, state); err != nil {
		t.Fatalf("SaveDeployState failed: %v", err)
	}

	// Verify file exists at expected path.
	path := filepath.Join(tmp, DeployStateFile)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file not found at %s: %v", path, err)
	}

	loaded, err := LoadDeployState(tmp)
	if err != nil {
		t.Fatalf("LoadDeployState failed: %v", err)
	}

	if loaded.Strategy != state.Strategy {
		t.Errorf("Strategy: got %q, want %q", loaded.Strategy, state.Strategy)
	}
	if loaded.Target != state.Target {
		t.Errorf("Target: got %q, want %q", loaded.Target, state.Target)
	}
	if !loaded.Success {
		t.Errorf("expected Success=true")
	}
	if loaded.CanaryPercent != 100 {
		t.Errorf("CanaryPercent: got %d, want 100", loaded.CanaryPercent)
	}
}

func TestLoadDeployState_missingFile(t *testing.T) {
	tmp := t.TempDir()
	state, err := LoadDeployState(tmp)
	if err != nil {
		t.Fatalf("expected nil error for missing state file, got: %v", err)
	}
	// Zero-value state returned.
	if state.Strategy != "" {
		t.Errorf("expected empty Strategy for missing file, got %q", state.Strategy)
	}
}

func TestSaveDeployState_createsParentDirs(t *testing.T) {
	tmp := t.TempDir()
	state := StrategyState{
		Strategy: StrategyCanary,
		Target:   "staging",
		Success:  false,
		Error:    "health check timed out",
	}

	// The .nself/state/ directories do not exist yet.
	if err := SaveDeployState(tmp, state); err != nil {
		t.Fatalf("SaveDeployState should create parent dirs: %v", err)
	}

	// Verify file is readable.
	loaded, err := LoadDeployState(tmp)
	if err != nil {
		t.Fatalf("LoadDeployState failed: %v", err)
	}
	if loaded.Error != "health check timed out" {
		t.Errorf("Error: got %q, want %q", loaded.Error, "health check timed out")
	}
}

func TestSaveDeployState_failureState(t *testing.T) {
	tmp := t.TempDir()
	state := StrategyState{
		Strategy:   StrategyBlueGreen,
		Target:     "prod",
		Success:    false,
		RolledBack: true,
		Error:      "green containers did not become healthy within 30s",
	}

	if err := SaveDeployState(tmp, state); err != nil {
		t.Fatalf("SaveDeployState failed: %v", err)
	}

	loaded, err := LoadDeployState(tmp)
	if err != nil {
		t.Fatalf("LoadDeployState failed: %v", err)
	}

	if loaded.Success {
		t.Errorf("expected Success=false for failure state")
	}
	if !loaded.RolledBack {
		t.Errorf("expected RolledBack=true")
	}
	if loaded.Error == "" {
		t.Errorf("expected non-empty Error in failure state")
	}
}

func TestStrategyConstants(t *testing.T) {
	if StrategyRolling != "rolling" {
		t.Errorf("StrategyRolling: got %q, want %q", StrategyRolling, "rolling")
	}
	if StrategyBlueGreen != "blue-green" {
		t.Errorf("StrategyBlueGreen: got %q, want %q", StrategyBlueGreen, "blue-green")
	}
	if StrategyCanary != "canary" {
		t.Errorf("StrategyCanary: got %q, want %q", StrategyCanary, "canary")
	}
}

func TestDeployStateFilePath(t *testing.T) {
	if DeployStateFile != ".nself/state/deploy.json" {
		t.Errorf("DeployStateFile: got %q, want %q", DeployStateFile, ".nself/state/deploy.json")
	}
}

// TestSaveDeployState_schemaVersion verifies that SaveDeployState always stamps
// SchemaVersion=1 on write, and that LoadDeployState round-trips it correctly.
func TestSaveDeployState_schemaVersion(t *testing.T) {
	tmp := t.TempDir()

	// Caller does not set SchemaVersion — SaveDeployState must set it to 1.
	state := StrategyState{
		Strategy: StrategyBlueGreen,
		Target:   "prod",
		Success:  true,
	}

	if err := SaveDeployState(tmp, state); err != nil {
		t.Fatalf("SaveDeployState failed: %v", err)
	}

	loaded, err := LoadDeployState(tmp)
	if err != nil {
		t.Fatalf("LoadDeployState failed: %v", err)
	}
	if loaded.SchemaVersion != 1 {
		t.Errorf("SchemaVersion: got %d, want 1", loaded.SchemaVersion)
	}
}

// TestSaveDeployState_atomicWrite verifies that the state file is written
// atomically (no .tmp file left behind on success).
func TestSaveDeployState_atomicWrite(t *testing.T) {
	tmp := t.TempDir()

	state := StrategyState{Strategy: StrategyCanary, Target: "staging", Success: true}
	if err := SaveDeployState(tmp, state); err != nil {
		t.Fatalf("SaveDeployState failed: %v", err)
	}

	// No .tmp sibling should remain after a successful write.
	tmpFile := filepath.Join(tmp, DeployStateFile+".tmp")
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Errorf("temp file still present after successful write: %s", tmpFile)
	}
}

// TestSaveDeployState_filePerms verifies that the state file is written with
// 0600 permissions (deploy state contains hostnames and error context).
func TestSaveDeployState_filePerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped on Windows: chmod 0600 is not enforced")
	}
	tmp := t.TempDir()

	state := StrategyState{Strategy: StrategyBlueGreen, Target: "prod", Success: true}
	if err := SaveDeployState(tmp, state); err != nil {
		t.Fatalf("SaveDeployState failed: %v", err)
	}

	path := filepath.Join(tmp, DeployStateFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("file permissions: got %04o, want 0600", got)
	}
}

// TestLoadDeployState_readOutcomes pins D-0144: only an absent file is the
// empty state; an unreadable or corrupt file is an error, never a zero state.
func TestLoadDeployState_readOutcomes(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(t *testing.T, path string)
		wantErr bool
	}{
		{name: "absent", setup: func(*testing.T, string) {}},
		{name: "unreadable", wantErr: true, setup: func(t *testing.T, path string) {
			// A directory at the file path fails ReadFile on every platform,
			// without relying on permissions a root test runner would ignore.
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "corrupt", wantErr: true, setup: func(t *testing.T, path string) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(`{"strategy": "rolling"`), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(t, filepath.Join(root, DeployStateFile))
			st, err := LoadDeployState(root)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if st.Strategy != "" || st.Target != "" {
				t.Errorf("want zero state, got %+v", st)
			}
		})
	}
}
