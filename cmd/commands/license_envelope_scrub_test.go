package commands

// Purpose: hostile-server tests of the licence envelopes (P7-SURF-16): a
// reply that carries another licence key, a JWT or a token in a field the
// command copies into a document, or in the text of a failure, reaches the
// output masked or redacted.
// Inputs: the licenseFixture ping stub (license_envelope_test.go).

import (
	"errors"
	"strings"
	"testing"

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
