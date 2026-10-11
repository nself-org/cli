package invoke

// Purpose: shared types and limits of the machine invocation core.
// Inputs: none. Outputs: Spec, Result, Options, Confirmation and constants.
// Constraints: the constants are part of contract:cli.machine-request v1.

import (
	"io"
	"time"
)

// Transports. TransportHTTPStream is an HTTP request answered as NDJSON; it is
// the only transport that may run a command whose effective output is stream.
const (
	TransportMCP        = "mcp"
	TransportHTTP       = "http"
	TransportHTTPStream = "http-ndjson"
)

// Limits and timings (EPIC D2, D16).
const (
	// DefaultTimeout bounds a document invocation (--timeout on mcp and admin serve).
	DefaultTimeout = 300 * time.Second
	// MaxStdoutBytes caps the child's captured stdout; the rest is drained and dropped.
	MaxStdoutBytes = 32 << 20
	// StderrRingBytes is the stderr kept from the child (the tail).
	StderrRingBytes = 64 << 10
	// CauseTailBytes is the stderr tail an E422 envelope carries in cause.
	CauseTailBytes = 2 << 10
	// MaxRequestBytes bounds DecodeRequest input.
	MaxRequestBytes = 1 << 20
	// MaxElementBytes and MaxArgvBytes bound one argv element and the whole argv.
	MaxElementBytes = 32 << 10
	MaxArgvBytes    = 128 << 10
)

// Environment names (EPIC D2, D16).
const (
	SelfExecOverrideEnv = "NSELF_MCP_EXEC_OVERRIDE"
	TokenEnv            = "NSELF_MCP_TOKEN"
	InvokedByEnv        = "NSELF_INVOKED_BY"
	DeadlineEnv         = "NSELF_INVOKE_DEADLINE_MS"
)

// Redacted replaces secret values in ids, audit records and stderr tails.
const Redacted = "[REDACTED]"

// Spec is one child execution.
type Spec struct {
	// Argv is everything after the binary: path tokens, --json, flags, "--", args.
	Argv []string
	// Dir is the child's working directory (the server's project dir); empty inherits.
	Dir string
	// Transport is TransportMCP, TransportHTTP or TransportHTTPStream.
	Transport string
	// Timeout of 0 means none (NDJSON): only context cancellation ends the child.
	Timeout time.Duration
	// Stdout, when set, receives the child's stdout as it is produced (NDJSON);
	// the child's output is then not captured and Result.Stdout is nil.
	Stdout io.Writer
}

// Result is the outcome of Exec and Invoke.
type Result struct {
	// Stdout is the document bytes: the child's stdout (Exec) or the validated
	// document / error envelope (Invoke).
	Stdout []byte
	// Stderr is the raw, unredacted stderr tail. Exec only: Invoke clears it.
	// Never forward it to a client.
	Stderr []byte
	// ExitCode is the child's exit status, or the exit class of an E42x envelope;
	// -1 when the child was killed by a signal.
	ExitCode int
	// TimedOut is true when the invocation deadline ended the child.
	TimedOut bool
	// StdoutTruncated is true when the child wrote more than MaxStdoutBytes.
	StdoutTruncated bool
	// RequestID is the redacted request id (Invoke only).
	RequestID string
	// ErrorCode is the E42x code when Stdout is an envelope written by this
	// package (Invoke only); empty when Stdout is the child's own document.
	ErrorCode string
}

// Options configures Invoke.
type Options struct {
	// Transport defaults to TransportMCP.
	Transport string
	// Dir is the project directory the child runs in.
	Dir string
	// Timeout of 0 selects DefaultTimeout for document transports and no
	// timeout for TransportHTTPStream.
	Timeout time.Duration
	// Stdout, with TransportHTTPStream, receives the child's NDJSON.
	Stdout io.Writer
}

// Confirmation is the success-envelope data an invoker returns when a command
// needs confirmation (contract:cli.machine-request v1; produced by the gate of
// P7-SURF-25, defined here so the schema ships with the contract). Field order
// is the contract's.
type Confirmation struct {
	ConfirmationRequired bool   `json:"confirmation_required"`
	Kind                 string `json:"kind"`
	Confirm              string `json:"confirm"`
	SideEffect           string `json:"side_effect"`
	Reason               string `json:"reason"`
	Plan                 any    `json:"plan"`
}
