package secrets

// secrets_validate.go — refuses to store a value that is not a literal
// secret before it ever reaches the encrypted store.
//
// Purpose: close the exact gap behind the 2026-09-11/12 production incident,
// where an operator captured `grep | sed` output into a vault file and
// assumed the right-hand side of every `KEY=` line was a literal value. It
// often is not: `STRIPE_NSELF_SECRET_KEY=<set in .env.secrets>` is a
// placeholder that reads as valid shell but is not a secret, and
// `AUTH_JWT_SECRET=${PROD...}` is an unexpanded shell variable reference
// that resolves to empty at the point it was captured. Both were written
// over a working credential; the second also broke every line `source`
// read after it. ValidateSecretValue is the fail-closed gate: called from
// Set (and therefore from the `edit` re-save path, which calls Set for
// every parsed line) so nothing reaches saveStore without passing it.
// Inputs: the secret's key name and the candidate value, exactly as
// supplied to `nself secrets set` / `nself secrets edit`.
// Outputs: nil when the value is acceptable; a descriptive error naming
// the specific rule that failed otherwise. Never logs or wraps the value
// itself into the error message.
// Constraints: pure, no I/O, no external calls — safe to unit test without
// the `age` binary. Deliberately independent of, and stricter than,
// internal/config's placeholder-secrets validator (which only screens a
// small known-placeholder substring list at config-load time for
// non-dev environments); this one runs unconditionally for every
// environment because a corrupted vault/backup copy is exactly as
// dangerous in dev as it is in prod.
import (
	"fmt"
	"regexp"
	"strings"
)

// referenceLikePattern matches a value that is entirely a bracketed
// placeholder token, e.g. "<set in .env.secrets>" or "<CHANGE ME>".
var referenceLikePattern = regexp.MustCompile(`^<.*>$`)

// minLengthByKeySuffix mirrors the type classification generateRotationValue
// already uses for rotation, reused here as a floor: a value shorter than
// this for its key's shape is implausible for that secret type (e.g. an
// 8-char "JWT secret" is not a rotated JWT signing key, it is someone's
// typo or a truncated paste).
func minLengthByKeySuffix(key string) int {
	keyUpper := strings.ToUpper(key)
	switch {
	case strings.HasSuffix(keyUpper, "_PASSWORD") || strings.HasSuffix(keyUpper, "_PASS"):
		return 12
	case strings.Contains(keyUpper, "JWT") || strings.Contains(keyUpper, "SECRET"):
		return 24
	case strings.Contains(keyUpper, "API_KEY") || strings.Contains(keyUpper, "TOKEN") || strings.Contains(keyUpper, "_KEY"):
		return 12
	default:
		return 6
	}
}

// ValidateSecretValue rejects a candidate secret value that cannot be a
// literal, working credential. It never returns the value in its error —
// callers must not either.
func ValidateSecretValue(key, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("secret %q: value is empty — refusing to store (this would silently blank a working credential)", key)
	}
	if strings.Contains(value, "${") {
		return fmt.Errorf("secret %q: value contains an unexpanded shell variable reference (\"${...}\") — capture the literal value, not the reference", key)
	}
	if strings.Contains(value, "$(") {
		return fmt.Errorf("secret %q: value contains an unexpanded shell command substitution (\"$(...)\") — capture the literal value, not the expression", key)
	}
	if strings.Contains(value, "<") || strings.Contains(value, ">") {
		return fmt.Errorf("secret %q: value contains \"<\" or \">\" — this looks like a placeholder token (e.g. \"<set in .env.secrets>\"), not a literal secret", key)
	}
	if referenceLikePattern.MatchString(strings.TrimSpace(value)) {
		return fmt.Errorf("secret %q: value is entirely a bracketed placeholder token — not a literal secret", key)
	}
	if min := minLengthByKeySuffix(key); len(value) < min {
		return fmt.Errorf("secret %q: value is only %d characters, implausibly short for this secret type (expected at least %d) — refusing to store", key, len(value), min)
	}
	return nil
}
