package compat

import (
	"os"
	"testing"
)

// unset removes EnvVar for the rest of the test; a prior t.Setenv call in the
// same test restores the original value at cleanup.
func unset(t *testing.T) {
	t.Helper()
	if err := os.Unsetenv(EnvVar); err != nil {
		t.Fatal(err)
	}
}
