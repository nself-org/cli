//go:build e2eagent

package agent

// Purpose: the agent E2E transcript: one JSON object per line recording every
// agent MCP request, every server result and every harness action.
// Inputs: Records from the replay client (flow.go) or a live session.
// Outputs: a JSONL file; ReadTranscript parses one back for verify.go.
// Constraints: the field set is the ticket's transcript format
// {seq, time, role, kind, tool, arguments, envelope, is_error, note}; secret
// argument values and confirmation tokens are never written (they become
// Redacted), so a transcript is safe to upload as a CI artifact.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Roles and kinds of a transcript record.
const (
	RoleAgent   = "agent"
	RoleHarness = "harness"
	RoleServer  = "server"

	KindToolsList = "tools/list"
	KindToolsCall = "tools/call"
	KindResult    = "result"
	KindAction    = "action"
	KindProbe     = "probe"
)

// Redacted replaces secret values and confirmation tokens in a transcript.
const Redacted = "[REDACTED]"

// Record is one transcript line.
type Record struct {
	Seq       int             `json:"seq"`
	Time      string          `json:"time"`
	Role      string          `json:"role"`
	Kind      string          `json:"kind"`
	Tool      string          `json:"tool,omitempty"`
	Arguments map[string]any  `json:"arguments,omitempty"`
	Envelope  json.RawMessage `json:"envelope,omitempty"`
	IsError   bool            `json:"is_error"`
	Note      string          `json:"note,omitempty"`
}

// Writer appends records to a transcript file; safe for concurrent use.
type Writer struct {
	mu  sync.Mutex
	f   *os.File
	seq int
	// Records keeps every written record so the test can verify in process.
	Records []Record
}

// NewWriter creates (truncates) path with mode 0600.
func NewWriter(path string) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &Writer{f: f}, nil
}

// Write stamps seq and time, redacts r and appends it as one line.
func (w *Writer) Write(r Record) (Record, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seq++
	r.Seq = w.seq
	r.Time = time.Now().UTC().Format(time.RFC3339Nano)
	r.Arguments = redactArgs(r.Arguments)
	r.Envelope = redactEnvelope(r.Envelope)
	b, err := json.Marshal(r)
	if err != nil {
		return r, err
	}
	if _, err := w.f.Write(append(b, '\n')); err != nil {
		return r, err
	}
	w.Records = append(w.Records, r)
	return r, nil
}

// Close closes the file.
func (w *Writer) Close() error { return w.f.Close() }

// redactArgs copies args with the confirm token and every flag or arg named
// in the private "_secret" list replaced by Redacted. The list itself is a
// replay-file annotation and is dropped.
func redactArgs(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}
	out := map[string]any{}
	secret := map[string]bool{}
	if list, ok := args["_secret"].([]any); ok {
		for _, s := range list {
			if name, ok := s.(string); ok {
				secret[name] = true
			}
		}
	}
	for k, v := range args {
		switch {
		case k == "_secret":
		case k == "confirm":
			out[k] = Redacted
		case k == "flags":
			out[k] = redactFlags(v, secret)
		default:
			out[k] = v
		}
	}
	return out
}

func redactFlags(v any, secret map[string]bool) any {
	flags, ok := v.(map[string]any)
	if !ok || len(secret) == 0 {
		return v
	}
	out := map[string]any{}
	for k, fv := range flags {
		if secret[k] {
			out[k] = Redacted
		} else {
			out[k] = fv
		}
	}
	return out
}

// redactEnvelope replaces data.confirm (a nonce or plan id) with Redacted.
func redactEnvelope(env json.RawMessage) json.RawMessage {
	if len(env) == 0 {
		return env
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(env, &doc) != nil {
		return env
	}
	var data map[string]any
	if json.Unmarshal(doc["data"], &data) != nil || data["confirm"] == nil {
		return env
	}
	data["confirm"] = Redacted
	b, err := json.Marshal(data)
	if err != nil {
		return env
	}
	doc["data"] = b
	out, err := json.Marshal(doc)
	if err != nil {
		return env
	}
	return out
}

// ReadTranscript parses a JSONL transcript; a malformed line is an error.
func ReadTranscript(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for n := 1; sc.Scan(); n++ {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("transcript line %d: %w", n, err)
		}
		out = append(out, r)
	}
	return out, sc.Err()
}
