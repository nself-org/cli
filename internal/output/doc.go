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
// legacy.go (EmitLegacyCompatible, ADR 0021), state.go (invocation and meta
// process state), args.go (JSONRequestedFromArgs), render.go (RenderError).
//
// Layer: L0. Imports the standard library, internal/errs, internal/ui and
// internal/compat only. It never imports cmd/, cobra or a domain package and
// never calls os.Exit.
package output
