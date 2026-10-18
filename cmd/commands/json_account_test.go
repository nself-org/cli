package commands

// Purpose: tests for the account fragment's v1 envelopes (P7-SURF-16): the
// coverage of the canon rows, the registry classes (interactive, stream,
// cli-only, confirm, secret), the `license account` documents and the golden
// writer. The licence documents are in license_envelope_test.go.
// Inputs: package-level commands driven in v1.5 mode with --json set, on a
// prepared (moved) command tree and an httptest account server.
// Constraints: no network beyond httptest; fixtures are obviously fake.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/auth"
	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/output"
)

// TestEnvelopeCoverageAccount: every runnable document row of the account
// canon fragment is an envelope command, and each reclassified row says why.
func TestEnvelopeCoverageAccount(t *testing.T) {
	assertFragmentEnvelopeCoverage(t, "account")
	reg, err := buildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]int{}
	for i, c := range reg.Commands {
		rows[c.Path] = i
	}
	row := func(path string) int {
		i, ok := rows["nself "+path]
		if !ok {
			t.Fatalf("no registry row for %s", path)
		}
		return i
	}
	for _, p := range []string{"license login", "license logout", "license account login", "license account logout", "license account transfer"} {
		if c := reg.Commands[row(p)]; c.Output != canon.OutputInteractive || c.JSON != canon.JSONNone {
			t.Errorf("%s = output %s json %s, want interactive/none", p, c.Output, c.JSON)
		}
	}
	if c := reg.Commands[row("license tail")]; c.Output != canon.OutputStream {
		t.Errorf("license tail output = %s, want stream", c.Output)
	}
	if c := reg.Commands[row("license export")]; c.Surface != "cli-only" || c.JSON != canon.JSONEnvelope {
		t.Errorf("license export = surface %q json %s, want cli-only/envelope", c.Surface, c.JSON)
	}
	if c := reg.Commands[row("license revoke")]; c.Confirm == nil || strings.Join(c.Confirm.Flags, ",") != "yes" {
		t.Errorf("license revoke confirm = %+v, want flag yes", c.Confirm)
	}
	for _, p := range []string{"license set", "license remove", "license clear", "license migrate"} {
		if c := reg.Commands[row(p)]; c.Confirm == nil || len(c.Confirm.Flags) != 0 {
			t.Errorf("%s confirm = %+v, want an empty consent list", p, c.Confirm)
		}
	}
	for _, p := range []string{"license set", "license add", "license remove"} {
		secret := false
		for _, a := range reg.Commands[row(p)].Args {
			secret = secret || a.Secret
		}
		if !secret {
			t.Errorf("%s has no secret arg", p)
		}
	}
	for _, p := range []string{"license migrate", "license restore"} {
		secret := false
		for _, f := range reg.Commands[row(p)].Flags {
			secret = secret || (f.Name == "key" && f.Secret)
		}
		if !secret {
			t.Errorf("%s --key is not secret", p)
		}
	}
	old, err := buildRegistry(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range old.Commands {
		if strings.HasPrefix(c.Path, "nself license") && c.JSON == canon.JSONEnvelope {
			t.Errorf("v1.4 %s reports an envelope", c.Path)
		}
	}
}

// accountServer answers the account endpoints with fixed fake data.
func accountServer(t *testing.T, sessionOK bool) {
	t.Helper()
	setAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/session":
			if !sessionOK {
				http.Error(w, "down", http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(`{"authenticated":true,"account":{"email":"alice@example.test","display_name":"Alice","tier":"","email_verified":true,"mfa_enabled":false}}`))
		case "/account/devices":
			_, _ = w.Write([]byte(`{"devices":[{"id":"dev-1","name":"laptop","os":"darwin","last_active":"2026-10-01T00:00:00Z","is_current":true}]}`))
		case "/account/licenses":
			_, _ = w.Write([]byte(`{"licenses":[{"id":"lic-1","product":"chat","tier":"plus","seats_included":1,"seats_used":0,"is_active":true,"activated_at":"","expires_at":""}]}`))
		case "/account/team":
			_, _ = w.Write([]byte(`{"members":[]}`))
		default:
			http.NotFound(w, r)
		}
	})
}

func TestAccountEnvelopes(t *testing.T) {
	compattest.Set(t, true)
	undo := prepareTreeWith(&canonTable, RootCmd, true, false)
	defer undo()
	home := isolateHome(t)
	writeTestAuthFile(t, home, &auth.AuthFile{AccessToken: fixtureAccessToken, SessionToken: fixtureAccessToken, Email: "alice@example.test", Tier: "plus", Bundles: []string{"chat"}})
	accountServer(t, true)
	for _, c := range []*struct {
		path string
		run  func() (string, error)
	}{
		{"license account status", func() (string, error) { return runLicenseJSON(t, accountStatusCmd, nil) }},
		{"license account devices", func() (string, error) { return runLicenseJSON(t, accountDevicesCmd, nil) }},
		{"license account licenses", func() (string, error) { return runLicenseJSON(t, accountLicensesCmd, nil) }},
		{"license account team", func() (string, error) { return runLicenseJSON(t, accountTeamCmd, nil) }},
	} {
		out, err := c.run()
		if err != nil {
			t.Fatalf("%s: %v", c.path, err)
		}
		noSecrets(t, c.path, out)
		doc := oneEnvelope(t, out, c.path)
		if c.path == "license account licenses" && !strings.Contains(string(doc["data"]), `"bundles": []`) {
			t.Errorf("a licence without bundles prints null, want []: %s", out)
		}
		if c.path == "license account team" && !strings.Contains(string(doc["data"]), "[]") {
			t.Errorf("empty team is not []: %s", out)
		}
	}
	// The live status carries the session facts and the default tier.
	out, _ := runLicenseJSON(t, accountStatusCmd, nil)
	var st struct {
		Data accountStatusData `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatal(err)
	}
	if st.Data.Status != "active" || st.Data.Tier != "free" || st.Data.EmailVerified == nil || !*st.Data.EmailVerified {
		t.Errorf("status = %+v", st.Data)
	}
	// A mutation flag is refused with E402 and nothing is sent.
	_, err := runLicenseJSON(t, accountDevicesCmd, map[string]string{"revoke": "dev-1"})
	expectE402(t, "devices --revoke", err)
	_, err = runLicenseJSON(t, accountLicensesCmd, map[string]string{"activate": "lic-1"})
	expectE402(t, "licenses --activate", err)
	_, err = runLicenseJSON(t, accountTeamCmd, map[string]string{"invite": "bob@example.test"})
	expectE402(t, "team --invite", err)
}

func expectE402(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "E402") && !strings.Contains(err.Error(), "does not support --json") {
		t.Errorf("%s with a mutation flag and --json: err = %v, want the E402 refusal", what, err)
	}
}

// TestAccountStatusStates: not logged in, and the server down (cached).
func TestAccountStatusStates(t *testing.T) {
	compattest.Set(t, true)
	undo := prepareTreeWith(&canonTable, RootCmd, true, false)
	defer undo()
	home := isolateHome(t)
	out, err := runLicenseJSON(t, accountStatusCmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc := oneEnvelope(t, out, "license account status")
	if !strings.Contains(string(doc["data"]), `"not-logged-in"`) {
		t.Errorf("logged-out status: %s", out)
	}
	writeTestAuthFile(t, home, &auth.AuthFile{AccessToken: fixtureAccessToken, Email: "alice@example.test", Tier: "plus"})
	accountServer(t, false)
	out, err = runLicenseJSON(t, accountStatusCmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	noSecrets(t, "cached status", out)
	doc = oneEnvelope(t, out, "license account status")
	if !strings.Contains(string(doc["data"]), `"cached"`) || !strings.Contains(string(doc["data"]), `"bundles": []`) {
		t.Errorf("cached status: %s", out)
	}
}

// TestAccountGoldenWrite regenerates the goldens of the account fragment; it
// runs only with NSELF_UPDATE_GOLDEN=1.
func TestAccountGoldenWrite(t *testing.T) {
	if os.Getenv("NSELF_UPDATE_GOLDEN") != "1" {
		t.Skip("set NSELF_UPDATE_GOLDEN=1 to rewrite the account goldens")
	}
	for path, zero := range accountEnvelopeTypes {
		var buf bytes.Buffer
		if err := output.EmitData(output.Writer{Out: &buf, Err: &bytes.Buffer{}}, path, accountGoldenSample(path, zero)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixtureFileFor(path), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// accountGoldenSample is a representative document per path; the key is the
// masked fixture key, never the key itself.
func accountGoldenSample(path string, zero any) any {
	mk := license.MaskKey(fixtureLicenseKey)
	yes := true
	switch path {
	case "license account", "license account status":
		return accountStatusData{Status: "active", Email: "alice@example.test", DisplayName: "Alice", Tier: "plus", EmailVerified: &yes, MFAEnabled: new(bool), Bundles: []string{"chat"}}
	case "license account devices":
		return []auth.DeviceEntry{{ID: "dev-1", Name: "laptop", OS: "darwin", LastActive: "2026-10-01T00:00:00Z", IsCurrent: true}}
	case "license account licenses":
		return []auth.LicenseInfo{{ID: "lic-1", Product: "chat", Tier: "plus", Bundles: []string{}, SeatsIncluded: 1, IsActive: true}}
	case "license add":
		return licenseAddData{Added: []licenseKeyRef{{Key: mk, Product: "ɳSelf Pro"}}}
	case "license set":
		return licenseSetData{Key: mk, Product: "ɳSelf Pro", Replaced: 1}
	case "license remove":
		return licenseRemoveData{Removed: 1}
	case "license status", "license list":
		return licenseStatusData{Keys: []licenseStatusKey{{Key: mk, Product: "plus", Status: "Active", Expires: "2027-01-02", Plugins: []string{"chat"}}},
			Products: []string{"plus"}, Plugins: []string{"chat"}, PluginAccess: &licensePluginAccess{Plugin: "chat", Granted: true}}
	case "license show":
		days := 7
		return licenseShowData{Configured: true, KeyPrefix: "...1234", Tier: "ɳSelf Pro", Bundles: []string{"chat"}, PluginsCount: 1, DaysToExpire: &days, CacheExpiry: "2026-10-25"}
	case "license validate":
		return licenseValidateData{Keys: []string{mk}}
	case "license upgrade":
		return licenseUpgradeData{URL: pricingURL}
	case "license revalidate":
		return licenseRevalidateData{Tier: "plus", ExpiresAt: "2027-01-02"}
	case "license revoke":
		return licenseRevokeData{Revoked: true}
	case "license restore":
		return licenseRestoreData{Restored: true, Product: "ɳSelf Pro"}
	case "license migrate":
		return licenseMigrateData{Mode: "status", Keys: []licenseMigrateKey{{Key: mk, Product: "Pro", Remaining: "12 days", Status: "legacy"}}}
	case "license refresh":
		return licenseRefreshData{Mode: "keys", Keys: []licenseRefreshKey{{Key: mk, Refreshed: true, Tier: "plus", ExpiresAt: "2027-01-02"}}}
	case "license export":
		return licenseExportData{Tier: "plus", PluginsAllowed: []string{"chat"}, FetchedAt: 1790000000, ExpiresAt: 1790600000, Signed: true}
	case "license simulate-offline":
		return licenseSimulateData{Days: 5, State: "soft-warning", CanProceed: true, WriteAllowed: true, Message: "offline for 5 days", Banner: "license check overdue"}
	case "license health":
		return licenseHealthJSON{FormatOK: true, ServerReachable: true, ServerValid: true, CacheIntegrity: true, HealthScore: 3, Status: "healthy"}
	}
	return goldenSample(path, zero)
}
