package report

// scrub_patterns.go: the pattern passes of the scrubber (PEM, URL userinfo,
// secret-named assignments).
//
// Purpose: catch secrets whose value was not in the env cascade.
// Inputs: text.
// Outputs: text with placeholders; counts added to the Scrubber.
// Constraints: already-redacted placeholders, ${REF} values and boolean words
//         are left alone; quotes around a redacted value are kept so JSON and
//         YAML stay parseable.

import (
	"regexp"
	"strings"
)

const placeholder = "[REDACTED]"

var (
	// Token shapes observability.Redact does not know, plus any-case bearer.
	extraTokenRe = regexp.MustCompile(`\b(?:xox[abprs]-[A-Za-z0-9-]{10,}|glpat-[A-Za-z0-9_\-]{16,}|npm_[A-Za-z0-9]{30,}|ya29\.[A-Za-z0-9_\-]{20,}|shpat_[A-Fa-f0-9]{16,}|SG\.[A-Za-z0-9_\-]{16,}\.[A-Za-z0-9_\-]{16,}|AGE-SECRET-KEY-[A-Z0-9]{20,}|dop_v1_[a-f0-9]{20,}|hvs\.[A-Za-z0-9_\-]{20,})|(?i:\bbearer)[ \t]+[A-Za-z0-9._~+/=\-]{8,}|` + jwtShape)

	// scheme://user:pass@host where pass may hold / ? # and @ (the match runs to
	// the LAST @ of the token); or scheme://token@host with no colon.
	userinfoRe = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.\-]*://)(?:[^\s"'<>\\@/:?#]*:[^\s"'<>\\]*|[^/?#\s"'<>\\]+)@`)
	// //user:pass@host (scheme dropped), preceded by a non-colon character.
	userinfoNoSchemeRe = regexp.MustCompile(`(^|[^:A-Za-z0-9+.\-])(//)[^\s"'<>\\@/:?#]+:[^\s"'<>\\]*@`)
	// user:pass@tcp(host) database DSN form.
	userinfoDSNRe = regexp.MustCompile(`[A-Za-z0-9_.\-]+:[^\s"'<>\\]*@(tcp|udp|unix|pipe|cloudsql)\(`)

	ident     = secretIdent
	valueExpr = `"(?:[^"\\\r\n]|\\.)*"|'[^'\r\n]*'|\S+`
	// KEY=value, KEY: value, "key": "value", key = value.
	assignRe = regexp.MustCompile(`(?i)(` + ident + `\\?["']?[ \t]*[:=][ \t]*)(` + valueExpr + `)`)
	// ?token=abc&x=1 : the value ends at & or #.
	queryRe = regexp.MustCompile(`(?i)([?&;]` + ident + `=)([^&#\s"']+)`)
	// --password value, --password=value, -p-style flags with a secret word.
	flagRe = regexp.MustCompile(`(?i)(--?` + ident + `(?:=|[ \t]+))("[^"\r\n]*"|'[^'\r\n]*'|[^\s\-][^\s]*)`)
	// ["--admin-secret", "abc"] and --admin-secret","abc" list forms.
	flagListRe = regexp.MustCompile(`(?i)(--?` + ident + `["']?[ \t]*,[ \t]*["']?)([^"',\]\s]+)`)
	// nginx: proxy_set_header X-Admin-Secret abc;  set $token abc;
	nginxRe = regexp.MustCompile(`(?i)(\b(?:proxy_set_header|add_header|more_set_headers|more_set_input_headers|auth_request_set|set)[ \t]+\$?` + ident + `[ \t]+)("[^"\r\n]*"|'[^'\r\n]*'|[^\s;]+)`)
	// Credential-bearing headers whose name has no secret word: rest of line.
	headerRe = regexp.MustCompile(`(?i)(\b(?:proxy-)?authorization["']?[ \t]*[:=][ \t]*|\b(?:set-)?cookie["']?[ \t]*:[ \t]*)([^\r\n]+)`)
)

func (s *Scrubber) scrubUserinfo(text string) string {
	for _, p := range []struct {
		re   *regexp.Regexp
		repl string
	}{
		{userinfoRe, "${1}[REDACTED]@"},
		{userinfoNoSchemeRe, "${1}${2}[REDACTED]@"},
		{userinfoDSNRe, "[REDACTED]@${1}("},
	} {
		if n := len(p.re.FindAllStringIndex(text, -1)); n > 0 {
			text = p.re.ReplaceAllString(text, p.repl)
			s.counts[ClassUserinfo] += n
		}
	}
	return text
}

// scrubAssignments is the defense-in-depth pass.
func (s *Scrubber) scrubAssignments(text string) string {
	text = s.scrubBlockScalars(text)
	for _, re := range []*regexp.Regexp{headerRe, nginxAuthRe, queryRe, sasSigRe, sqlPasswordRe, curlUserRe, assignRe, flagRe, flagListRe, nginxRe} {
		text = s.rewriteValue(re, text)
	}
	return text
}

// rewriteValue replaces submatch 2 of every match of re with a placeholder,
// keeping a matching pair of quotes and one trailing comma.
func (s *Scrubber) rewriteValue(re *regexp.Regexp, text string) string {
	var sb strings.Builder
	last := 0
	for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
		v := text[m[4]:m[5]]
		repl, ok := redactedForm(v)
		if !ok {
			continue
		}
		sb.WriteString(text[last:m[4]])
		sb.WriteString(repl)
		last = m[5]
		s.counts[ClassAssignment]++
	}
	sb.WriteString(text[last:])
	return sb.String()
}

// redactedForm returns the replacement for a value token, or false to keep it.
// Leading and trailing quote, backslash and comma characters stay outside the
// placeholder so JSON/YAML punctuation survives; a secret that itself starts or
// ends with one of them leaks only that character.
func redactedForm(v string) (string, bool) {
	const edge = "\"'\\,"
	core := strings.TrimLeft(v, edge)
	lead := v[:len(v)-len(core)]
	trimmed := strings.TrimRight(core, edge)
	trail := core[len(trimmed):]
	switch {
	case trimmed == "", onlyPlaceholders(trimmed), refValueRe.MatchString(trimmed), isWordValue(trimmed):
		return "", false
	case strings.HasPrefix(trimmed, placeholder+"&"), strings.HasPrefix(trimmed, placeholder+"#"):
		// queryRe already redacted this value and stopped at the & or #: what
		// follows is the next query parameter or the fragment, not a tail.
		return "", false
	}
	return lead + placeholder + trail, true
}

// placeholderRe is one placeholder as the literal pass or this pass writes it.
var placeholderRe = regexp.MustCompile(`\[REDACTED(?::[^\]\s]*)?\]`)

// onlyPlaceholders reports whether s holds at least one placeholder and nothing
// else but known separators: at most one closing bracket or semicolon after
// the last placeholder, and one or two of ":.,;/@-_" between placeholders. A
// token with any other text, symbols included, is NOT already redacted: the
// literal pass may have replaced only the head of a longer secret (R6 d), so
// the later passes redact it whole. A NUL (the PEM tail mark) is ignored.
func onlyPlaceholders(s string) bool {
	s = strings.ReplaceAll(s, "\x00", "")
	if !placeholderRe.MatchString(s) {
		return false
	}
	parts := placeholderRe.Split(s, -1) // text before, between and after the placeholders
	last := len(parts) - 1
	for i, p := range parts {
		switch {
		case i == 0 && p != "":
			return false
		case i == last && i > 0 && (len(p) > 1 || strings.Trim(p, ")]}>;") != ""):
			return false
		case i > 0 && i < last && (len(p) > 2 || strings.Trim(p, ":.,;/@-_") != ""):
			return false
		}
	}
	return true
}

func isWordValue(v string) bool {
	switch strings.ToLower(v) {
	case "true", "false", "yes", "no", "on", "off", "null", "none", "nil":
		return true
	}
	return false
}

// scrubExtraTokens redacts the token shapes in extraTokenRe.
func (s *Scrubber) scrubExtraTokens(text string) string {
	n := len(extraTokenRe.FindAllStringIndex(text, -1))
	if n > 0 {
		text = extraTokenRe.ReplaceAllString(text, placeholder)
		s.counts[ClassPattern] += n
	}
	return text
}
