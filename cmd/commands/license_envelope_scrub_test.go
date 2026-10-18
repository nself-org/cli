package commands

// Purpose: hostile-server tests of the licence envelopes (P7-SURF-16): a
// reply that carries another licence key, a JWT or a token in a field the
// command copies into a document, or in the text of a failure, reaches the
// output masked or redacted.
// Inputs: the licenseFixture ping stub (license_envelope_test.go).

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/auth"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
)

const hostileOtherKey = "nself_plus_zyxwvutsrqponmlkjihgfedcba9876"

// TestLicenseHostileFieldsAreScrubbed: tier, plugin names and expiry from an
// accepted reply are scrubbed in the status document.
func TestLicenseHostileFieldsAreScrubbed(t *testing.T) {
	fx := newLicenseFixture(t)
	t.Setenv("NSELF_LICENSE_KEY_1", fixtureLicenseKey)
	hostile := hostileOtherKey + " " + fx.jwt + " " + fixtureAccessToken
	fx.reply(t, map[string]any{"valid": true, "tier": hostile, "plugins": []string{hostile}, "plugins_allowed": []string{hostile},
		"expires": "2027-01-02 " + hostileOtherKey})
	out, err := runLicenseJSON(t, licenseStatusCmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	noSecrets(t, "license status", out, fx.jwt, hostileOtherKey)
	oneEnvelope(t, out, "license status")
}

// TestLicenseFailureTextIsScrubbed: the text of a failed command in envelope
// mode carries no key, and the original error stays in the chain.
func TestLicenseFailureTextIsScrubbed(t *testing.T) {
	fx := newLicenseFixture(t)
	t.Setenv("NSELF_LICENSE_KEY_1", fixtureLicenseKey)
	fx.reply(t, map[string]any{"valid": false, "reason": "revoked " + hostileOtherKey + " opaque0123456789abcdef0123456789"})
	_, err := runLicenseJSON(t, licenseRevalidateCmd, nil)
	if err == nil {
		t.Fatal("revalidate of an invalid licence should fail")
	}
	noSecrets(t, "license revalidate error", err.Error(), hostileOtherKey, "opaque0123456789abcdef0123456789")
	if !strings.Contains(err.Error(), license.MaskKey(hostileOtherKey)) {
		t.Errorf("error text lost the masked key: %v", err)
	}
	var s *scrubbedError
	if !errors.As(err, &s) || errors.Unwrap(err) == nil {
		t.Errorf("scrubbed error does not keep the original in its chain: %T", err)
	}
}

// TestLicenseMigrateLinkKeepsAccountID: the account UUID of a link reply is
// plain data and must not be redacted as a token; a key in it still is.
func TestLicenseMigrateLinkKeepsAccountID(t *testing.T) {
	fx := newLicenseFixture(t)
	t.Setenv("NSELF_LICENSE_KEY_1", fixtureLicenseKey)
	const uuid = "0b1c2d3e-4f50-4a6b-8c7d-9e0f1a2b3c4d"
	fx.reply(t, map[string]any{"migrated": true, "account_id": uuid, "tier": "pro"})
	out, err := runLicenseJSON(t, licenseMigrateCmd, map[string]string{"account-id": uuid})
	if err != nil {
		t.Fatal(err)
	}
	noSecrets(t, "license migrate link", out, fx.jwt)
	oneEnvelope(t, out, "license migrate")
	if !strings.Contains(out, uuid) {
		t.Errorf("account id was redacted:\n%s", out)
	}
}

const hostileToken = "opaque0123456789abcdef0123456789"

// TestScrubDocWalksEveryLeaf: strings inside pointers, slices, maps and
// structs are scrubbed; a UUID, a date, a masked key and a unit name pass;
// nil and empty slices keep their shape.
func TestScrubDocWalksEveryLeaf(t *testing.T) {
	type leaf struct {
		Name string
		Tags []string
	}
	type doc struct {
		ID      string
		When    string
		Masked  string
		Unit    string
		Hostile *string
		Rows    []leaf
		ByName  map[string]string
		Nil     []string
		Empty   []string
	}
	bad := "x " + hostileOtherKey + " " + hostileToken + " opaqueSessionTokenWithoutDigitsXYZ 2026-10-18opaque0123456789abcdef"
	in := doc{ID: "0b1c2d3e-4f50-4a6b-8c7d-9e0f1a2b3c4d", When: "2026-10-18T13:00:00.123456789+00:00",
		Masked: license.MaskKey(hostileOtherKey), Unit: "nself-license-refresh.timer", Hostile: &bad,
		Rows: []leaf{{Name: bad, Tags: []string{bad}}}, ByName: map[string]string{"k": bad}, Empty: []string{}}
	out := scrubDoc(in).(doc)
	if out.ID != in.ID || out.When != in.When || out.Masked != in.Masked || out.Unit != in.Unit {
		t.Errorf("plain data changed: %+v", out)
	}
	for _, got := range []string{*out.Hostile, out.Rows[0].Name, out.Rows[0].Tags[0], out.ByName["k"]} {
		noSecrets(t, "scrubDoc", got, hostileOtherKey, hostileToken, "opaqueSessionTokenWithoutDigitsXYZ", "2026-10-18opaque0123456789abcdef")
	}
	if out.Nil != nil || out.Empty == nil || *in.Hostile != bad {
		t.Errorf("shape changed: nil=%v empty=%v input=%q", out.Nil, out.Empty, *in.Hostile)
	}
}

// TestScrubErrKeepsClassification: the final error detail carries scrubbed
// What/Why/Fix and the same code and exit status as the original.
func TestScrubErrKeepsClassification(t *testing.T) {
	bad := hostileOtherKey + " " + hostileToken
	orig := &errs.CLIError{Code: "E402", What: "what " + bad, Why: "why " + bad, Fix: "fix " + bad}
	got := errs.Describe(scrubErr(orig))
	want := errs.Describe(orig)
	if got.Code != want.Code || got.ExitCode != want.ExitCode {
		t.Errorf("classification changed: %+v vs %+v", got, want)
	}
	noSecrets(t, "error detail", got.Message+got.Cause+got.Remediation, hostileOtherKey, hostileToken)
}

// TestAccountHostileReplyIsScrubbed: a failing account call and an accepted
// reply with secrets in tier and bundles reach the output masked.
func TestAccountHostileReplyIsScrubbed(t *testing.T) {
	compattest.Set(t, true)
	undo := prepareTreeWith(&canonTable, RootCmd, true, false)
	defer undo()
	home := isolateHome(t)
	writeTestAuthFile(t, home, &auth.AuthFile{AccessToken: fixtureAccessToken, SessionToken: fixtureAccessToken, Email: "alice@example.test", Tier: "plus"})
	bad := hostileOtherKey + " " + hostileToken
	setAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account/licenses":
			_, _ = w.Write([]byte(`{"licenses":[{"id":"lic-1","product":"chat","tier":"` + bad + `","bundles":["` + bad + `"]}]}`))
		default:
			http.Error(w, "boom "+bad, http.StatusInternalServerError)
		}
	})
	out, err := runLicenseJSON(t, accountLicensesCmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	noSecrets(t, "account licenses", out, hostileOtherKey, hostileToken)
	_, err = runLicenseJSON(t, accountDevicesCmd, nil)
	if err == nil {
		t.Fatal("devices against a failing server should fail")
	}
	noSecrets(t, "account devices error", err.Error(), hostileOtherKey, hostileToken)
}

// TestLicenseStatusHeaderAndPluginFlag: a hostile Retry-After header and a
// --plugin value that is a configured key reach the document masked.
func TestLicenseStatusHeaderAndPluginFlag(t *testing.T) {
	fx := newLicenseFixture(t)
	t.Setenv("NSELF_LICENSE_KEY_1", fixtureLicenseKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", hostileOtherKey+hostileToken)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("NSELF_PING_API_URL", srv.URL)
	out, err := runLicenseJSON(t, licenseStatusCmd, map[string]string{"plugin": fixtureLicenseKey})
	if err != nil {
		t.Fatal(err)
	}
	noSecrets(t, "license status", out, fx.jwt, hostileOtherKey, hostileToken)
	oneEnvelope(t, out, "license status")
}

// TestLicenseImportTargetIsHidden: the import path can carry a key.
func TestLicenseImportTargetIsHidden(t *testing.T) {
	if !dataTargetSecret["license import"] {
		t.Error("license import echoes its path argument into the document")
	}
}

// TestAccountParentFailureIsScrubbed: the production `license account` parent
// (the status default) scrubs a real auth-file read failure whose text carries
// the home path, and so a key that sits in it.
func TestAccountParentFailureIsScrubbed(t *testing.T) {
	compattest.Set(t, true)
	undo := prepareTreeWith(&canonTable, RootCmd, true, false)
	defer undo()
	home := filepath.Join(t.TempDir(), hostileOtherKey)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// auth.json is a directory: loading it fails with a text holding its path.
	if err := os.MkdirAll(filepath.Join(home, ".nself", "auth.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := runLicenseJSON(t, accountCmd, nil)
	if err == nil {
		t.Fatal("want a failure")
	}
	noSecrets(t, "license account error", err.Error(), hostileOtherKey)
	if !strings.Contains(err.Error(), license.MaskKey(hostileOtherKey)) {
		t.Errorf("the failure no longer carries the path (test would pass for the wrong reason): %q", err.Error())
	}
}

// TestUsageErrorMasksLicenceKey: a licence key typed as the value of any flag
// (here the bool --yes, which is not secret-named) is masked in the E401 text
// that reaches the error envelope, for the parser's own wording and for a
// shorthand tail.
func TestUsageErrorMasksLicenceKey(t *testing.T) {
	compattest.Set(t, true)
	for _, msg := range []string{
		`invalid argument "` + hostileOtherKey + `" for "--yes" flag: strconv.ParseBool: parsing "` + hostileOtherKey + `": invalid syntax`,
		`unknown flag: --` + hostileOtherKey,
	} {
		err := codedUsageError(errors.New(msg))
		if err == nil {
			t.Fatalf("no error for %q", msg)
		}
		noSecrets(t, "usage error", err.Error(), hostileOtherKey)
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E401" {
			t.Errorf("want E401, got %#v", err)
		}
	}
}

// TestUsageErrorKeepsLongFlagNames: human usage text is not rewritten by the
// opaque-token rule (a long camelCase flag name is not a token).
func TestUsageErrorKeepsLongFlagNames(t *testing.T) {
	compattest.Set(t, true)
	err := codedUsageError(errors.New("unknown flag: --someVeryLongCamelCaseOption"))
	if err == nil || !strings.Contains(err.Error(), "--someVeryLongCamelCaseOption") {
		t.Errorf("flag name rewritten: %v", err)
	}
}

// TestScrubKeepsIdentityText: emails, device names and hostnames are not
// tokens; the scrubber must leave them readable (a device id is shown to the
// user so that `devices --revoke <id>` works).
func TestScrubKeepsIdentityText(t *testing.T) {
	sc := newScrubber()
	for _, keep := range []string{
		"johnathan.richardson1985@example.com",
		"Alis-MacBook-Pro-14-inch.local",
		"ping-api-staging-1.nself.org",
		"dev-0b1c2d3e-4f50-4a6b-8c7d-9e0f1a2b3c4d",
		"nself-license-refresh.timer",
	} {
		if got := sc.str(keep); got != keep {
			t.Errorf("legitimate text rewritten: %q -> %q", keep, got)
		}
	}
	for _, bad := range []string{hostileToken, "tok-surf16-access-0123456789abcdef", "opaqueSessionTokenWithoutDigitsXYZ"} {
		if got := sc.str("x " + bad); strings.Contains(got, bad) {
			t.Errorf("token survived: %q", got)
		}
	}
}

// TestScrubErrKeepsExitStatus: an explicit exit status wrapping a coded error
// is still the exit status after the text is scrubbed.
func TestScrubErrKeepsExitStatus(t *testing.T) {
	in := &errs.ExitError{Code: 7, Err: errs.New("E401", "bad "+hostileOtherKey)}
	out := scrubErr(in)
	var xe *errs.ExitError
	if !errors.As(out, &xe) || xe.Code != 7 {
		t.Fatalf("exit status lost: %#v", out)
	}
	noSecrets(t, "scrubbed exit error", out.Error(), hostileOtherKey)
	var ce *errs.CLIError
	if !errors.As(out, &ce) || ce.Code != "E401" {
		t.Errorf("code lost: %#v", out)
	}
}

// TestScrubRedactsHeldTokensAndSegmentedTokens: the tokens in the auth file are
// redacted whatever their shape, and a base64url token split into short mixed
// segments is not mistaken for a device name.
func TestScrubRedactsHeldTokensAndSegmentedTokens(t *testing.T) {
	home := isolateHome(t)
	const held = "plain-held-session"
	writeTestAuthFile(t, home, &auth.AuthFile{AccessToken: held + "-access", SessionToken: held})
	const segmented = "aB3dE5fG7h_Jk2Lm4No6P_qR8sT0uV1w_Xy2Za4Bc6D"
	got := newScrubber().str("a " + held + "-access b " + held + " c " + segmented)
	for _, secret := range []string{held, segmented} {
		if strings.Contains(got, secret) {
			t.Errorf("secret survived: %q", got)
		}
	}
	if keep := "Alis-MacBook-Pro-14-inch"; newScrubber().str(keep) != keep {
		t.Error("device name rewritten")
	}
}
