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
)

// redactTail returns the last n non-empty lines of s with password material replaced.
func redactTail(s string, n int) string {
	s = passwordClause.ReplaceAllString(s, "${1}'<redacted>'")
	s = scramHash.ReplaceAllString(s, "<redacted>")
	s = md5Hash.ReplaceAllString(s, "<redacted>")
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
