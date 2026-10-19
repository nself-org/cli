package licensetest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/license/licensetest"
)

// TestIsolateHidesAmbientLicense is the D-1256 regression: a host that stores a
// licence key (env or ~/.nself/license) must not leak it into a test that
// expects "no licence". Such a leak made revalidateKeys renew a real token on
// mm4, so the V15 gate returned nil instead of E125.
func TestIsolateHidesAmbientLicense(t *testing.T) {
	ambient := t.TempDir()
	dir := filepath.Join(ambient, ".nself", "license")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key"), []byte("nself_pro_ambientkey0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", ambient)
	t.Setenv("USERPROFILE", ambient)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "nself_pro_envkey0123456789abcdef")
	t.Setenv("NSELF_LICENSE_KEY_1", "nself_pro_numbered0123456789abcdef")
	t.Setenv("NSELF_LICENSE_KEY_10", "nself_pro_numbered1023456789abcdef")
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "01")

	if len(license.CollectLicenseKeys()) == 0 {
		t.Fatal("fixture invalid: ambient keys not visible before Isolate")
	}
	licensetest.Isolate(t)

	if keys := license.CollectLicenseKeys(); len(keys) != 0 {
		t.Fatalf("ambient licence keys visible after Isolate: %d", len(keys))
	}
	if entry, err := license.ReadCache(); err != nil || entry != nil {
		t.Fatalf("licence cache visible after Isolate: %v %v", entry, err)
	}
	if v := os.Getenv("LICENSE_PUBLIC_KEY_OVERRIDE"); v != "" {
		t.Fatalf("key override survived Isolate: %q", v)
	}
}
