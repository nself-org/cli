// Package compattest runs a plugin test body in both ADR 0021 compat modes.
//
// Tests select a mode with Set or Both, never by editing the environment by
// hand. It imports the standard library and the sibling compat package only.
package compattest

import (
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/compat"
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
		t.Run(m.name, func(t *testing.T) {
			Set(t, m.on)
			fn(t)
		})
	}
}
