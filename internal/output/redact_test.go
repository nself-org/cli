package output

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// Fixtures are assembled at run time so no token-shaped literal sits in source.
var (
	fixturePassword = "s3" + "cretpw"
	fixtureToken    = "gh" + "p_" + strings.Repeat("a1B2", 9)
	fixtureDSN      = "postgres://app:" + fixturePassword + "@db:5432/main"
)

func secretError() error {
	return fmt.Errorf("connect %s failed (token %s) for ops@example.com", fixtureDSN, fixtureToken)
}

func assertNoSecrets(t *testing.T, label, s string) {
	t.Helper()
	for _, secret := range []string{fixturePassword, fixtureToken, "ops@example.com"} {
		if strings.Contains(s, secret) {
			t.Fatalf("%s leaks %q:\n%s", label, secret, s)
		}
	}
	if !strings.Contains(s, "[REDACTED]") {
		t.Fatalf("%s has no redaction marker:\n%s", label, s)
	}
}

func TestEnvelopeRedactsErrorText(t *testing.T) {
	reset(t)
	w, out, _ := bufs()
	if err := EmitError(w, "start", secretError()); err != nil {
		t.Fatal(err)
	}
	assertNoSecrets(t, "error envelope", out.String())
	if !strings.Contains(out.String(), "postgres://[REDACTED]@db:5432/main") {
		t.Fatalf("DSN host and path should survive: %s", out.String())
	}
}

func TestEnvelopeRedactsCauseAndRemediation(t *testing.T) {
	reset(t)
	ce := &errs.CLIError{
		Code: "E057", What: "bad " + fixtureDSN,
		Why: "token " + fixtureToken, Fix: "mail ops@example.com",
	}
	w, out, _ := bufs()
	if err := EmitError(w, "x", ce); err != nil {
		t.Fatal(err)
	}
	assertNoSecrets(t, "CLIError envelope", out.String())
}

func TestRenderRedactsErrorText(t *testing.T) {
	assertNoSecrets(t, "plain line", string(render(secretError())))
	ce := &errs.CLIError{Code: "E057", What: "bad " + fixtureDSN, Why: "token " + fixtureToken, Fix: "mail ops@example.com"}
	assertNoSecrets(t, "coded block", string(render(ce)))
}

func TestRedactionLeavesOrdinaryTextAlone(t *testing.T) {
	for _, s := range []string{"boom", "docker daemon is not running", "open /tmp/x: no such file or directory"} {
		if got := redactText(s); got != s {
			t.Errorf("redactText(%q) = %q", s, got)
		}
	}
	// Redaction does not touch the typed error value itself.
	err := secretError()
	_ = redactText(err.Error())
	if !strings.Contains(err.Error(), fixturePassword) {
		t.Fatal("original error was mutated")
	}
}

// TestRedactsKeyValueSecrets covers secrets assigned in text, which neither the
// URL scrub nor the telemetry redactor catches.
func TestRedactsKeyValueSecrets(t *testing.T) {
	v := "hunt" + "er2value"
	for _, in := range []string{
		"login failed password=" + v,
		"bad flag --password=" + v + " given",
		"HASURA_ADMIN_SECRET=" + v,
		"hasura_admin_secret=" + v + ",next",
		"token=" + v + "&x=1",
		"apikey=" + v,
		"api_key='" + v + "'",
		`body {"password": "` + v + `"}`,
		"secret=\"" + v + " with space\"",
	} {
		got := redactText(in)
		if strings.Contains(got, v) || !strings.Contains(got, "[REDACTED]") {
			t.Errorf("redactText(%q) = %q", in, got)
		}
	}
	// Prose and unrelated assignments survive.
	for _, in := range []string{"invalid token: expired", "project=demo port=5432", "the secret was rotated"} {
		if got := redactText(in); got != in {
			t.Errorf("redactText(%q) changed to %q", in, got)
		}
	}
}

// TestEnvelopeAndRenderKeepRegistryText proves static registry cause and
// remediation are not mangled (E101's fix names the 'nself_pro_' prefix) while
// runtime text in the same error is still redacted.
func TestEnvelopeAndRenderKeepRegistryText(t *testing.T) {
	reset(t)
	err := fmt.Errorf("licence check password=%s: %w", fixturePassword, errs.ErrInvalidLicenseKey)
	w, out, _ := bufs()
	if e := EmitError(w, "license", err); e != nil {
		t.Fatal(e)
	}
	human := string(render(err))
	for label, s := range map[string]string{"envelope": out.String(), "human": human} {
		if !strings.Contains(s, "'nself_pro_'") || strings.Contains(s, "start with '[REDACTED]'") {
			t.Errorf("%s mangles the registry fix text:\n%s", label, s)
		}
		if strings.Contains(s, fixturePassword) {
			t.Errorf("%s leaks the password:\n%s", label, s)
		}
	}
}
