package entitlement_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license/entitlement"
)

func TestRefreshInstalledRevalidatesWithoutNewKey(t *testing.T) {
	compattest.Set(t, true)
	entry, _ := signedFixture(t, "accepted", []string{"chat"}, time.Now().Add(time.Hour))
	installLicensed(t, "chat")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	t.Setenv("NSELF_PING_API_URL", server.URL)
	dir := t.TempDir()
	putRuntimeToken(t, dir, entry.JWT)
	if err := entitlement.RefreshInstalled(context.Background(), dir); err == nil {
		t.Fatal("start accepted a revoked licence from cache")
	}
	data, err := os.ReadFile(entitlement.Path(dir))
	if err != nil || len(data) != 0 {
		t.Fatalf("revoked runtime token survived: size=%d, err=%v", len(data), err)
	}
}

func TestRejectedKeyReportsPublicationFailure(t *testing.T) {
	compattest.Set(t, true)
	signedFixture(t, "accepted", []string{"chat"}, time.Now().Add(time.Hour))
	installLicensed(t, "chat")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	t.Setenv("NSELF_PING_API_URL", server.URL)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".nself"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	err := entitlement.RefreshInstalledForKey(context.Background(), dir, "accepted")
	if err == nil || !strings.Contains(err.Error(), "could not write the plugin entitlement file") {
		t.Fatalf("lost publication failure diagnostic: %v", err)
	}
}
