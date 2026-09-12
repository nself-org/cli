package secrets

// secrets_validate_test.go covers ValidateSecretValue against the exact
// incident shapes it exists to catch (2026-09-11/12 vault corruption:
// a literal "<set in .env.secrets>" placeholder and an unexpanded
// "${PROD...}" shell reference), plus the general empty/short-value cases.

import (
	"strings"
	"testing"
)

func TestValidateSecretValue_RejectsIncidentShapes(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		value string
	}{
		{"literal placeholder token", "STRIPE_NSELF_SECRET_KEY", "<set in .env.secrets>"},
		{"unexpanded shell var reference", "AUTH_JWT_SECRET", "${PROD_AUTH_JWT_SECRET}"},
		{"command substitution", "POSTGRES_PASSWORD", "$(cat /run/secrets/pg)"},
		{"empty value", "HASURA_GRAPHQL_ADMIN_SECRET", ""},
		{"whitespace-only value", "HASURA_GRAPHQL_ADMIN_SECRET", "   "},
		{"bare angle bracket prefix", "REDIS_PASSWORD", "<partial"},
		{"implausibly short jwt secret", "AUTH_JWT_SECRET", "abc123"},
		{"implausibly short password", "POSTGRES_PASSWORD", "abc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := ValidateSecretValue(c.key, c.value); err == nil {
				t.Errorf("ValidateSecretValue(%q, %q) = nil, want rejection", c.key, c.value)
			}
		})
	}
}

func TestValidateSecretValue_AcceptsPlausibleValues(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		value string
	}{
		{"long random jwt secret", "AUTH_JWT_SECRET", "kR8f3nQ2vL9wZ4xM7pT1bY6cJ0hD5sA3eU8gN2rK4vW7qX1z"},
		{"reasonable password", "POSTGRES_PASSWORD", "Tr0ub4dor&3xamplePW"},
		{"reasonable api key", "STRIPE_NSELF_SECRET_KEY", "not-a-real-key-fixture-51AbCdEfGhIjKlMnOpQrStUv"},
		{"generic short-but-allowed value", "NODE_ENV", "production"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := ValidateSecretValue(c.key, c.value); err != nil {
				t.Errorf("ValidateSecretValue(%q, %q) = %v, want nil", c.key, c.value, err)
			}
		})
	}
}

func TestValidateSecretValue_ErrorNeverEchoesTheRejectedValue(t *testing.T) {
	// The error message must name the rule, never the offending value —
	// a secret that fails validation must not leak into logs/output either.
	secretValue := "${SOME_SUPER_SECRET_REFERENCE_VALUE}"
	err := ValidateSecretValue("AUTH_JWT_SECRET", secretValue)
	if err == nil {
		t.Fatal("expected rejection")
	}
	if got := err.Error(); strings.Contains(got, secretValue) {
		t.Errorf("error message echoed the rejected value: %q", got)
	}
}
