package output

// Purpose: writers for contract:cli.json-stream v1 (P7-SURF-27). A stream is
// NDJSON where every line is one compact v1 envelope, so the envelope contract
// holds per line. Record lines are data envelopes whose data.type names the
// record kind (log, progress, event; never "result"). The last line is either a
// data envelope with data.type "result" or a compact error envelope.
// Inputs: an io.Writer, the registry command path, and a record or result value
// that marshals to a JSON object.
// Outputs: one newline-terminated line per call, written in a single Write.
// Constraints: same encoder settings as the document writer (no HTML escaping,
// UTF-8, key order of Envelope); sdk/go/output renders the same bytes for
// plugins, pinned by the shared cases under sdk/go/output/testdata/cases.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ResultType is the data.type of the last line of a successful stream.
const ResultType = "result"

// StreamRecord writes one record line: {"schema_version":"1","command":...,
// "data":rec} followed by a newline. rec must marshal to a JSON object whose
// first member is "type" with a non-empty string value other than "result"
// (a struct with Type as its first field does). Nothing is written on error.
func StreamRecord(w io.Writer, command string, rec any) error {
	raw, keys, err := objectOf(rec)
	if err != nil {
		return fmt.Errorf("output: stream record: %w", err)
	}
	if len(keys) == 0 || keys[0] != "type" {
		return errors.New(`output: stream record: first member must be "type"`)
	}
	var head struct {
		Type any `json:"type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return fmt.Errorf("output: stream record: %w", err)
	}
	typ, ok := head.Type.(string)
	if !ok || typ == "" || typ == ResultType {
		return fmt.Errorf("output: stream record: type %v is empty, not a string or reserved", head.Type)
	}
	return writeLine(w, Envelope{SchemaVersion: SchemaVersion, Command: command, Data: json.RawMessage(raw)})
}

// StreamEnd writes the last line of a stream. With err nil it is a data
// envelope whose data object is {"type":"result", <members of result>}; result
// may be nil (a bare result) and must not carry its own "type" member, and
// recorded meta notices are attached. With err non-nil it is the compact v1
// error envelope for err (same error object and redaction as EmitError) and
// result is ignored. Nothing is written when it returns an error.
func StreamEnd(w io.Writer, command string, result any, err error) error {
	if err != nil {
		return writeErrorLine(w, command, err)
	}
	raw, keys, oerr := objectOf(result)
	if oerr != nil {
		return fmt.Errorf("output: stream end: %w", oerr)
	}
	for _, k := range keys {
		if k == "type" {
			return errors.New(`output: stream end: result must not carry a "type" member`)
		}
	}
	data := `{"type":"result"}`
	if len(keys) > 0 {
		data = `{"type":"result",` + string(raw[1:])
	}
	return writeLine(w, Envelope{SchemaVersion: SchemaVersion, Command: command, Data: json.RawMessage(data), Meta: currentMeta()})
}

// writeErrorLine renders the error envelope through EmitError (the shared
// error-envelope logic) and writes it compact.
func writeErrorLine(w io.Writer, command string, err error) error {
	var doc bytes.Buffer
	if werr := EmitError(Writer{Out: &doc, Err: io.Discard}, command, err); werr != nil {
		return werr
	}
	var line bytes.Buffer
	if cerr := json.Compact(&line, bytes.TrimSpace(doc.Bytes())); cerr != nil {
		return fmt.Errorf("output: stream error line: %w", cerr)
	}
	line.WriteByte('\n')
	return writeBytes(w, line.Bytes())
}

// writeLine encodes doc compact (no indent, no HTML escaping, trailing
// newline) and writes it in a single call.
func writeLine(w io.Writer, doc any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	return writeBytes(w, buf.Bytes())
}

func writeBytes(w io.Writer, b []byte) error {
	if w == nil {
		return errNoOut
	}
	_, err := w.Write(b)
	return err
}

// objectOf marshals v compact without HTML escaping and returns the bytes with
// the top-level member names in document order. A nil v and JSON null yield an
// empty object description (nil bytes, no keys); anything else must be an object.
func objectOf(v any) ([]byte, []string, error) {
	if v == nil {
		return nil, nil, nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, nil, fmt.Errorf("json marshal: %w", err)
	}
	raw := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	if string(raw) == "null" {
		return nil, nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, nil, errors.New("value must marshal to a JSON object")
	}
	keys := []string{}
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		name, _ := k.(string)
		keys = append(keys, name)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, nil, err
		}
	}
	return raw, keys, nil
}
