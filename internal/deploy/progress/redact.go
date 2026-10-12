package progress

// Purpose: redact secrets from an event detail before it is written.
// Inputs: free text (an error message or a reason).
// Outputs: the text with URL credentials and secret assignments masked, then
// the shared telemetry redactor applied.
// Constraints: userinfoRe, keyValueRe and the order of the three passes MIRROR
// redactText in internal/output/redact.go, which is unexported. A third copy
// of these expressions must not be made: move them to one shared exported
// helper instead and call it from both packages.

import (
	"regexp"

	"github.com/nself-org/cli/internal/observability"
)

// userinfoRe matches the user:password part of a URL (a DSN).
var userinfoRe = regexp.MustCompile(`(://)[^/\s:@]+:[^/\s@]+@`)

// keyValueRe matches a secret assigned in text: password=x, --token=x,
// api_key="x", and the JSON form "secret": "x". Prose such as
// "invalid token: expired" is left alone (only "=" and the quoted-JSON colon
// count as assignment).
var keyValueRe = regexp.MustCompile(
	`(?i)([a-z0-9_.-]*(?:password|passwd|pwd|secret|token|apikey|api[_-]key|private[_-]key|credential)[a-z0-9_.-]*"?\s*(?:=|"\s*:)\s*)("[^"]*"|'[^']*'|[^\s,;&"')\]}]+)`)

// redactText masks URL credentials, then secret assignments, then runs the
// shared telemetry redactor.
func redactText(s string) string {
	s = userinfoRe.ReplaceAllString(s, "${1}[REDACTED]@")
	s = keyValueRe.ReplaceAllString(s, "${1}[REDACTED]")
	return observability.Redact(s)
}
