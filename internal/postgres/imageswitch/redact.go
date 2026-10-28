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

// serverLine matches a server message line: "ERROR:  ...", "FATAL: ..." and the pg_restore
// "pg_restore: error: ..." wrapper around one.
var serverLine = regexp.MustCompile(`\b(ERROR|FATAL|PANIC):|^pg_restore: error:`)

// commandWas matches the pg_restore line that repeats the failing statement.
var commandWas = regexp.MustCompile(`(?i)\bCommand was:`)

// toolLine matches the start of a new message after a "Command was:" statement, which can run over
// several lines of SQL text.
var toolLine = regexp.MustCompile(`^(pg_restore|pg_dump|psql|pg_dumpall):|^(ERROR|FATAL|PANIC|WARNING):`)

// objectNouns are the words that may directly precede a quoted object name in a server message
// ("relation \"t\"", "constraint \"u\""). Any other quoted text can be a value.
var objectNouns = map[string]bool{
	"relation": true, "table": true, "column": true, "constraint": true, "index": true, "schema": true,
	"role": true, "database": true, "type": true, "function": true, "extension": true, "sequence": true,
	"trigger": true, "view": true, "encoding": true, "language": true, "parameter": true, "policy": true,
}

// serverError keeps a server message line up to the first quoted text that is not an object name:
// `invalid input syntax for type integer: "abc"` becomes `invalid input syntax for type integer:`
// while `relation "t" does not exist` is kept whole (D-0805).
func serverError(l string) string {
	for i := 0; i < len(l); i++ {
		if l[i] != '"' && l[i] != '\'' {
			continue
		}
		fields := strings.Fields(l[:i])
		if len(fields) > 0 && objectNouns[strings.ToLower(fields[len(fields)-1])] {
			if end := strings.IndexByte(l[i+1:], l[i]); end >= 0 {
				i += end + 1
				continue
			}
		}
		return strings.TrimSpace(l[:i])
	}
	return l
}

// redact is redactTail that also removes the project's database password value.
func (s *Switch) redact(text string, n int) string {
	if len(s.cfg.Password) >= 4 {
		text = strings.ReplaceAll(text, s.cfg.Password, "<redacted>")
	}
	return redactTail(text, n)
}

// redactTail returns the last n non-empty lines of s with password material
// replaced, the lines that quote row data left out and quoted values cut from server messages.
func redactTail(s string, n int) string {
	s = passwordClause.ReplaceAllString(s, "${1}'<redacted>'")
	s = scramHash.ReplaceAllString(s, "<redacted>")
	s = md5Hash.ReplaceAllString(s, "<redacted>")
	s = uriPassword.ReplaceAllString(s, "${1}<redacted>@")
	s = connPassword.ReplaceAllString(s, "${1}<redacted>")
	var lines []string
	inCommand := false
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		l = strings.TrimSpace(l)
		if inCommand && !toolLine.MatchString(l) {
			continue
		}
		inCommand = false
		if loc := commandWas.FindStringIndex(l); loc != nil {
			// The statement can carry literals: keep only what came before it.
			l, inCommand = strings.TrimSpace(l[:loc[0]]), true
		}
		if l == "" || dataLine.MatchString(l) {
			continue
		}
		if serverLine.MatchString(l) {
			l = serverError(l)
		}
		lines = append(lines, l)
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
