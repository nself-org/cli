//go:build nself_devkeys

package entitlement

import (
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// A development build with the override set skips publishing in v1.5 mode, without touching
// the filesystem or the licence cache.
func TestPublishFromCacheDevKeyEarlyReturn(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "dev-key")
	if err := PublishFromCache(t.TempDir()); err != nil {
		t.Fatalf("PublishFromCache with dev key = %v, want nil", err)
	}
}
