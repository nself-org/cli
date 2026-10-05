// Package output is the plugin-module copy of the CLI machine contract:
// contract:cli.json-envelope v1, contract:cli.error-codes v1,
// contract:cli.exit-codes v1 and contract:cli.json-stream v1.
//
// Plugins are separate Go modules and cannot import the CLI's internal/output,
// so this package renders the same bytes. Key order, two-space indent,
// trailing newline and no HTML escaping are part of the contract. The two
// implementations are pinned together by the shared cases in testdata/cases,
// which this package's tests and the CLI's internal/repoqa test both read:
// a drift in either side fails a test.
//
// Redaction is not done here. The CLI redacts error text in its own writer;
// a plugin must keep secrets and personal data out of the messages it passes
// to Error.
//
// This package imports the standard library only.
package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// SchemaVersion is the envelope version, the string "1". It versions the
// envelope only; the shape of Data is each command's own schema.
const SchemaVersion = "1"

// Envelope is the v1 success document: schema_version, command, data, then
// the optional meta. Data is written as null when nil.
type Envelope struct {
	SchemaVersion string `json:"schema_version"`
	Command       string `json:"command"`
	Data          any    `json:"data"`
	Meta          *Meta  `json:"meta,omitempty"`
}

// ErrorEnvelope is the v1 error document: schema_version, command, error, then
// the optional meta. It has no data key.
type ErrorEnvelope struct {
	SchemaVersion string       `json:"schema_version"`
	Command       string       `json:"command"`
	Error         *ErrorDetail `json:"error"`
	Meta          *Meta        `json:"meta,omitempty"`
}

// ErrorDetail is the machine error object (contract:cli.error-codes v1). JSON
// field order is the struct field order.
type ErrorDetail struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Cause       string `json:"cause,omitempty"`
	Remediation string `json:"remediation,omitempty"`
	DocsURL     string `json:"docs_url,omitempty"`
	ExitCode    int    `json:"exit_code"`
	Class       string `json:"class"`
}

// Meta is the optional last member of an envelope. Both lists are omitted when
// empty, and a Meta with both empty is omitted entirely.
type Meta struct {
	Deprecations []Deprecation `json:"deprecations,omitempty"`
	Warnings     []string      `json:"warnings,omitempty"`
}

// Deprecation is one deprecation notice: the old invocation, its replacement
// and the release at which the old form is removed.
type Deprecation struct {
	Old       string `json:"old"`
	New       string `json:"new"`
	RemovalAt string `json:"removal_at"`
}

// Data renders the v1 success envelope for command: 2-space indent, no HTML
// escaping, one trailing newline. data nil is written as `"data": null`. meta
// may be nil.
func Data(command string, data any, meta *Meta) ([]byte, error) {
	return encode(Envelope{SchemaVersion: SchemaVersion, Command: command, Data: data, Meta: trimMeta(meta)}, true)
}

// Error renders the v1 error envelope for command. It has no data key. When
// ExitCode is 0 it is derived from Class (ExitCodeFor); when Class is empty it
// is derived from ExitCode (ClassFor). Code and Message are required.
func Error(command string, d ErrorDetail, meta *Meta) ([]byte, error) {
	if d.Code == "" || d.Message == "" {
		return nil, errors.New("output: error detail needs a code and a message")
	}
	d = completeDetail(d)
	return encode(ErrorEnvelope{SchemaVersion: SchemaVersion, Command: command, Error: &d, Meta: trimMeta(meta)}, true)
}

// WriteData renders Data and writes it to w in one call.
func WriteData(w io.Writer, command string, data any, meta *Meta) error {
	b, err := Data(command, data, meta)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// WriteError renders Error and writes it to w in one call.
func WriteError(w io.Writer, command string, d ErrorDetail, meta *Meta) error {
	b, err := Error(command, d, meta)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// completeDetail fills the exit code and class from each other.
func completeDetail(d ErrorDetail) ErrorDetail {
	if d.ExitCode == 0 && d.Class != "" {
		d.ExitCode = ExitCodeFor(d.Class)
	}
	if d.Class == "" && d.ExitCode != 0 {
		d.Class = ClassFor(d.ExitCode)
	}
	return d
}

// trimMeta returns nil for a nil or empty meta so no meta key is written.
func trimMeta(m *Meta) *Meta {
	if m == nil || (len(m.Deprecations) == 0 && len(m.Warnings) == 0) {
		return nil
	}
	return m
}

// encode writes v as one JSON document with a trailing newline and without
// HTML escaping; indent selects the 2-space form.
func encode(v any, indent bool) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if indent {
		enc.SetIndent("", "  ")
	}
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}
	return buf.Bytes(), nil
}
