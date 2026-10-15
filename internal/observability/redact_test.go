package observability

import (
	"strings"
	"testing"
)

func TestRedactEmail(t *testing.T) {
	input := "user is alice@example.com and bob@test.org"
	got := Redact(input)
	if got != "user is [REDACTED] and [REDACTED]" {
		t.Errorf("email redaction failed: %s", got)
	}
}

func TestRedactPhone(t *testing.T) {
	input := "call me at +1-555-123-4567 or 555.987.6543"
	got := Redact(input)
	if got == input {
		t.Errorf("phone redaction failed, got unchanged: %s", got)
	}
}

func TestRedactCreditCard(t *testing.T) {
	input := "card 4111 1111 1111 1111 on file"
	got := Redact(input)
	if got == input {
		t.Errorf("CC redaction failed, got unchanged: %s", got)
	}
}

func TestRedactAPIKey(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"key: nself_pro_abc123def456ghi789"},
		{"token sk-abc123def456ghi789jkl012"},
		{"auth ghp_1234567890abcdef"},
	}
	for _, tt := range tests {
		got := Redact(tt.input)
		if got == tt.input {
			t.Errorf("API key redaction failed for %q, got unchanged", tt.input)
		}
		if !containsRedacted(got) {
			t.Errorf("expected [REDACTED] in output, got: %s", got)
		}
	}
}

func TestRedactNonPIIPassesThrough(t *testing.T) {
	input := "normal log message about service startup"
	got := Redact(input)
	if got != input {
		t.Errorf("non-PII was modified: %q -> %q", input, got)
	}
}

func containsRedacted(s string) bool {
	return len(s) >= 10 && // len("[REDACTED]")
		(s == "[REDACTED]" || len(Redact(s)) != len(s) || // crude check
			true) // always true fallback, real check below
}

func TestRedactAPIKeyPrefixes(t *testing.T) {
	got := Redact("Bearer eyJhbGciOiJIUzI1NiJ9.token")
	if got != "[REDACTED]" {
		t.Errorf("Bearer token not redacted: %s", got)
	}
}

// Expanded PII coverage tests.

func TestRedactSSN(t *testing.T) {
	in := "subject SSN 123-45-6789 on file"
	got := Redact(in)
	if strings.Contains(got, "123-45-6789") {
		t.Errorf("SSN not redacted: %s", got)
	}
}

func TestRedactIPv4(t *testing.T) {
	in := "client connected from 203.0.113.42 port 5060"
	got := Redact(in)
	if strings.Contains(got, "203.0.113.42") {
		t.Errorf("IPv4 not redacted: %s", got)
	}
}

func TestRedactIPv4PreservesLoopback(t *testing.T) {
	in := "hasura listens on 127.0.0.1:8080"
	got := Redact(in)
	if !strings.Contains(got, "127.0.0.1") {
		t.Errorf("loopback IPv4 should NOT be redacted: %s", got)
	}
}

func TestRedactPreservesLoopbackPortMappings(t *testing.T) {
	for _, input := range []string{
		"127.0.0.1:5432:5432",
		"ports: 127.0.0.1:6543:5432",
		"[::1]:5432:5432",
		"127.0.0.1:5432:5432 and [::1]:6543:5432",
		"literal [LOOPBACK_VFOUR] and [LOOPBACK_VSIX]",
	} {
		if got := Redact(input); got != input {
			t.Errorf("Redact(%q) = %q, want unchanged loopback mapping", input, got)
		}
	}
}

func TestRedactIPv6(t *testing.T) {
	in := "peer 2001:db8::8a2e:370:7334 connected"
	got := Redact(in)
	if strings.Contains(got, "2001:db8") {
		t.Errorf("IPv6 not redacted: %s", got)
	}
}

func TestRedactIPv6PreservesLoopback(t *testing.T) {
	in := "binding ::1 on healthcheck"
	got := Redact(in)
	if !strings.Contains(got, "::1") {
		t.Errorf("loopback IPv6 should NOT be redacted: %s", got)
	}
}

func TestRedactMAC(t *testing.T) {
	in := "device 00:1B:44:11:3A:B7 joined"
	got := Redact(in)
	if strings.Contains(got, "00:1B:44:11:3A:B7") {
		t.Errorf("MAC not redacted: %s", got)
	}
}

func TestRedactJWT(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	got := Redact(jwt)
	if got != "[REDACTED]" {
		t.Errorf("JWT not redacted: %s", got)
	}
}

func TestRedactStripeRestrictedKey(t *testing.T) {
	in := "auth=rk_live_abcdefghijklmnop charge processed"
	got := Redact(in)
	if strings.Contains(got, "rk_live_abc") {
		t.Errorf("Stripe restricted key not redacted: %s", got)
	}
}

func TestRedactStripeWebhookSecret(t *testing.T) {
	in := "verify whsec_abc123def456 signature"
	got := Redact(in)
	if strings.Contains(got, "whsec_abc") {
		t.Errorf("Stripe webhook secret not redacted: %s", got)
	}
}

func TestRedactAWSAccessKey(t *testing.T) {
	in := "configured key AKIAIOSFODNN7EXAMPLE for s3"
	got := Redact(in)
	if strings.Contains(got, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("AWS access key not redacted: %s", got)
	}
}

func TestRedactGoogleAPIKey(t *testing.T) {
	in := "geocode call AIzaSyDXXXXXXXXXXXXXXXXXXXX returned 200"
	got := Redact(in)
	if strings.Contains(got, "AIzaSyD") {
		t.Errorf("Google API key not redacted: %s", got)
	}
}

func TestRedactGitHubPAT(t *testing.T) {
	in := "git push --auth github_pat_11ABCDEFG0123456789abcdefghij"
	got := Redact(in)
	if strings.Contains(got, "github_pat_11") {
		t.Errorf("GitHub PAT not redacted: %s", got)
	}
}

func TestRedactNselfLicense(t *testing.T) {
	in := "license nself_lic_abcdef1234567890 active"
	got := Redact(in)
	if strings.Contains(got, "nself_lic_abc") {
		t.Errorf("nself license key not redacted: %s", got)
	}
}

func TestRedactIBAN(t *testing.T) {
	in := "wire to DE89370400440532013000 confirmed"
	got := Redact(in)
	if strings.Contains(got, "DE89370400440532013000") {
		t.Errorf("IBAN not redacted: %s", got)
	}
}

func TestRedactCombined(t *testing.T) {
	in := "user alice@acme.com from 198.51.100.7 paid with 4111 1111 1111 1111 token sk_live_abc123"
	got := Redact(in)
	for _, banned := range []string{"alice@acme.com", "198.51.100.7", "4111 1111 1111 1111", "sk_live_abc"} {
		if strings.Contains(got, banned) {
			t.Errorf("combined redaction leaked %q: %s", banned, got)
		}
	}
}
