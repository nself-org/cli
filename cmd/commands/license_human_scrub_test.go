package commands

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestLicenseHumanDiagnosticsScrubbed(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
		body map[string]any
		link bool
	}{
		{"validate reason", licenseValidateCmd, map[string]any{"valid": false}, false},
		{"refresh reason", licenseRefreshCmd, map[string]any{"valid": false}, false},
		{"revalidate reason", licenseRevalidateCmd, map[string]any{"valid": false}, false},
		{"migration error", licenseMigrateCmd, map[string]any{"error": "hostile"}, false},
		{"migration status", licenseMigrateCmd, map[string]any{"migration_status": "hostile"}, false},
		{"migration link error", licenseMigrateCmd, map[string]any{"error": "hostile"}, true},
		{"migration linked", licenseMigrateCmd, map[string]any{"already_migrated": true, "account_id": "hostile", "tier": "hostile"}, true},
		{"migration completed", licenseMigrateCmd, map[string]any{"migrated": true, "account_id": "hostile", "tier": "hostile"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newLicenseFixture(t)
			t.Setenv("NSELF_LICENSE_KEY_1", fixtureLicenseKey)
			hostile := fixtureLicenseKey + " " + fx.jwt + " " + fixtureAccessToken
			for k, value := range tc.body {
				if value == "hostile" {
					tc.body[k] = hostile
				}
			}
			tc.body["reason"] = hostile
			fx.reply(t, tc.body)
			flags := map[string]string{}
			if tc.link {
				flags["account-id"] = "00000000-0000-0000-0000-000000000001"
			}
			var diagnostics bytes.Buffer
			tc.cmd.SetErr(&diagnostics)
			t.Cleanup(func() { tc.cmd.SetErr(nil) })
			var out string
			var runErr error
			stderr := captureStderr(t, func() {
				out, runErr = runLicenseJSON(t, tc.cmd, flags)
			})
			all := out + stderr + diagnostics.String()
			if runErr != nil {
				all += runErr.Error()
			}
			for _, secret := range []string{fixtureLicenseKey, fx.jwt, fixtureAccessToken} {
				if strings.Contains(all, secret) {
					t.Error("human diagnostic or JSON contains an unredacted synthetic secret")
				}
			}
			if !strings.Contains(all, "[REDACTED]") {
				t.Error("expected hostile reply to reach the scrubbed diagnostic or document")
			}
		})
	}
}

func TestMigrationStatusUsesHeader(t *testing.T) {
	for _, key := range []string{fixtureLicenseKey, "synthetic-key-with+reserved&characters"} {
		t.Run(key, func(t *testing.T) {
			requests := make(chan *http.Request, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r
				_, _ = w.Write([]byte(`{"migration_status":"legacy"}`))
			}))
			defer srv.Close()
			if _, err := fetchMigrationStatus(context.Background(), key, srv.URL); err != nil {
				t.Fatal(err)
			}
			r := <-requests
			if r.Method != http.MethodGet || r.URL.Path != "/license/migration-status" || r.URL.RawQuery != "" {
				t.Error("migration status must use GET without a query string")
			}
			if r.Header.Get("X-License-Key") != key {
				t.Error("migration status must send the key in X-License-Key")
			}
		})
	}
}
