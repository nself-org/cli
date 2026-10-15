package imageswitch

// redact.go — error text that is safe to print.
//
// Purpose: psql and pg_restore echo the failing statement, and a failing
// `ALTER ROLE ... PASSWORD '<hash>'` would print a password hash.
// Inputs: tool output. Outputs: its last lines with password material removed.
// Constraints: applied to every one-shot's stderr before it reaches an error.

import (
	"regexp"
	"strings"
)

var (
	passwordClause = regexp.MustCompile(`(?i)(PASSWORD\s+)'[^']*'`)
	scramHash      = regexp.MustCompile(`SCRAM-SHA-256\$\S+`)
	md5Hash        = regexp.MustCompile(`\bmd5[0-9a-f]{32}\b`)
	uriPassword    = regexp.MustCompile(`(://[^/\s:@]+:)[^@\s]+@`)
	connPassword   = regexp.MustCompile(`(?i)(password=)\S+`)
)

// dataLine matches the lines psql and pg_restore add to an error that quote the
// failing row: "CONTEXT: COPY t, line 3: \"<row>\"", "DETAIL: Key (email)=(a@b.c)", "LINE 1: ...".
var dataLine = regexp.MustCompile(`^(CONTEXT|DETAIL|LINE \d+):`)

// redact is redactTail that also removes the project's database password value.
func (s *Switch) redact(text string, n int) string {
	if len(s.cfg.Password) >= 4 {
		text = strings.ReplaceAll(text, s.cfg.Password, "<redacted>")
	}
	return redactTail(text, n)
}

// redactTail returns the last n non-empty lines of s with password material
// replaced and row data omitted. Error summaries end before the first quote;
// command text and continuations after an error summary are discarded.
func redactTail(s string, n int) string {
	s = passwordClause.ReplaceAllString(s, "${1}'<redacted>'")
	s = scramHash.ReplaceAllString(s, "<redacted>")
	s = md5Hash.ReplaceAllString(s, "<redacted>")
	s = uriPassword.ReplaceAllString(s, "${1}<redacted>@")
	s = connPassword.ReplaceAllString(s, "${1}<redacted>")
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "Command was:") {
			break
		}
		if l = strings.TrimSpace(l); l != "" && !dataLine.MatchString(l) {
			if strings.Contains(strings.ToLower(l), "error:") {
				if i := strings.IndexAny(l, "\"'"); i >= 0 {
					l = strings.TrimSpace(l[:i])
				}
				lines = append(lines, l)
				break
			}
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
