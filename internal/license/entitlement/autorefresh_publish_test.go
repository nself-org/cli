package entitlement

import (
	"os"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license"
)

// PublishFromCache (air-gapped `license import`) writes the cached, verified token to the
// entitlement file when a licensed plugin is installed.
func TestPublishFromCacheWritesTheCachedToken(t *testing.T) {
	compattest.Set(t, true)
	p := newArPing(t, time.Now(), "chat")
	p.seedCache(t)
	entry, err := license.ReadCache()
	if err != nil || entry == nil {
		t.Fatalf("seeded cache unreadable: %v", err)
	}
	dir := t.TempDir()
	if err := PublishFromCache(dir); err != nil {
		t.Fatalf("PublishFromCache = %v, want nil", err)
	}
	got, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatalf("entitlement file not written: %v", err)
	}
	if string(got) != entry.JWT {
		t.Fatalf("entitlement file holds %q, want the cached token", got)
	}
}

// With no licensed plugin installed there is nothing to entitle: no error and no file.
func TestPublishFromCacheWithoutLicensedPluginWritesNothing(t *testing.T) {
	compattest.Set(t, true)
	p := newArPing(t, time.Now())
	p.seedCache(t)
	dir := t.TempDir()
	if err := PublishFromCache(dir); err != nil {
		t.Fatalf("PublishFromCache = %v, want nil", err)
	}
	if _, err := os.Stat(Path(dir)); !os.IsNotExist(err) {
		t.Fatalf("entitlement file exists without a licensed plugin (stat err = %v)", err)
	}
}
