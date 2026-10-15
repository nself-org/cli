package config

// Purpose: the one rule for "this config key names a secret" and the one rule
// for masking credentials inside a URL value, shared by the CLI masking
// helpers, internal/env and the machine audit log (P7-SURF-25, P7-SURF-12).
// Inputs: a config key or flag name; a config value.
// Outputs: true when the name contains SECRET, PASS (which covers PASSWORD and
// AUTH_SMTP_PASS), KEY, TOKEN, CREDENTIAL, WEBHOOK, PRIVATE, DSN, SALT or BASICAUTH
// (case-insensitive); a value with its URL credentials masked.
// Constraints: over-matching (for example PUBLIC_KEY or BYPASS) is the safe
// side: a match only redacts. internal/env must not keep a second list.

import (
	"regexp"
	"strings"
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
	return false
}

// urlParam is one query, matrix or fragment parameter found by urlParams:
// the name is v[nameStart:nameEnd]; the value starts at valueStart, after
// the `=` or its encoded form `%3D` (encodedEq).
type urlParam struct {
	start, nameStart, nameEnd, valueStart int
	encodedEq                             bool
}

// urlParams finds every parameter of v: a separator (?, &, ; or #), the
// name (any bytes but &, ; and #; spaces and ? allowed) and the first `=` or
// `%3D` after it. It never consumes a value, so a parameter nested in another
// parameter's value (?redirect=https://x/?token=...) is still seen. Two
// reverse-built tables make it linear in len(v) on any input.
func urlParams(v string) []urlParam {
	n := len(v)
	nextEq, nextStop := make([]int, n+1), make([]int, n+1)
	nextEq[n], nextStop[n] = n, n
	for k := n - 1; k >= 0; k-- {
		nextEq[k], nextStop[k] = nextEq[k+1], nextStop[k+1]
		switch {
		case v[k] == '=' || (v[k] == '%' && k+2 < n && v[k+1] == '3' && (v[k+2] == 'D' || v[k+2] == 'd')):
			nextEq[k] = k
		case v[k] == '&' || v[k] == ';' || v[k] == '#':
			nextStop[k] = k
		}
	}
	var out []urlParam
	for k := 0; k < n; k++ {
		if c := v[k]; c != '?' && c != '&' && c != ';' && c != '#' {
			continue
		}
		eq := nextEq[k+1]
		if eq >= n || nextStop[k+1] < eq {
			continue
		}
		p := urlParam{start: k, nameStart: k + 1, nameEnd: eq, valueStart: eq + 1}
		if v[eq] == '%' {
			p.encodedEq, p.valueStart = true, eq+3
		}
		out = append(out, p)
		k = p.valueStart - 1
	}
	return out
}

// credentialWord matches a credential word anywhere in a decoded parameter
// name (access_token, client_secret, api-key, X-Amz-Signature, auth, jwt,
// jsessionid). Over-matching only masks.
var credentialWord = regexp.MustCompile(`(?i)pass|pwd|token|secret|key|signature|auth|credential|jwt|session`)

// credentialShortName matches the short credential names that would mask
// ordinary words as substrings (signup, design, country_code), so they count
// only as the whole name: Azure SAS sig, the OAuth code, pw.
var credentialShortName = regexp.MustCompile(`(?i)^\s*(sig|code|pw)\s*$`)

// isCredentialName reports whether a decoded parameter name names a credential.
func isCredentialName(name string) bool {
	return credentialWord.MatchString(name) || credentialShortName.MatchString(name)
}

// maxNestedChecks bounds the decode checks for parameters nested in one value;
// a value holding more is masked whole, so a hostile value cannot make the
// scan quadratic.
const maxNestedChecks = 8

// MaskURLCredentials masks the credentials of a value that holds a URL. For
// `scheme://` followed later by `@`, everything between `://` and the LAST `@`
// becomes `****` (user and password), so a password that contains `@`, `/`,
// `:`, `+` or spaces never leaks and the rule does not depend on net/url
// accepting the string. Query, matrix (`;`) and fragment (`#`) parameters
// whose name is a credential (isCredentialName, any case, spaces around the
// name allowed) are masked too; the name is percent-decoded first, so
// `access%5Ftoken=`, `access_token%3D` and `#access_token=` are caught, and
// a parameter nested in another one's value is found (D-0430). A value
// whose percent-decoded form holds a credential parameter (an encoded
// redirect such as https%3A%2F%2Fx%2F%3Ftoken%3D...) is masked whole. A
// credential value runs to the next `&` or whitespace (a credential holding
// a raw `&` or space keeps the part after it); a value masked whole for an
// encoded credential runs to the next `&`, `;`, `#` or whitespace. Time is
// linear in len(v). A value without `://` is returned unchanged. The rule
// over-masks a path that holds an `@`; that is the safe side.
func MaskURLCredentials(v string) string {
	i := strings.Index(v, "://")
	if i < 0 {
		return v
	}
	if rest := v[i+3:]; strings.Contains(rest, "@") {
		v = v[:i+3] + "****" + rest[strings.LastIndex(rest, "@"):]
	}
	params := urlParams(v)
	if len(params) == 0 {
		return v
	}
	valueEnd, credEnd := valueEnds(v)
	checks := map[int]int{} // value end -> nested decode checks spent
	var b strings.Builder
	done := 0 // v[:done] is already written
	for _, p := range params {
		if p.start < done {
			continue
		}
		end := valueEnd[p.valueStart]
		if isCredentialName(decodeParamName(v[p.nameStart:p.nameEnd])) {
			end = credEnd[p.valueStart]
		} else {
			checks[end]++
			if checks[end] <= maxNestedChecks && !hidesCredentialParam(v[p.valueStart:end], p.encodedEq) {
				continue
			}
		}
		b.WriteString(v[done:p.valueStart])
		b.WriteString("****")
		done = end
	}
	if done == 0 {
		return v
	}
	b.WriteString(v[done:])
	return b.String()
}

// valueEnds returns, for every offset of v (and len(v)), where a parameter
// value starting there ends: valueEnd at the next `&`, `;`, `#` or
// whitespace, credEnd at the next `&` or whitespace (len(v) when none). One
// reverse pass, so lookups stay linear.
func valueEnds(v string) (valueEnd, credEnd []int) {
	valueEnd, credEnd = make([]int, len(v)+1), make([]int, len(v)+1)
	valueEnd[len(v)], credEnd[len(v)] = len(v), len(v)
	for k := len(v) - 1; k >= 0; k-- {
		valueEnd[k], credEnd[k] = valueEnd[k+1], credEnd[k+1]
		switch v[k] {
		case '&', ' ', '\t', '\n', '\r', '\f':
			valueEnd[k], credEnd[k] = k, k
		case ';', '#':
			valueEnd[k] = k
		}
	}
	return valueEnd, credEnd
}

// hidesCredentialParam reports whether a parameter value, once
// percent-decoded, holds a parameter whose name is a credential: the
// encoded redirect case, which the raw scan cannot see. Values without an
// escape are skipped (the raw scan already covers them) unless the name
// ended in an encoded `=` (encodedSep), where the raw scan cannot restart.
func hidesCredentialParam(value string, encodedSep bool) bool {
	if !encodedSep && !hasPercentEscape(value) {
		return false
	}
	d := "?" + decodeParamName(value)
	for _, p := range urlParams(d) {
		if isCredentialName(d[p.nameStart:p.nameEnd]) {
			return true
		}
	}
	return false
}

// decodeParamName percent-decodes a parameter name up to three times, so a
// double-encoded name (access%255Ftoken) is matched too. It is lenient: an
// invalid escape (%zz) stays as written and the valid ones around it still
// decode, so one bad escape cannot hide a credential word. Linear per pass.
func decodeParamName(name string) string {
	for n := 0; n < 3; n++ {
		d, changed := percentDecodeOnce(name)
		if !changed {
			break
		}
		name = d
	}
	return name
}

// hasPercentEscape reports whether s holds a valid %XX escape.
func hasPercentEscape(s string) bool {
	_, changed := percentDecodeOnce(s)
	return changed
}

// percentDecodeOnce decodes every valid %XX escape of s once and reports
// whether any was found.
func percentDecodeOnce(s string) (string, bool) {
	i := strings.IndexByte(s, '%')
	if i < 0 {
		return s, false
	}
	var b strings.Builder
	b.Grow(len(s))
	changed := false
	last := 0
	for ; i < len(s); i++ {
		if s[i] != '%' || i+2 >= len(s) {
			continue
		}
		hi, lo := unhex(s[i+1]), unhex(s[i+2])
		if hi < 0 || lo < 0 {
			continue
		}
		b.WriteString(s[last:i])
		b.WriteByte(byte(hi<<4 | lo))
		last, changed = i+3, true
		i += 2
	}
	if !changed {
		return s, false
	}
	b.WriteString(s[last:])
	return b.String(), true
}

// unhex returns the value of one hex digit, or -1.
func unhex(c byte) int {
	switch {
	case '0' <= c && c <= '9':
		return int(c - '0')
	case 'a' <= c && c <= 'f':
		return int(c-'a') + 10
	case 'A' <= c && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}
