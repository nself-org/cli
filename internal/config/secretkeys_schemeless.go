package config

// Purpose: mask credentials in values that carry no `scheme://` (D-0743): a Go
// MySQL DSN (user:pass@tcp(host:3306)/db), a bare user:pass@host target and
// libpq or ODBC keyword pairs (password=..., pwd=...).
// Inputs: any string. Outputs: the string with those credentials replaced by
// `****`.
// Constraints: never touches plain user@host or an email (no colon before the
// `@`). Linear in len(v) apart from the regexp, which is RE2. Over-masking is
// the safe side, as in MaskURLCredentials.

import (
	"regexp"
	"strings"
)

// dsnProtocol matches the `@protocol(` that follows the userinfo of a Go DSN.
var dsnProtocol = regexp.MustCompile(`@[A-Za-z][A-Za-z0-9]*\(`)

// bareUserinfo matches a whole value of the form user:pass@host[...].
var bareUserinfo = regexp.MustCompile(`^([^\s:@/]+):\S*@([^\s@]+)$`)

// keywordPassword matches a keyword/value password pair. The value is a
// closed single- or double-quoted string, an unclosed quote (to end of line),
// or a run up to whitespace.
var keywordPassword = regexp.MustCompile(`(?i)((?:^|[^A-Za-z0-9_])(?:ssl)?(?:password|passwd|pwd)[ \t]*=[ \t]*)('(?:[^'\\]|\\.)*'|"(?:[^"\\]|\\.)*"|['"].*|[^\s'"]\S*)`)

// urlKeywordPassword is keywordPassword for a value that holds a URL: an
// unquoted value also ends at `&`, `;` and `#`, so the parameters around it
// (already handled by the URL rules) stay readable.
var urlKeywordPassword = regexp.MustCompile(`(?i)((?:^|[^A-Za-z0-9_])(?:ssl)?(?:password|passwd|pwd)[ \t]*=[ \t]*)('(?:[^'\\]|\\.)*'|"(?:[^"\\]|\\.)*"|['"].*|[^\s'"&;#][^\s&;#]*)`)

// maskSchemeless masks the credentials MaskURLCredentials' `://` rule cannot
// see.
func maskSchemeless(v string) string {
	isURL := strings.Contains(v, "://")
	v = maskDSNUserinfo(v)
	if m := bareUserinfo.FindStringSubmatch(v); m != nil && !isURL {
		v = "****@" + m[2]
	}
	if strings.Contains(v, "=") {
		kw := keywordPassword
		if isURL {
			kw = urlKeywordPassword
		}
		v = maskKeywordValues(kw, v)
	}
	return v
}

// maskDSNUserinfo masks user:pass before each `@protocol(`. The token runs back
// to the previous whitespace or quote; a `key=` prefix on the user is kept; a
// token holding `://` was already masked as a URL and is left alone.
func maskDSNUserinfo(v string) string {
	locs := dsnProtocol.FindAllStringIndex(v, -1)
	if len(locs) == 0 {
		return v
	}
	var b strings.Builder
	done := 0
	// One forward pass tracks where the token holding each match starts, so a
	// long run without whitespace is not rescanned per match.
	pos, tokenStart := 0, 0
	for _, loc := range locs {
		at := loc[0]
		for ; pos < at; pos++ {
			if strings.IndexByte(" \t\r\n\"'", v[pos]) >= 0 {
				tokenStart = pos + 1
			}
		}
		if at < done {
			continue
		}
		start := max(tokenStart, done)
		token := v[start:at]
		colon := strings.IndexByte(token, ':')
		if colon < 0 || strings.Contains(token, "://") {
			continue
		}
		keep := token[:strings.LastIndexByte(token[:colon], '=')+1]
		b.WriteString(v[done:start])
		b.WriteString(keep)
		b.WriteString("****")
		done = at
	}
	b.WriteString(v[done:])
	return b.String()
}

// alreadyMasked are the values an earlier pass left in place of a secret; a
// keyword pair holding one is not rewritten, so a caller's own placeholder
// (the unified-diff `[REDACTED]`, or its internal NUL-delimited sentinel)
// survives.
var alreadyMasked = map[string]bool{"****": true, "[REDACTED]": true}

// maskKeywordValues replaces the value of every keyword pair kw finds in v
// with `****`, except a value that is already masked (quotes ignored) or holds
// a NUL byte, which no real credential does.
func maskKeywordValues(kw *regexp.Regexp, v string) string {
	return kw.ReplaceAllStringFunc(v, func(m string) string {
		sub := kw.FindStringSubmatch(m)
		val := strings.Trim(sub[2], `'"`)
		if alreadyMasked[val] || strings.Contains(val, "\x00") {
			return m
		}
		return sub[1] + "****"
	})
}
