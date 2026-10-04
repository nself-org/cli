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
