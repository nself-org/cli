package entitlement_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/license/entitlement"
)

func TestRefreshPublishesEveryBundleOffline(t *testing.T) {
	compattest.Set(t, true)
	first, priv := signedFixture(t, "bundle-one", []string{"chat"}, time.Now().Add(time.Hour))
	t.Setenv("NSELF_LICENSE_KEY_1", "bundle-two")
	second := replacementEntry(t, priv, "bundle-two", []string{"claw"}, time.Now().Add(2*time.Hour))
	if err := license.WriteCache(second); err != nil {
		t.Fatal(err)
	}
	installLicensed(t, "chat")
	installLicensed(t, "claw")
	dir := t.TempDir()
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatalf("two owned bundles must cover both plugins offline: %v", err)
	}
	data, err := os.ReadFile(entitlement.Path(dir))
	if err != nil || string(data) != strings.Join([]string{second.JWT, first.JWT}, "\n") {
		t.Fatalf("want both bundle JWTs, latest expiry first; read error: %v", err)
	}
	cache, _ := license.CachePath()
	if err := os.Remove(cache); err != nil {
		t.Fatal(err)
	}
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatalf("multiline runtime file must retain both bundles without cache: %v", err)
	}
}
