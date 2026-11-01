//go:build !nself_devkeys

package entitlement

import "testing"

// Default builds ignore the override, so PublishFromCache cannot take the dev-key early return.
func TestDevKeyModeIgnoresOverrideInDefaultBuild(t *testing.T) {
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "dev-key")
	if DevKeyMode() {
		t.Fatal("DevKeyMode() = true in a default build, want false")
	}
}
