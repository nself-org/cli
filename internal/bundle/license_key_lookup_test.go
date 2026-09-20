package bundle

// license_key_lookup_test.go — regression for the bundle installer's
// "no license key configured" false negative.
//
// `nself license set <key>` writes ~/.nself/license/key. hasAnyLicenseKey
// used to look for ~/.nself/license.key — a LEGACY path with a dot where the
// current store has a directory separator — so a paying customer who set a
// key was told they had none, while `nself license status` showed the very
// same key as Active. Reproduced end to end against the live licence server
// before the fix.

import (
	"os"
	"path/filepath"
	"testing"
)

// withHome points os.UserHomeDir at a temp dir for the duration of a test.
func withHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads this on Windows
	// Ensure no ambient key leaks in from the developer's environment.
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY_OWNER", "")
	return home
}

func TestHasAnyLicenseKey_FindsTheKeyLicenseSetWrites(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".nself", "license")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// This is the exact path and filename `nself license set` writes.
	if err := os.WriteFile(filepath.Join(dir, "key"), []byte("nself_pro_"+testKeyBody), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if !hasAnyLicenseKey() {
		t.Error("a key stored at ~/.nself/license/key was not found; " +
			"bundle install will wrongly report \"no license key configured\"")
	}
}

func TestHasAnyLicenseKey_FalseWhenNoKeyAnywhere(t *testing.T) {
	withHome(t)
	if hasAnyLicenseKey() {
		t.Error("reported a license key with none set, in env or on disk")
	}
}

func TestHasAnyLicenseKey_HonoursOwnerEnvVar(t *testing.T) {
	withHome(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY_OWNER", "nself_pro_"+testKeyBody)
	if !hasAnyLicenseKey() {
		t.Error("the owner all-access env var must still count as a key")
	}
}

const testKeyBody = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
