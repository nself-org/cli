package mcpgen_test

// Purpose: tool generation (names, schemas, annotations, exposure), the gate
// round trip, the stream flag, and the v1.4 / v1.5 name rules.

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/mcpgen"
)

func names(h *harness) []string {
	var out []string
	for n := range h.tools {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func props(t *testing.T, tool mcp.Tool) map[string]any {
	t.Helper()
	var s struct {
		Properties map[string]any `json:"properties"`
		Additional any            `json:"additionalProperties"`
	}
	if err := json.Unmarshal(tool.RawInputSchema, &s); err != nil {
		t.Fatalf("%s: input schema: %v", tool.Name, err)
	}
	if s.Additional != false {
		t.Errorf("%s: additionalProperties = %v, want false", tool.Name, s.Additional)
	}
	return s.Properties
}

func TestToolInputSchema(t *testing.T) {
	h := newHarness(t, true, true, true)
	// Every exposed command is a tool named ToolName(path), and nothing else is
	// a tool but the legacy rows.
	table, err := mcpgen.LoadTable()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for i := range h.reg.Commands {
		c := &h.reg.Commands[i]
		if ok, _ := invoke.Exposure(c, invoke.TransportMCP, nil); ok {
			want[cmdregistry.ToolName(c.Path)] = true
		}
	}
	for n := range h.tools {
		_, legacy := table.Lookup(n)
		if !want[n] && !legacy {
			t.Errorf("tool %s is neither a generated name nor a legacy row", n)
		}
	}
	for n := range want {
		if _, ok := h.tools[n]; !ok {
			t.Errorf("exposed command tool %s is missing", n)
		}
	}
	// A plain command: machine request members only.
	p := props(t, h.tools["nself_status_urls"].Tool)
	if p["args"] == nil || p["flags"] == nil || p["confirm"] != nil {
		t.Errorf("nself_status_urls properties %v", p)
	}
	// A gated command adds confirm; a plugin command with args declares them.
	if props(t, h.tools["nself_config_set"].Tool)["confirm"] == nil {
		t.Error("nself_config_set has no confirm property")
	}
	var ack struct {
		Properties struct {
			Args struct {
				MinItems int `json:"minItems"`
			} `json:"args"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(h.tools["sentry_incidents_ack"].Tool.RawInputSchema, &ack); err != nil {
		t.Fatal(err)
	}
	// Annotations follow the side effect.
	for name, want := range map[string][4]bool{
		"nself_doctor":     {true, false, true, false},
		"nself_build":      {false, true, false, false}, // wipe escalates to destructive
		"nself_config_set": {false, true, false, false},
	} {
		a := h.tools[name].Tool.Annotations
		got := [4]bool{*a.ReadOnlyHint, *a.DestructiveHint, *a.IdempotentHint, *a.OpenWorldHint}
		if got != want {
			t.Errorf("%s annotations %v, want %v", name, got, want)
		}
	}
	// The output schema is the committed data schema (an object), or the
	// generic object for a command with none.
	var out map[string]any
	if err := json.Unmarshal(h.tools["nself_status"].Tool.RawOutputSchema, &out); err != nil || out["type"] != "object" {
		t.Errorf("nself_status output schema %v %v", out, err)
	}
	if string(h.tools["sentry_status"].Tool.RawOutputSchema) != `{"type":"object"}` {
		t.Errorf("a plugin command's output schema is %s", h.tools["sentry_status"].Tool.RawOutputSchema)
	}
	// A gated command can answer with a confirmation, so its output schema allows both.
	if !strings.Contains(string(h.tools["nself_config_set"].Tool.RawOutputSchema), `"anyOf"`) {
		t.Error("nself_config_set output schema does not allow a confirmation")
	}
}

func TestStreamFlagHidden(t *testing.T) {
	h := newHarness(t, true, false, false)
	logs := h.tools["nself_logs"]
	flags, _ := props(t, logs.Tool)["flags"].(map[string]any)
	fp, _ := flags["properties"].(map[string]any)
	if _, has := fp["follow"]; has {
		t.Fatal("nself_logs offers follow, a flag with output: stream")
	}
	if fp["tail"] == nil {
		t.Fatal("nself_logs lost tail")
	}
	if !strings.Contains(string(logs.Tool.RawInputSchema), `"additionalProperties":false`) {
		t.Fatal("schema is open")
	}
	res := h.call("nself_logs", `{"flags":{"follow":true}}`)
	if code := errCode(t, res); code != "E420" && code != "E421" {
		t.Fatalf("follow=true returned %s", code)
	}
	// The generated list never carries a stream flag anywhere.
	for n, st := range h.tools {
		if strings.Contains(string(st.Tool.RawInputSchema), `"follow"`) {
			t.Errorf("%s mentions follow", n)
		}
	}
}

func TestUndeclaredSideEffectNotListed(t *testing.T) {
	h := newHarness(t, true, false, true)
	if _, ok := h.tools["fixp_undeclared"]; ok {
		t.Fatal("a plugin subcommand with an undeclared side effect is listed")
	}
	if _, ok := h.tools["nself_fixp_undeclared"]; ok {
		t.Fatal("a plugin subcommand with an undeclared side effect is listed")
	}
	reader, ok := h.tools["nself_fixp_reader"]
	if !ok {
		t.Fatalf("the declared read subcommand is not listed: %v", names(h))
	}
	if a := reader.Tool.Annotations; a.ReadOnlyHint == nil || !*a.ReadOnlyHint {
		t.Errorf("readOnlyHint missing: %+v", a)
	}
}

func TestConfirmationIsNotAnError(t *testing.T) {
	h := newHarness(t, true, false, false)
	res := h.call("nself_config_set", `{"args":["FOO","bar"]}`)
	if res.IsError {
		t.Fatalf("a confirmation is isError: %+v", res)
	}
	sc, _ := res.StructuredContent.(map[string]any)
	if sc["confirmation_required"] != true {
		t.Fatalf("structuredContent %v", res.StructuredContent)
	}
	nonce, _ := sc["confirm"].(string)
	// Repeating the identical call with the nonce runs the command.
	res = h.call("nself_config_set", `{"args":["FOO","bar"],"confirm":"`+nonce+`"}`)
	if res.IsError {
		t.Fatalf("confirmed call failed: %s", res.Content[0].(mcp.TextContent).Text)
	}
	argv := argvOf(t, res)
	if !contains(argv, "--yes=true") || argv[0] != "config" || argv[1] != "set" {
		t.Errorf("argv %v", argv)
	}
	// A nonce is single use.
	if code := errCode(t, h.call("nself_config_set", `{"args":["FOO","bar"],"confirm":"`+nonce+`"}`)); code != "E424" {
		t.Errorf("replayed nonce returned %s", code)
	}
}

func argvOf(t *testing.T, res *mcp.CallToolResult) []string {
	t.Helper()
	sc, _ := res.StructuredContent.(map[string]any)
	raw, _ := sc["argv"].([]any)
	var out []string
	for _, a := range raw {
		out = append(out, a.(string))
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestResultMapping(t *testing.T) {
	h := newHarness(t, true, false, false)
	res := h.call("nself_status", ``)
	if res.IsError || res.StructuredContent == nil {
		t.Fatalf("%+v", res)
	}
	env := envelope(t, res)
	if env["command"] != "stub" || env["data"] == nil {
		t.Errorf("the text content is not the full envelope: %v", env)
	}
	if _, isMap := res.StructuredContent.(map[string]any); !isMap {
		t.Errorf("structuredContent %T", res.StructuredContent)
	}
	// A non-object data is wrapped, an error has isError and no structured content.
	t.Setenv("STUBCLI_MODE", "two")
	if code := errCode(t, h.call("nself_status", ``)); code != "E422" {
		t.Errorf("two documents returned %s", code)
	}
	t.Setenv("STUBCLI_MODE", "raw")
	t.Setenv("STUBCLI_STDOUT", `{"schema_version":"1","command":"stub","data":[1,2]}`)
	res = h.call("nself_status", ``)
	if sc, _ := res.StructuredContent.(map[string]any); res.IsError || sc["value"] == nil {
		t.Errorf("array data not wrapped: %+v", res)
	}
	if bad := h.call("nself_status", `{"nope":1}`); errCode(t, bad) != "E420" || bad.StructuredContent != nil {
		t.Errorf("unknown member: %+v", bad)
	}
}

func TestV14AddsOnlyUnusedNames(t *testing.T) {
	table, err := mcpgen.LoadTable()
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, false, true, true)
	for n := range h.tools {
		if _, legacy := table.Lookup(n); legacy {
			t.Errorf("v1.4 mode registered the legacy name %s", n)
		}
	}
	if _, ok := h.tools["nself_status_urls"]; !ok {
		t.Error("v1.4 mode lost the generated tools")
	}
	if len(h.tools) == 0 {
		t.Fatal("no tools")
	}
}
