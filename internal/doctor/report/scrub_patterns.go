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
	// A whole PEM block whose label holds KEY or CERTIFICATE.
	pemBlockRe = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*(?:KEY|CERTIFICATE)[A-Z0-9 ]*-----.*?-----END [A-Z0-9 ]*(?:KEY|CERTIFICATE)[A-Z0-9 ]*-----`)
	// A BEGIN with no END (a log tail cut the block): redact to the end.
	pemOpenRe = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*(?:KEY|CERTIFICATE)[A-Z0-9 ]*-----.*$`)
	// An END with its body but no BEGIN (the tail started mid-block).
	pemTailRe = regexp.MustCompile(`(?s)(?:[A-Za-z0-9+/=]{16,}(?:\r?\n|\\n)?)+-----END [A-Z0-9 ]*(?:KEY|CERTIFICATE)[A-Z0-9 ]*-----`)

	// Token shapes observability.Redact does not know, plus any-case bearer.
	extraTokenRe = regexp.MustCompile(`\b(?:xox[abprs]-[A-Za-z0-9-]{10,}|glpat-[A-Za-z0-9_\-]{16,}|npm_[A-Za-z0-9]{30,}|ya29\.[A-Za-z0-9_\-]{20,}|shpat_[A-Fa-f0-9]{16,}|SG\.[A-Za-z0-9_\-]{16,}\.[A-Za-z0-9_\-]{16,}|AGE-SECRET-KEY-[A-Z0-9]{20,}|dop_v1_[a-f0-9]{20,}|hvs\.[A-Za-z0-9_\-]{20,})|(?i:\bbearer)[ \t]+[A-Za-z0-9._~+/=\-]{8,}`)

	userinfoRe = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.\-]*://)[^/?#\s"'<>\\]+@`)

	ident     = `[A-Za-z0-9_.\-]*(?:` + secretWords + `)[A-Za-z0-9_.\-]*`
	valueExpr = `"(?:[^"\\\r\n]|\\.)*"|'[^'\r\n]*'|\S+`
	// KEY=value, KEY: value, "key": "value", key = value.
	assignRe = regexp.MustCompile(`(?i)(` + ident + `\\?["']?[ \t]*[:=][ \t]*)(` + valueExpr + `)`)
	// ?token=abc&x=1 : the value ends at & or #.
	queryRe = regexp.MustCompile(`(?i)([?&;]` + ident + `=)([^&#\s"']+)`)
	// --password value, --password=value, -p-style flags with a secret word.
	flagRe = regexp.MustCompile(`(?i)(--?` + ident + `(?:=|[ \t]+))([^\s\-][^\s]*|"[^"\r\n]*"|'[^'\r\n]*')`)
	// ["--admin-secret", "abc"] and --admin-secret","abc" list forms.
	flagListRe = regexp.MustCompile(`(?i)(--?` + ident + `["']?[ \t]*,[ \t]*["']?)([^"',\]\s]+)`)
	// nginx: proxy_set_header X-Admin-Secret abc;  set $token abc;
	nginxRe = regexp.MustCompile(`(?i)(\b(?:proxy_set_header|add_header|more_set_headers|more_set_input_headers|auth_request_set|set)[ \t]+\$?` + ident + `[ \t]+)("[^"\r\n]*"|'[^'\r\n]*'|[^\s;]+)`)
	// Credential-bearing headers whose name has no secret word: rest of line.
	headerRe = regexp.MustCompile(`(?i)(\b(?:proxy-)?authorization["']?[ \t]*[:=][ \t]*|\b(?:set-)?cookie["']?[ \t]*:[ \t]*)([^\r\n]+)`)
)

func (s *Scrubber) scrubPEM(text string) string {
	for _, re := range []*regexp.Regexp{pemBlockRe, pemOpenRe, pemTailRe} {
		n := len(re.FindAllStringIndex(text, -1))
		if n > 0 {
			text = re.ReplaceAllString(text, "[REDACTED:PEM]")
			s.counts[ClassPEM] += n
		}
	}
	return text
}

func (s *Scrubber) scrubUserinfo(text string) string {
	n := len(userinfoRe.FindAllStringIndex(text, -1))
	if n > 0 {
		text = userinfoRe.ReplaceAllString(text, "${1}[REDACTED]@")
		s.counts[ClassUserinfo] += n
	}
	return text
}

// scrubAssignments is the defense-in-depth pass.
func (s *Scrubber) scrubAssignments(text string) string {
	for _, re := range []*regexp.Regexp{headerRe, queryRe, assignRe, flagRe, flagListRe, nginxRe} {
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
	case trimmed == "", strings.HasPrefix(trimmed, "[REDACTED"), refValueRe.MatchString(trimmed), isWordValue(trimmed):
		return "", false
	}
	return lead + placeholder + trail, true
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
