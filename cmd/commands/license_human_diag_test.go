package commands

// Purpose: the human diagnostics of `license validate`, `revalidate`,
// `refresh` and `migrate` carry no licence key (D-0979).
// Inputs: the fixture key, a refused ping URL that echoes the key in a
// transport error, and an httptest stub that records the status request.
// Constraints: no network beyond httptest; human mode (no --json).

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// humanStderr runs cmd in human mode and returns what it wrote to stderr.
func humanStderr(t *testing.T, cmd *cobra.Command) (string, error) {
	t.Helper()
	cmd.SetContext(context.Background())
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)
	t.Cleanup(func() { cmd.SetErr(nil) })
	var err error
	captureStdout(t, func() error { err = cmd.RunE(cmd, nil); return nil })
	return errBuf.String(), err
}

// TestHumanDiagnosticsMaskTheKey: a transport error that echoes the request
// URL (and the key in it) reaches the person masked.
func TestHumanDiagnosticsMaskTheKey(t *testing.T) {
	newLicenseFixture(t)
	t.Setenv("NSELF_LICENSE_KEY_1", fixtureLicenseKey)
	t.Setenv("NSELF_PING_API_URL", "http://127.0.0.1:1/"+fixtureLicenseKey)

	for name, cmd := range map[string]*cobra.Command{
		"validate": licenseValidateCmd, "refresh": licenseRefreshCmd, "revalidate": licenseRevalidateCmd,
	} {
		stderr, err := humanStderr(t, cmd)
		text := stderr
		if err != nil {
			text += err.Error()
		}
		if text == "" {
			t.Errorf("license %s printed no diagnostic", name)
		}
		noSecrets(t, "license "+name+" human output", text)
	}

	migrateKeyFlag, migrateAccountID = fixtureLicenseKey, "00000000-0000-0000-0000-000000000001"
	t.Cleanup(func() { migrateKeyFlag, migrateAccountID = "", "" })
	err := runMigrateLink(context.Background(), "http://127.0.0.1:1/"+fixtureLicenseKey, &licenseMigrateData{})
	if err == nil {
		t.Fatal("migrate against a refused port must fail")
	}
	noSecrets(t, "license migrate human error", err.Error())
}

// TestMigrationStatusKeyTravelsInAHeader: the key is never in the GET URL.
func TestMigrationStatusKeyTravelsInAHeader(t *testing.T) {
	var query, header string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, header = r.URL.RawQuery, r.Header.Get("X-License-Key")
		_, _ = w.Write([]byte(`{"migration_status":"legacy"}`))
	}))
	t.Cleanup(srv.Close)
	if _, err := fetchMigrationStatus(context.Background(), fixtureLicenseKey, srv.URL); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(query, fixtureLicenseKey) {
		t.Errorf("key is in the query string: %q", query)
	}
	if header != fixtureLicenseKey {
		t.Errorf("X-License-Key = %q, want the key", header)
	}
}
