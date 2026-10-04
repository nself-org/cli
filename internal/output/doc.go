// Package output writes command results and errors for humans and machines.
//
// It is the one writer of contract:cli.json-envelope v1 and the one renderer of
// the human error block (EPIC P7-REG decisions D7-D9). Every surface that
// prints a machine result (help --json, the pilot commands, main, and later
// MCP and HTTP) goes through it, so the envelope cannot drift.
//
// Success envelope:  {"schema_version":"1","command":"<path>","data":<value>}
// Error envelope:    {"schema_version":"1","command":"<path>","error":<Detail>}
// Exactly one of data/error is present. An optional last member "meta" carries
// deprecation notices and warnings recorded with AddDeprecation/AddWarning and
// is omitted when empty. Encoding is 2-space indent, trailing newline, HTML
// escaping off.
//
// Files: envelope.go (types), writer.go (Writer, EmitData, EmitError),
// legacy.go (EmitLegacyCompatible, ADR 0021), redact.go (error text redaction), state.go (invocation and meta
// process state), args.go (JSONRequestedFromArgs), render.go (RenderError).
//
// Error text is redacted before it is written (redact.go), in the envelope and
// in the human block.
//
// Layer: L0. Imports the standard library, golang.org/x/term, internal/errs,
// internal/ui, internal/compat and internal/observability (Redact; it imports
// no other cli package). It never imports cmd/, cobra or a domain package and
// never calls os.Exit.
package output
