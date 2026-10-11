package config

// Purpose: the one rule for "this config key names a secret" and the one rule
// for masking credentials inside a URL value, shared by the CLI masking
// helpers, internal/env and the machine audit log (P7-SURF-25, P7-SURF-12).
// Inputs: a config key or flag name; a config value.
// Outputs: true when the name contains SECRET, PASS (which covers PASSWORD and
// AUTH_SMTP_PASS), KEY, TOKEN, CREDENTIAL, WEBHOOK, PRIVATE, DSN or SALT
// (case-insensitive); a value with its URL credentials masked.
// Constraints: over-matching (for example PUBLIC_KEY or BYPASS) is the safe
// side: a match only redacts. internal/env must not keep a second list.

import (
	"regexp"
	"strings"
)

// secretKeyParts are the substrings whose presence in an upper-cased key name
// marks a secret value.
var secretKeyParts = []string{"SECRET", "PASS", "KEY", "TOKEN", "CREDENTIAL", "WEBHOOK", "PRIVATE", "DSN", "SALT"}

// IsSecretKey reports whether key names a secret value.
func IsSecretKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, part := range secretKeyParts {
		if strings.Contains(upper, part) {
			return true
		}
	}
	return false
}

// urlSecretParam matches a query parameter whose name carries a credential
// word anywhere (access_token, client_secret, api-key, X-Amz-Signature, auth).
var urlSecretParam = regexp.MustCompile(`(?i)([?&;][A-Za-z0-9_.-]*(?:pass|pwd|token|secret|key|signature|auth|credential)[A-Za-z0-9_.-]*=)[^&;#\s]*`)

// MaskURLCredentials masks the credentials of a value that holds a URL. For
// `scheme://` followed later by `@`, everything between `://` and the LAST `@`
// becomes `****` (user and password), so a password that contains `@`, `/`,
// `:`, `+` or spaces never leaks and the rule does not depend on net/url
// accepting the string. Query parameters whose name contains pass, pwd, token,
// secret, key, signature, auth or credential (any case) are masked too. A
// value without `://` is returned unchanged. The rule over-masks a path that
// holds an `@`; that is the safe side.
func MaskURLCredentials(v string) string {
	i := strings.Index(v, "://")
	if i < 0 {
		return v
	}
	if rest := v[i+3:]; strings.Contains(rest, "@") {
		v = v[:i+3] + "****" + rest[strings.LastIndex(rest, "@"):]
	}
	return urlSecretParam.ReplaceAllString(v, "${1}****")
}
