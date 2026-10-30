package commands

import (
	"os"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestBackupStatusRestoresProcessEnv exercises the loader-driving test as a
// subtest so its cleanup runs before the caller checks for fixture pollution.
func TestBackupStatusRestoresProcessEnv(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		t.Setenv("PROJECT_NAME", "caller-project")
		t.Setenv("ENV", "staging")
		t.Run("project status", TestBackupStatusInProjectKeepsExistingFields)
		for key, want := range map[string]string{"PROJECT_NAME": "caller-project", "ENV": "staging"} {
			if got := os.Getenv(key); got != want {
				t.Errorf("backup status leaked %s: got %q, want %q", key, got, want)
			}
		}
	})
}

// TestRestoreProcessEnv preserves unset, empty and populated variables even
// when the loader adds, overwrites or removes values without t.Setenv.
func TestRestoreProcessEnv(t *testing.T) {
	const added, empty, changed = "NSELF_TEST_ENV_ADDED", "NSELF_TEST_ENV_EMPTY", "NSELF_TEST_ENV_CHANGED"
	t.Setenv(added, "")
	if err := os.Unsetenv(added); err != nil {
		t.Fatal(err)
	}
	t.Setenv(empty, "")
	t.Setenv(changed, "original")
	t.Run("loader mutation", func(t *testing.T) {
		restoreProcessEnv(t)
		for key, value := range map[string]string{added: "added", changed: "changed"} {
			if err := os.Setenv(key, value); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Unsetenv(empty); err != nil {
			t.Fatal(err)
		}
	})
	if _, ok := os.LookupEnv(added); ok {
		t.Error("cleanup retained added variable")
	}
	if value, ok := os.LookupEnv(empty); !ok || value != "" {
		t.Error("cleanup did not restore empty variable")
	}
	if os.Getenv(changed) != "original" {
		t.Error("cleanup did not restore populated variable")
	}
}
