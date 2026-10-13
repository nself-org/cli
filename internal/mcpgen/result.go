package mcpgen

// Purpose: map a v1 envelope document onto an MCP tool result (EPIC D5).
// Inputs: the envelope bytes an invoker returned, optional deprecation notes.
// Outputs: *mcp.CallToolResult: structuredContent = envelope data (a non-object
// wrapped as {"value": ...}), text content = the full envelope, isError = the
// envelope has error. A confirmation is a success envelope, so isError is false.
// Constraints: never returns a Go error; a document that is not an envelope
// becomes an error result so a client always gets a result object.

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
)

// envelopeDoc is the v1 envelope with its members kept raw.
type envelopeDoc struct {
	SchemaVersion string          `json:"schema_version"`
	Command       string          `json:"command"`
	Data          json.RawMessage `json:"data,omitempty"`
	Error         json.RawMessage `json:"error,omitempty"`
	Meta          *output.Meta    `json:"meta,omitempty"`
}

// ErrorEnvelope renders err as a v1 error envelope for command.
func ErrorEnvelope(command string, err error) []byte {
	var buf bytes.Buffer
	if e := output.EmitError(output.Writer{Out: &buf, Err: io.Discard}, command, err); e != nil {
		return []byte(`{"schema_version":"1","command":"` + command + `","error":{"code":"E422","message":"could not write the error"}}` + "\n")
	}
	return buf.Bytes()
}

// ErrorResult is the tool result for an error raised before or instead of a run.
func ErrorResult(command string, err error, deps []output.Deprecation) *mcp.CallToolResult {
	return Result(ErrorEnvelope(command, err), deps)
}

// Result maps envelope bytes to a tool result, adding deps to meta.deprecations.
func Result(stdout []byte, deps []output.Deprecation) *mcp.CallToolResult {
	var doc envelopeDoc
	if err := json.Unmarshal(stdout, &doc); err != nil || doc.SchemaVersion == "" {
		return textError(errs.New("E422", "the command did not return a v1 envelope"))
	}
	if len(deps) > 0 {
		meta := output.Meta{}
		if doc.Meta != nil {
			meta = *doc.Meta
		}
		meta.Deprecations = append(append([]output.Deprecation{}, meta.Deprecations...), deps...)
		doc.Meta = &meta
		if b, err := json.MarshalIndent(doc, "", "  "); err == nil {
			stdout = append(b, '\n')
		}
	}
	text := string(bytes.TrimRight(stdout, "\n"))
	isErr := len(doc.Error) > 0 && string(doc.Error) != "null"
	res := &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: text}}, IsError: isErr}
	if isErr {
		return res
	}
	data := bytes.TrimSpace(doc.Data)
	if len(data) == 0 {
		data = []byte("null")
	}
	if data[0] != '{' {
		data = append(append([]byte(`{"value":`), data...), '}')
	}
	var generic any
	if err := json.Unmarshal(data, &generic); err == nil {
		res.StructuredContent = generic
		res.RawStructuredContent = append(json.RawMessage{}, data...)
	}
	return res
}

// textError never echoes the document: it was not a valid envelope, so it is untrusted.
func textError(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: err.Error()}}, IsError: true}
}
