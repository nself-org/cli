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
	"strconv"
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

// urlParam matches one query, matrix or fragment parameter: a separator
// (?, &, ; or #), the raw name up to `=`, and the value up to the next
// separator or space.
var urlParam = regexp.MustCompile(`([?&;#])([^=&;#\s]*)=([^&;#\s]*)`)

// credentialWord matches a credential word anywhere in a decoded parameter
// name (access_token, client_secret, api-key, X-Amz-Signature, auth).
var credentialWord = regexp.MustCompile(`(?i)pass|pwd|token|secret|key|signature|auth|credential`)

// MaskURLCredentials masks the credentials of a value that holds a URL. For
// `scheme://` followed later by `@`, everything between `://` and the LAST `@`
// becomes `****` (user and password), so a password that contains `@`, `/`,
// `:`, `+` or spaces never leaks and the rule does not depend on net/url
// accepting the string. Query, matrix (`;`) and fragment (`#`) parameters
// whose name contains pass, pwd, token, secret, key, signature, auth or
// credential (any case) are masked too; the name is percent-decoded first, so
// `access%5Ftoken` and `#access_token=` are caught (D-0430). A value without
// `://` is returned unchanged. The rule over-masks a path that holds an `@`;
// that is the safe side.
func MaskURLCredentials(v string) string {
	i := strings.Index(v, "://")
	if i < 0 {
		return v
	}
	if rest := v[i+3:]; strings.Contains(rest, "@") {
		v = v[:i+3] + "****" + rest[strings.LastIndex(rest, "@"):]
	}
	return urlParam.ReplaceAllStringFunc(v, func(m string) string {
		p := urlParam.FindStringSubmatch(m)
		if !credentialWord.MatchString(decodeParamName(p[2])) {
			return m
		}
		return p[1] + p[2] + "=****"
	})
}

// pctEscape matches one valid percent escape.
var pctEscape = regexp.MustCompile(`%[0-9A-Fa-f]{2}`)

// decodeParamName percent-decodes a parameter name up to three times, so a
// double-encoded name (access%255Ftoken) is matched too. It is lenient: an
// invalid escape (%zz) stays as written and the valid ones around it still
// decode, so one bad escape cannot hide a credential word.
func decodeParamName(name string) string {
	for n := 0; n < 3 && pctEscape.MatchString(name); n++ {
		name = pctEscape.ReplaceAllStringFunc(name, func(e string) string {
			b, _ := strconv.ParseUint(e[1:], 16, 8)
			return string([]byte{byte(b)})
		})
	}
	return name
}
