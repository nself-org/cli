package errs

import "errors"

// Detail is the machine error object of contract:cli.error-codes v1. It is
// what the JSON error envelope carries under "error". JSON field order is the
// struct field order.
type Detail struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Cause       string `json:"cause,omitempty"`
	Remediation string `json:"remediation,omitempty"`
	DocsURL     string `json:"docs_url,omitempty"`
	ExitCode    int    `json:"exit_code"`
	Class       string `json:"class"`
}

// docsBaseURL prefixes every DocsPath.
const docsBaseURL = "https://nself.org/docs/"

// ClassFor names the exit class of a process exit status: user (1), infra
// (2), auth (3), destructive_blocked (4), and other for anything else.
func ClassFor(exitCode int) string {
	switch exitCode {
	case ExitUserError:
		return "user"
	case ExitInfraError:
		return "infra"
	case ExitAuthError:
		return "auth"
	case ExitDestructiveBlocked:
		return "destructive_blocked"
	}
	return "other"
}

// Describe returns the machine error object for err, or nil when err is nil.
//
// Purpose: the one function main and the JSON envelope call to turn any error
// into code, message, cause, remediation, docs link, exit status and class.
//
// Inputs: any error. Precedence for the code: a *CLIError's Code, else the
// sentinel table's code for the matching sentinel of the highest class
// (auth, then destructive, then infra, then user), else E400.
//
// Outputs: Message is the CLIError's What, else err.Error(). Cause and
// Remediation are the CLIError's Why and Fix; for a sentinel-derived code
// they are the registry defaults. DocsURL is https:// plus the DocsPath of
// the error, else of the registry entry. ExitCode equals ExitCodeFor(err), so
// it is the process exit status, and Class follows from it.
func Describe(err error) *Detail {
	if err == nil {
		return nil
	}
	d := &Detail{Message: err.Error(), ExitCode: ExitCodeFor(err)}
	d.Class = ClassFor(d.ExitCode)

	var ce *CLIError
	if errors.As(err, &ce) && ce.Code != "" {
		d.Code = ce.Code
		if ce.What != "" {
			d.Message = ce.What
		}
		d.Cause = ce.Why
		d.Remediation = ce.Fix
		d.DocsURL = docsURL(ce.DocsPath, ce.Code)
		return d
	}

	if code, ok := CodeForSentinel(err); ok {
		d.Code = code
		entry := Registry[code]
		d.Cause = entry.DefaultWhy
		d.Remediation = entry.DefaultFix
		d.DocsURL = docsURL(entry.DocsPath, code)
		return d
	}

	d.Code = "E400"
	d.DocsURL = docsURL("", d.Code)
	return d
}

// docsURL builds the docs link from path, falling back to the registry
// entry's DocsPath for code. It returns "" when neither is known.
func docsURL(path, code string) string {
	if path == "" {
		path = Registry[code].DocsPath
	}
	if path == "" {
		return ""
	}
	return docsBaseURL + path
}
