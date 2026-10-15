package progress

// Purpose: redact secrets from an event detail before it is written.
// Inputs: free text (an error message or a reason).
// Outputs: the text with URL credentials and secret assignments masked, then
// the shared telemetry redactor applied.
// Constraints: config owns the secret-key and URL credential rules; the
// expressions here only locate URLs and assignments in free text.

import (
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/observability"
)

// urlRe locates individual URLs so masking cannot consume surrounding prose
// or a second URL's credentials when it searches for the last @.
var urlRe = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"'<>]+`)

// keyValueRe matches a secret assigned in text: password=x, --token=x,
// api_key="x", and the JSON form "secret": "x". Prose such as
// "invalid token: expired" is left alone (only "=" and the quoted-JSON colon
// count as assignment).
// The key is classified by config.IsSecretKey, never a private word list.
var keyValueRe = regexp.MustCompile(
	`(?i)([a-z0-9_.-]+)"?\s*(?:=|"\s*:)\s*`)

// assignmentValueRe locates the value after an assignment prefix.
var assignmentValueRe = regexp.MustCompile(`^("[^"]*"|'[^']*'|[^\s,;&"')\]}]+)`)

// redactText masks URL credentials, then secret assignments, then runs the
// shared telemetry redactor.
func redactText(s string) string {
	s = urlRe.ReplaceAllStringFunc(s, config.MaskURLCredentials)
	var b strings.Builder
	done := 0
	for _, match := range keyValueRe.FindAllStringSubmatchIndex(s, -1) {
		if match[0] < done || !config.IsSecretKey(s[match[2]:match[3]]) {
			continue
		}
		value := assignmentValueRe.FindStringIndex(s[match[1]:])
		if value == nil {
			continue
		}
		b.WriteString(s[done:match[1]])
		b.WriteString("[REDACTED]")
		done = match[1] + value[1]
	}
	b.WriteString(s[done:])
	return observability.Redact(b.String())
}
