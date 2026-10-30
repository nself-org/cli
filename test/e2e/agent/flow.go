//go:build e2eagent

package agent

// Purpose: the replay client: follow testdata/replay.jsonl against `nself mcp`
// with the mcp-go client, one tools/call per agent step, confirmation tokens
// taken from the previous result, every request, result and harness action
// written to the transcript.
// Inputs: the replay steps, a started *client.Client, a Harness for the
// harness-role steps, a transcript Writer.
// Outputs: nil when every step met its expectation; else the first failure.
// Constraints: a failed step stops the run (never retried silently); the only
// repeat is a step's declared `until` loop over a server-side wait; no sleeps;
// the harness never forges a confirmation token.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Step is one line of testdata/replay.jsonl.
type Step struct {
	ID   string `json:"id"`
	Role string `json:"role"` // agent | harness
	// Kind is tools/call (default) or tools/list for agent steps.
	Kind      string         `json:"kind,omitempty"`
	Tool      string         `json:"tool,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	// Confirm: the first result must be a confirmation; the call is repeated
	// once with its confirm token.
	Confirm bool `json:"confirm,omitempty"`
	// Expect: ok (default) | confirmation | unhealthy | state-ok | evidence-pass.
	Expect string `json:"expect,omitempty"`
	// Capture maps a name to a data field of the final result; later steps
	// use it as ${name} in their arguments.
	Capture map[string]string `json:"capture,omitempty"`
	// Until "terminal" repeats the call while data.state is not terminal
	// (the server waits; MaxRepeats bounds it).
	Until      string `json:"until,omitempty"`
	MaxRepeats int    `json:"max_repeats,omitempty"`
	// Require lists tool names a tools/list step must see.
	Require []string `json:"require,omitempty"`
	// Action names a harness action (harness.go).
	Action string `json:"action,omitempty"`
	Note   string `json:"note,omitempty"`
}

// LoadReplay parses a replay JSONL file.
func LoadReplay(path string) ([]Step, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var steps []Step
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		var s Step
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&s); err != nil {
			return nil, fmt.Errorf("replay line %d: %w", n, err)
		}
		if s.Role != RoleAgent && s.Role != RoleHarness {
			return nil, fmt.Errorf("replay line %d: role %q", n, s.Role)
		}
		steps = append(steps, s)
	}
	return steps, sc.Err()
}

// Harness runs the harness-role steps (sim fleet, docker, git).
type Harness interface {
	Do(ctx context.Context, s Step, w *Writer) error
}

// Runner drives one replay.
type Runner struct {
	C        *client.Client
	H        Harness
	W        *Writer
	captured map[string]string
}

// Run executes steps in order and stops at the first failure.
func (r *Runner) Run(ctx context.Context, steps []Step) error {
	r.captured = map[string]string{}
	for _, s := range steps {
		var err error
		switch {
		case s.Role == RoleHarness:
			err = r.H.Do(ctx, s, r.W)
		case s.Kind == KindToolsList:
			err = r.list(ctx, s)
		default:
			err = r.call(ctx, s)
		}
		if err != nil {
			return fmt.Errorf("step %s: %w", s.ID, err)
		}
	}
	return nil
}

func (r *Runner) list(ctx context.Context, s Step) error {
	if _, err := r.W.Write(Record{Role: RoleAgent, Kind: KindToolsList, Note: s.ID}); err != nil {
		return err
	}
	res, err := r.C.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return err
	}
	names := make([]string, 0, len(res.Tools))
	for _, t := range res.Tools {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	body, _ := json.Marshal(map[string]any{"tools": names})
	if _, err := r.W.Write(Record{Role: RoleServer, Kind: KindResult, Envelope: body, Note: s.ID}); err != nil {
		return err
	}
	if !hasAll(names, s.Require...) {
		return fmt.Errorf("tools/list lacks one of %v (P7-SURF-03 / P7-CI-62 gap)", s.Require)
	}
	return nil
}

// outcome is one tools/call result as the runner sees it.
type outcome struct {
	isError bool
	env     json.RawMessage
	data    map[string]any
}

func (r *Runner) invoke(ctx context.Context, tool string, args map[string]any, id string) (outcome, error) {
	if _, err := r.W.Write(Record{Role: RoleAgent, Kind: KindToolsCall, Tool: tool, Arguments: args, Note: id}); err != nil {
		return outcome{}, err
	}
	wire := map[string]any{}
	for k, v := range args {
		if k != "_secret" {
			wire[k] = v
		}
	}
	req := mcp.CallToolRequest{}
	req.Params.Name, req.Params.Arguments = tool, wire
	res, err := r.C.CallTool(ctx, req)
	if err != nil {
		return outcome{}, fmt.Errorf("tools/call %s: %w", tool, err)
	}
	o := outcome{isError: res.IsError}
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			o.env = json.RawMessage(tc.Text)
			break
		}
	}
	var doc struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(o.env, &doc)
	o.data = doc.Data
	_, err = r.W.Write(Record{Role: RoleServer, Kind: KindResult, Tool: tool, Envelope: compact(o.env), IsError: o.isError, Note: id})
	return o, err
}

// compact re-encodes b on one line; text that is not JSON becomes a JSON
// string (the verifier then reports it as a failed envelope).
func compact(b json.RawMessage) json.RawMessage {
	if len(b) == 0 {
		return nil
	}
	var v any
	if json.Unmarshal(b, &v) != nil {
		s, _ := json.Marshal(string(b))
		return s
	}
	out, _ := json.Marshal(v)
	return out
}

func (r *Runner) call(ctx context.Context, s Step) error {
	args := r.expand(s.Arguments)
	o, err := r.invoke(ctx, s.Tool, args, s.ID)
	if err != nil {
		return err
	}
	if s.Confirm {
		if o.isError || o.data["confirmation_required"] != true {
			return fmt.Errorf("%s: wanted a confirmation request, got %s", s.Tool, o.env)
		}
		token, _ := o.data["confirm"].(string)
		args = withConfirm(args, token)
		if o, err = r.invoke(ctx, s.Tool, args, s.ID+" (confirmed)"); err != nil {
			return err
		}
	}
	for n := 1; s.Until == "terminal" && !o.isError && !terminal(o.data); n++ {
		if n >= max(s.MaxRepeats, 1) {
			return fmt.Errorf("%s: still not terminal after %d calls", s.Tool, n)
		}
		if o, err = r.invoke(ctx, s.Tool, args, fmt.Sprintf("%s (repeat %d)", s.ID, n)); err != nil {
			return err
		}
	}
	if err := expect(s, o); err != nil {
		return err
	}
	for name, field := range s.Capture {
		v, ok := o.data[field].(string)
		if !ok || v == "" {
			return fmt.Errorf("%s: result has no data.%s to capture", s.Tool, field)
		}
		r.captured[name] = v
	}
	return nil
}

func withConfirm(args map[string]any, token string) map[string]any {
	out := map[string]any{}
	for k, v := range args {
		out[k] = v
	}
	out["confirm"] = token
	return out
}

func expect(s Step, o outcome) error {
	state, _ := o.data["state"].(string)
	switch s.Expect {
	case "", "ok":
		if o.isError {
			return fmt.Errorf("%s returned an error: %s", s.Tool, o.env)
		}
	case "confirmation":
		if o.isError || o.data["confirmation_required"] != true {
			return fmt.Errorf("%s: wanted a confirmation request, got %s", s.Tool, o.env)
		}
	case "unhealthy":
		if !o.isError && state == "ok" {
			return fmt.Errorf("%s: wanted an unhealthy report, got %s", s.Tool, o.env)
		}
	case "state-ok":
		if o.isError || state != "ok" {
			return fmt.Errorf("%s: wanted data.state ok, got %s", s.Tool, o.env)
		}
	case "evidence-pass":
		if o.isError || o.data["result"] != "pass" {
			return fmt.Errorf("%s: wanted evidence result pass, got %s", s.Tool, o.env)
		}
	default:
		return fmt.Errorf("unknown expect %q", s.Expect)
	}
	return nil
}

// expand replaces ${name} in string values with captured values.
func (r *Runner) expand(v map[string]any) map[string]any {
	b, _ := json.Marshal(v)
	s := string(b)
	for k, val := range r.captured {
		s = strings.ReplaceAll(s, "${"+k+"}", val)
	}
	out := map[string]any{}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}
