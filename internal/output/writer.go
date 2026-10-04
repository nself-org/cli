package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/nself-org/cli/internal/errs"
)

// Writer is the output seam: Out carries the one machine document, Err carries
// diagnostics. Tests pass buffers; Default is the real process streams.
type Writer struct {
	Out io.Writer
	Err io.Writer
}

// Default returns a Writer on os.Stdout and os.Stderr.
func Default() Writer { return Writer{Out: os.Stdout, Err: os.Stderr} }

// errNoOut is returned when a Writer has no Out stream to write the document to.
var errNoOut = errors.New("output: writer has no Out stream")

// EmitData writes the v1 success envelope for command to w.Out and nothing to
// w.Err. data nil is written as `"data": null`. The document is built in full
// before anything is written, so a marshal failure writes nothing and is
// returned. Recorded meta notices (AddDeprecation, AddWarning) are attached.
func EmitData(w Writer, command string, data any) error {
	return writeDoc(w, Envelope{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Data:          data,
		Meta:          currentMeta(),
	})
}

// EmitError writes the v1 error envelope for err to w.Out, using errs.Describe
// for the error object, with message, cause and remediation redacted (see
// redactText). The envelope has no data key. A nil err is a caller
// bug and is returned as an error without writing a document.
func EmitError(w Writer, command string, err error) error {
	d := errs.Describe(err)
	if d == nil {
		return errors.New("output: EmitError called with a nil error")
	}
	d = redactDetail(d)
	return writeDoc(w, ErrorEnvelope{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Error:         d,
		Meta:          currentMeta(),
	})
}

// writeDoc encodes doc as one envelope document (2-space indent, no HTML
// escaping, trailing newline) and writes it to w.Out in a single call.
func writeDoc(w Writer, doc any) error {
	if w.Out == nil {
		return errNoOut
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	_, err := w.Out.Write(buf.Bytes())
	return err
}

// writeBare writes v exactly as ui.PrintJSON does: json.MarshalIndent with
// 2-space indent and HTML escaping on, plus one newline.
func writeBare(w Writer, v any) error {
	if w.Out == nil {
		return errNoOut
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	_, err = w.Out.Write(append(data, '\n'))
	return err
}
