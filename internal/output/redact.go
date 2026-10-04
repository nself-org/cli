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

// redactText removes secrets and personal data from text that is about to
// leave the process in an error message: URL credentials first, then the
// shared telemetry redactor (e-mail, IPs, JWTs, API-key prefixes, ...).
//
// Error text reaches stdout inside the JSON envelope and, through P7-SURF, MCP
// and HTTP clients, so it is redacted before it is written, never after.
func redactText(s string) string {
	return observability.Redact(userinfoRe.ReplaceAllString(s, "${1}[REDACTED]@"))
}

// redactDetail returns a copy of d with its free-text fields redacted. Code,
// docs_url, exit_code and class are fixed vocabulary and stay as they are.
func redactDetail(d *errs.Detail) *errs.Detail {
	c := *d
	c.Message = redactText(c.Message)
	c.Cause = redactText(c.Cause)
	c.Remediation = redactText(c.Remediation)
	return &c
}
