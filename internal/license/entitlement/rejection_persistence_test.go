package entitlement_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/license/entitlement"
)

// D-1110: a 401 in one run must still withhold the cached token from a later offline run.
func TestRefreshServerRejectionSurvivesOfflineRefresh(t *testing.T) {
	compattest.Set(t, true)
	entry, _ := signedFixture(t, "synthetic-license-key", []string{"chat"}, time.Now().Add(time.Hour))
	installLicensed(t, "chat")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Setenv("NSELF_PING_API_URL", server.URL)
	dir := t.TempDir()
	if err := entitlement.Refresh(context.Background(), dir); errs.ExitCodeFor(err) != 3 {
		t.Fatalf("online rejection must fail closed: %v", err)
	}
	server.Close()
	t.Setenv("NSELF_PING_API_URL", "http://127.0.0.1:1")
	if err := entitlement.Refresh(context.Background(), dir); errs.ExitCodeFor(err) != 3 {
		t.Fatalf("offline refresh must stay fail-closed after a server rejection: %v", err)
	}
	if data, _ := os.ReadFile(entitlement.Path(dir)); len(data) != 0 || string(data) == entry.JWT {
		t.Fatal("offline refresh republished the cached token of a rejected key")
	}
}

// A later online acceptance of the same key lifts the persisted rejection.
func TestRefreshOnlineAcceptanceClearsPersistedRejection(t *testing.T) {
	compattest.Set(t, true)
	entry, _ := signedFixture(t, "synthetic-license-key", []string{"chat"}, time.Now().Add(time.Hour))
	installLicensed(t, "chat")
	reject := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reject {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("X-NSelf-License-Sig", entry.BodySig)
		_, _ = w.Write([]byte(entry.RawBody))
	}))
	defer server.Close()
	t.Setenv("NSELF_PING_API_URL", server.URL)
	dir := t.TempDir()
	if err := entitlement.Refresh(context.Background(), dir); errs.ExitCodeFor(err) != 3 {
		t.Fatalf("rejection expected: %v", err)
	}
	reject = false
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatalf("accepted key must publish again: %v", err)
	}
	t.Setenv("NSELF_PING_API_URL", "http://127.0.0.1:1")
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatalf("offline refresh after acceptance: %v", err)
	}
	if data, _ := os.ReadFile(entitlement.Path(dir)); string(data) != entry.JWT {
		t.Fatal("rejection was not cleared by the online acceptance")
	}
}

// D-1111: a candidate whose subject is on the persistent revocation list is not selected.
func TestRefreshSkipsSubjectOnRevocationList(t *testing.T) {
	compattest.Set(t, true)
	entry, _ := signedFixture(t, "synthetic-license-key", []string{"chat"}, time.Now().Add(time.Hour))
	installLicensed(t, "chat")
	t.Setenv("LICENSE_REVOCATION_CACHE_PATH", filepath.Join(t.TempDir(), "revocation.json"))
	dir := t.TempDir()
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatalf("unrevoked baseline: %v", err)
	}
	if data, _ := os.ReadFile(entitlement.Path(dir)); string(data) != entry.JWT {
		t.Fatal("baseline did not publish the cached token")
	}
	err := license.WriteRevocationCache(&license.RevocationCache{
		List:      license.RevocationList{Revoked: []license.RevocationEntry{{Type: "key_hash", ID: license.HashKey("synthetic-license-key")}}},
		FetchedAt: time.Now().Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := entitlement.Refresh(context.Background(), dir); errs.ExitCodeFor(err) != 3 {
		t.Fatalf("revoked subject must fail closed: %v", err)
	}
	if data, _ := os.ReadFile(entitlement.Path(dir)); len(data) != 0 {
		t.Fatal("revoked subject's token (cache or retained file) was republished")
	}
}

func rejectionRecordPath(t *testing.T) string {
	t.Helper()
	cache, err := license.CachePath()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(filepath.Dir(cache), "entitlement-rejected.json")
}

// An unreadable or corrupt rejection record is unknown state, not "nothing rejected".
func TestRefreshCorruptRejectionRecordFailsClosed(t *testing.T) {
	compattest.Set(t, true)
	entry, _ := signedFixture(t, "synthetic-license-key", []string{"chat"}, time.Now().Add(time.Hour))
	installLicensed(t, "chat")
	dir := t.TempDir()
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if data, _ := os.ReadFile(entitlement.Path(dir)); string(data) != entry.JWT {
		t.Fatal("baseline did not publish the cached token")
	}
	if err := os.WriteFile(rejectionRecordPath(t), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := entitlement.Refresh(context.Background(), dir); errs.ExitCodeFor(err) != 3 {
		t.Fatalf("corrupt rejection record must fail closed: %v", err)
	}
	if data, _ := os.ReadFile(entitlement.Path(dir)); len(data) != 0 {
		t.Fatal("a token was published while the rejection record was corrupt")
	}
}

// A rejection that cannot be written must not let the next offline run republish the key.
func TestRefreshUnwritableRejectionRecordStaysFailClosed(t *testing.T) {
	compattest.Set(t, true)
	entry, _ := signedFixture(t, "synthetic-license-key", []string{"chat"}, time.Now().Add(time.Hour))
	installLicensed(t, "chat")
	// A directory at the record path makes it neither readable nor replaceable, as root or not.
	if err := os.MkdirAll(rejectionRecordPath(t), 0700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Setenv("NSELF_PING_API_URL", server.URL)
	dir := t.TempDir()
	if err := entitlement.Refresh(context.Background(), dir); errs.ExitCodeFor(err) != 3 {
		t.Fatalf("online rejection must fail closed: %v", err)
	}
	server.Close()
	t.Setenv("NSELF_PING_API_URL", "http://127.0.0.1:1")
	if err := entitlement.Refresh(context.Background(), dir); errs.ExitCodeFor(err) != 3 {
		t.Fatalf("offline refresh must stay fail-closed: %v", err)
	}
	if data, _ := os.ReadFile(entitlement.Path(dir)); len(data) != 0 || string(data) == entry.JWT {
		t.Fatal("offline refresh republished the cached token of a key whose rejection could not be recorded")
	}
}
