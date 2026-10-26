package config

// Purpose: the one rule for "this config key names a secret" and the one rule
// for masking credentials inside a URL value, shared by the CLI masking
// helpers, internal/env and the machine audit log (P7-SURF-25, P7-SURF-12).
// Inputs: a config key or flag name; a config value.
// Outputs: true when the name contains SECRET, PASS (which covers PASSWORD and
// AUTH_SMTP_PASS), KEY, TOKEN, CREDENTIAL, WEBHOOK, PRIVATE, DSN, SALT or BASICAUTH,
// or is AUTH or ends in _AUTH (case-insensitive); a value with its URL credentials masked.
// Constraints: over-matching (for example PUBLIC_KEY or BYPASS) is the safe
// side: a match only redacts. internal/env must not keep a second list.

import (
	"strings"

	"github.com/nself-org/cli/internal/credmask"
)

// secretKeyParts are the substrings whose presence in an upper-cased key name
// marks a secret value.
var secretKeyParts = []string{"SECRET", "PASS", "KEY", "TOKEN", "CREDENTIAL", "WEBHOOK", "PRIVATE", "DSN", "SALT", "BASICAUTH"}

// IsSecretKey reports whether key names a secret value.
func IsSecretKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, part := range secretKeyParts {
		if strings.Contains(upper, part) {
			return true
		}
	}
	// A bare AUTH or a _AUTH suffix (BASIC_AUTH=user:pass, REDISCLI_AUTH) is
	// secret; AUTH_*, HASURA_AUTH_URL and AUTH_JWT_* config keys stay visible.
	return upper == "AUTH" || strings.HasSuffix(upper, "_AUTH")
}

// MaskURLCredentials masks the credentials of a value that holds a URL, a Go
// DSN, a bare user:pass@host or password=/pwd= pairs. The rules and their
// bounds are documented on credmask.URL.
func MaskURLCredentials(v string) string { return credmask.URL(v) }
