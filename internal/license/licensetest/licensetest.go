// Package licensetest isolates tests from the licence state of the host.
//
// Purpose: a test that expects "no licence" must not see a key stored on the CI
// host. Inputs: the test. Outputs: a private HOME and an empty licence cache.
// Constraints: tests only; never imported by production code.
// SPORT: D-1256 (mm4 V15 gate returned nil instead of E125).
package licensetest

import (
	"fmt"
	"path/filepath"
	"testing"
)

// Isolate hides every ambient licence source for the duration of the test:
// the stored key under ~/.nself/license, the NSELF_PLUGIN_LICENSE_KEY and
// NSELF_LICENSE_KEY_1..10 variables, the licence cache and the dev-key override, and points ping at a refused port.
// With a key visible, the build refresh validates it online, writes a fresh
// cache and publishes a covering token, so the expected E125 never fires.
// It returns the private home directory.
func Isolate(t testing.TB) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(home, "missing-license.json"))
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "")
	t.Setenv("NSELF_PING_API_URL", "http://127.0.0.1:1") // refused: no online validation
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	for i := 1; i <= 10; i++ {
		t.Setenv(fmt.Sprintf("NSELF_LICENSE_KEY_%d", i), "")
	}
	return home
}
