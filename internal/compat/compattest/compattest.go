// Package compattest runs a test body in both ADR 0021 compat modes.
//
// Tests select a mode with Set or Both, never with a bare compat.V15() branch
// (scripts/ci/compat-markers.sh scans non-test files only).
//
// Layer: L0. Imports the standard library and internal/compat only.
package compattest

import (
	"testing"

	"github.com/nself-org/cli/internal/compat"
)

// Set selects v1.5 behaviour when on is true and v1.4 behaviour otherwise for
// the duration of t. It uses t.Setenv, so the previous value is restored when
// the test ends and the test cannot run in parallel.
func Set(t testing.TB, on bool) {
	t.Helper()
	if on {
		t.Setenv(compat.EnvVar, "1")
		return
	}
	t.Setenv(compat.EnvVar, "0")
}

// Both runs fn as two subtests, "v1.4" then "v1.5", with the matching mode
// selected through Set. Each run observes the matching compat.V15() value.
func Both(t *testing.T, fn func(t *testing.T)) {
	t.Helper()
	for _, m := range []struct {
		name string
		on   bool
	}{{"v1.4", false}, {"v1.5", true}} {
		m := m
		t.Run(m.name, func(t *testing.T) {
			Set(t, m.on)
			fn(t)
		})
	}
}
