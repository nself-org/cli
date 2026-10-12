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

// urlParamName matches the start of one query, matrix or fragment parameter:
// a separator (?, &, ; or #), the raw name, and the first `=` or its encoded
// form `%3D` after it. It never consumes a value, so a parameter nested in
// another parameter's value (?redirect=https://x/?token=...) is still seen.
var urlParamName = regexp.MustCompile(`([?&;#])([^=&;#]*?)(=|%3[dD])`)

// urlParamValueEnd matches what ends a parameter value: the next separator
// or whitespace (a valid URL holds no raw space; text after it is prose).
var urlParamValueEnd = regexp.MustCompile(`[&;#\s]`)

// credentialValueEnd ends a masked credential value: only `&` or whitespace,
// so an unencoded `#` or `;` inside the credential cannot leave its tail.
var credentialValueEnd = regexp.MustCompile(`[&\s]`)

// credentialWord matches a credential word anywhere in a decoded parameter
// name (access_token, client_secret, api-key, X-Amz-Signature and Azure SAS
// sig, auth, jwt, jsessionid, the OAuth code). Over-matching only masks.
var credentialWord = regexp.MustCompile(`(?i)pass|pwd|token|secret|key|sig|auth|credential|jwt|session|code`)

// MaskURLCredentials masks the credentials of a value that holds a URL. For
// `scheme://` followed later by `@`, everything between `://` and the LAST `@`
// becomes `****` (user and password), so a password that contains `@`, `/`,
// `:`, `+` or spaces never leaks and the rule does not depend on net/url
// accepting the string. Query, matrix (`;`) and fragment (`#`) parameters
// whose name contains a credential word (credentialWord, any case, spaces
// around the name allowed) are masked too; the name is percent-decoded first, so
// `access%5Ftoken=`, `access_token%3D` and `#access_token=` are caught, and
// a parameter nested in another one's value is found (D-0430). A value
// whose percent-decoded form holds a credential parameter (an encoded
// redirect such as https%3A%2F%2Fx%2F%3Ftoken%3D...) is masked whole. A
// credential value runs to the next `&` or whitespace (a credential holding
// a raw `&` keeps the part after it); a value masked whole for an encoded
// credential runs to the next `&`, `;`, `#` or whitespace. A value without
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
	var b strings.Builder
	done := 0 // v[:done] is already written
	for _, m := range urlParamName.FindAllStringSubmatchIndex(v, -1) {
		if m[0] < done {
			continue
		}
		end := len(v)
		if e := urlParamValueEnd.FindStringIndex(v[m[1]:]); e != nil {
			end = m[1] + e[0]
		}
		if credentialWord.MatchString(decodeParamName(v[m[4]:m[5]])) {
			end = len(v)
			if e := credentialValueEnd.FindStringIndex(v[m[1]:]); e != nil {
				end = m[1] + e[0]
			}
		} else if !hidesCredentialParam(v[m[1]:end], v[m[6]] == '%') {
			continue
		}
		b.WriteString(v[done:m[1]])
		b.WriteString("****")
		done = end
	}
	if done == 0 {
		return v
	}
	b.WriteString(v[done:])
	return b.String()
}

// hidesCredentialParam reports whether a parameter value, once
// percent-decoded, holds a parameter whose name is a credential word: the
// encoded redirect case, which the raw scan cannot see. Values without an
// escape are skipped (the raw scan already covers them) unless the name
// ended in an encoded `=` (encodedSep), where the raw scan cannot restart.
func hidesCredentialParam(value string, encodedSep bool) bool {
	if !encodedSep && !pctEscape.MatchString(value) {
		return false
	}
	for _, m := range urlParamName.FindAllStringSubmatch("?"+decodeParamName(value), -1) {
		if credentialWord.MatchString(m[2]) {
			return true
		}
	}
	return false
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
