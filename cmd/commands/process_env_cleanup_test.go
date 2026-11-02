package commands

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestBackupLoadersRestoreProcessEnv exercises loader-driving tests as subtests
// so their cleanup runs before the caller checks for fixture pollution (D-0275).
func TestBackupLoadersRestoreProcessEnv(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		t.Setenv("PROJECT_NAME", "caller-project")
		t.Setenv("ENV", "staging")
		snapshot := func() map[string]string {
			env := make(map[string]string)
			for _, entry := range os.Environ() {
				key, value, _ := strings.Cut(entry, "=")
				env[key] = value
			}
			return env
		}
		for _, tc := range []struct {
			name string
			run  func(*testing.T)
		}{
			{"project status", TestBackupStatusInProjectKeepsExistingFields},
			{"schedule credentials", TestBackupScheduleProdMinioCredentials},
			{"stream credentials", TestBackupStreamProdMinioCredentials},
		} {
			before := snapshot()
			t.Run(tc.name, tc.run)
			if !reflect.DeepEqual(snapshot(), before) {
				t.Errorf("%s leaked process environment changes", tc.name)
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
