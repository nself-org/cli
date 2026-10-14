package entitlement_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license/entitlement"
)

func TestRefreshWithoutPluginsDoesNotValidateOnline(t *testing.T) {
	compattest.Set(t, true)
	entry, _ := signedFixture(t, "valid-key", []string{"chat"}, time.Now().Add(time.Hour))
	requests := make(chan struct{}, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests <- struct{}{}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	t.Setenv("NSELF_PING_API_URL", server.URL)
	dir := t.TempDir()
	if err := entitlement.RefreshForPlugins(context.Background(), dir, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 0 {
		t.Fatal("a project without licensed plugins must not contact validation")
	}
	data, err := os.ReadFile(entitlement.Path(dir))
	if err != nil || string(data) != entry.JWT {
		t.Fatalf("offline cached entitlement was not published: %v", err)
	}
}

func TestRefreshRejectsSignedZeroExpiry(t *testing.T) {
	compattest.Set(t, true)
	signedFixture(t, "valid-key", []string{"chat"}, time.Unix(0, 0))
	dir := t.TempDir()
	err := entitlement.RefreshForPlugins(context.Background(), dir, t.TempDir())
	if err == nil {
		t.Fatalf("zero signed expiry must be rejected: %v", err)
	}
	data, readErr := os.ReadFile(entitlement.Path(dir))
	if readErr != nil || len(data) != 0 {
		t.Fatalf("invalid expiry published token: %q %v", data, readErr)
	}
}
