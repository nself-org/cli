package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// ResultType is the data.type of the last line of a successful stream.
const ResultType = "result"

// Field is one member of a stream record's data object. Members are written
// in the order given, after the leading type member.
type Field struct {
	Key   string
	Value any
}

// StreamRecord renders one record line of contract:cli.json-stream v1: a
// compact v1 data envelope whose data object is {"type": recordType, ...}
// followed by fields, ending in a newline. recordType names the kind (log,
// progress, event) and may not be empty or "result".
func StreamRecord(command, recordType string, fields ...Field) ([]byte, error) {
	if recordType == "" || recordType == ResultType {
		return nil, fmt.Errorf("output: stream record type %q is reserved or empty", recordType)
	}
	return streamLine(command, recordType, nil, fields)
}

// StreamEnd renders the last line of a successful stream: a data envelope with
// data.type "result" and the command's result members beside it. meta may be
// nil.
func StreamEnd(command string, meta *Meta, fields ...Field) ([]byte, error) {
	return streamLine(command, ResultType, meta, fields)
}

// StreamError renders the last line of a failed stream: the compact v1 error
// envelope (see Error for the rules on d).
func StreamError(command string, d ErrorDetail, meta *Meta) ([]byte, error) {
	if d.Code == "" || d.Message == "" {
		return nil, errors.New("output: error detail needs a code and a message")
	}
	d = completeDetail(d)
	return encode(ErrorEnvelope{SchemaVersion: SchemaVersion, Command: command, Error: &d, Meta: trimMeta(meta)}, false)
}

func streamLine(command, typ string, meta *Meta, fields []Field) ([]byte, error) {
	obj, err := dataObject(typ, fields)
	if err != nil {
		return nil, err
	}
	return encode(Envelope{SchemaVersion: SchemaVersion, Command: command, Data: json.RawMessage(obj), Meta: trimMeta(meta)}, false)
}

// dataObject builds {"type":typ,<fields>} with members in order and no HTML
// escaping.
func dataObject(typ string, fields []Field) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	seen := map[string]bool{"type": true}
	if err := member(&buf, "type", typ, true); err != nil {
		return nil, err
	}
	for _, f := range fields {
		if f.Key == "" || seen[f.Key] {
			return nil, fmt.Errorf("output: stream field key %q is empty, \"type\" or repeated", f.Key)
		}
		seen[f.Key] = true
		if err := member(&buf, f.Key, f.Value, false); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func member(buf *bytes.Buffer, key string, v any, first bool) error {
	if !first {
		buf.WriteByte(',')
	}
	k, err := marshal(key)
	if err != nil {
		return err
	}
	val, err := marshal(v)
	if err != nil {
		return err
	}
	buf.Write(k)
	buf.WriteByte(':')
	buf.Write(val)
	return nil
}

// marshal is compact JSON of v without HTML escaping and without the encoder's
// trailing newline.
func marshal(v any) ([]byte, error) {
	b, err := encode(v, false)
	if err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b, []byte("\n")), nil
}
