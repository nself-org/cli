package output

import (
	"regexp"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/observability"
)

// userinfoRe matches the user:password part of a URL (a DSN such as
// postgres://app:s3cret@db:5432/x). observability.Redact only catches a
// password when the whole "pw@host.tld" looks like an e-mail address, so a DSN
// with a bare host name would pass through it untouched.
var userinfoRe = regexp.MustCompile(`(://)[^/\s:@]+:[^/\s@]+@`)

// keyValueRe matches a secret assigned in text: password=x, --token=x,
// hasura_admin_secret=x, api_key="x", and the JSON form "secret": "x". The key
// is any name containing a secret word; the value is a quoted string or a run
// up to whitespace or a separator. Only "=" and the quoted-JSON colon count as
// assignment, so prose such as "invalid token: expired" is left alone.
var keyValueRe = regexp.MustCompile(
	`(?i)([a-z0-9_.-]*(?:password|passwd|pwd|secret|token|apikey|api[_-]key|private[_-]key|credential)[a-z0-9_.-]*"?\s*(?:=|"\s*:)\s*)("[^"]*"|'[^']*'|[^\s,;&"')\]}]+)`)

// redactText removes secrets and personal data from text that is about to
// leave the process in an error message: URL credentials first, then the
// shared telemetry redactor (e-mail, IPs, JWTs, API-key prefixes, ...).
//
// Error text reaches stdout inside the JSON envelope and, through P7-SURF, MCP
// and HTTP clients, so it is redacted before it is written, never after.
func redactText(s string) string {
	s = userinfoRe.ReplaceAllString(s, "${1}[REDACTED]@")
	s = keyValueRe.ReplaceAllString(s, "${1}[REDACTED]")
	return observability.Redact(s)
}

// redactDetail returns a copy of d with its free-text fields redacted. Code,
// docs_url, exit_code and class are fixed vocabulary and stay as they are.
// A cause or remediation that is the registry's own default text for the code
// is static documentation, not runtime data, and is left untouched (the
// redactor would otherwise mangle prose such as "License keys start with
// 'nself_pro_'" into a redaction marker).
func redactDetail(d *errs.Detail) *errs.Detail {
	c := *d
	entry := errs.Registry[d.Code]
	c.Message = redactText(c.Message)
	if c.Cause != entry.DefaultWhy {
		c.Cause = redactText(c.Cause)
	}
	if c.Remediation != entry.DefaultFix {
		c.Remediation = redactText(c.Remediation)
	}
	return &c
}
