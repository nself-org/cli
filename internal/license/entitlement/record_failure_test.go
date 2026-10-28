package entitlement_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/license/entitlement"
)

// blockRejectionStores makes both the shared record lock and the per-key marker of key
// unwritable (a directory sits at each lock and marker path), so recordVerdict(hash, true)
// fails while the record itself stays readable for key selection.
func blockRejectionStores(t *testing.T, key string) {
	t.Helper()
	dir := filepath.Dir(rejectionRecordPath(t))
	lock := filepath.Join(dir, "entitlement-rejected.lock")
	marker := filepath.Join(dir, "entitlement-rejected-"+license.HashKey(key)+".mark")
	for _, p := range []string{lock, marker} {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
}

// A rejection that cannot be remembered must surface from Refresh even when another
// key still yields a publishable token. Pins the caller guard around recordVerdict in
// revalidateKeys: without the error assignment Refresh would return nil and the
// rejected key could be revived by the next offline run.
func TestRefreshReportsRejectionThatCannotBeRecorded(t *testing.T) {
	compattest.Set(t, true)
	entry, _ := signedFixture(t, "synthetic-license-key", []string{"chat"}, time.Now().Add(time.Hour))
	installLicensed(t, "chat")
	t.Setenv("NSELF_LICENSE_KEY_1", "rejected-second-key")
	blockRejectionStores(t, "rejected-second-key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "rejected-second-key") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable) // the first key stays on its cached token
	}))
	defer server.Close()
	t.Setenv("NSELF_PING_API_URL", server.URL)
	dir := t.TempDir()
	err := entitlement.Refresh(context.Background(), dir)
	if err == nil || errs.ExitCodeFor(err) != 3 || !strings.Contains(err.Error(), "could not be recorded") {
		t.Fatalf("an unrecordable rejection must be reported: %v", err)
	}
	if data, _ := os.ReadFile(entitlement.Path(dir)); string(data) != entry.JWT {
		t.Fatal("the covered key's token was not published")
	}
}
