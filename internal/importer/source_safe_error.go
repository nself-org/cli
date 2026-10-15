package importer

import "github.com/nself-org/cli/internal/errs"

// SafeError lets a source explicitly expose a reviewed, credential-free message.
// Inputs: a CLIError whose What uses only fixed templates, never source input.
// Outputs: publicError preserves Code and What for source failure codes only.
// Constraints: Why, Fix, DocsPath and Wrapped are discarded in favor of registered
// defaults; implementations must not put URLs, keys, identifiers or other source
// values into What. A wrapper's error text is never exposed.
type SafeError interface {
	error
	SafeCLIError() *errs.CLIError
}

type sourceSafeError struct{ code, what string }

// NewSafeError marks a fixed source message safe without global registration.
// Inputs: E517/E518/E520/E521 and credential-free fixed template text.
// Outputs: a SafeError, inspectable as a CLIError through errors.As.
// Constraints: callers must never interpolate source input into what; publicError
// still rejects unsupported codes, control characters and oversized text.
func NewSafeError(code, what string) SafeError {
	return sourceSafeError{code: code, what: what}
}

// SafeCLIError returns a fresh structured error, so callers cannot mutate the template.
func (e sourceSafeError) SafeCLIError() *errs.CLIError { return errs.New(e.code, e.what) }

// Error formats the fixed message using the registered error defaults.
func (e sourceSafeError) Error() string { return e.SafeCLIError().Error() }

// Unwrap provides structured inspection without retaining a source error chain.
func (e sourceSafeError) Unwrap() error { return e.SafeCLIError() }

// sourceErrorCode limits the explicit safe-message contract to source failures.
func sourceErrorCode(code string) bool {
	switch code {
	case "E517", "E518", "E520", "E521":
		return true
	default:
		return false
	}
}
