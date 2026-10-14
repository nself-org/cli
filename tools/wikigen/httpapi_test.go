package main

// Tests for HTTP-API-Reference.md: the contract bytes, one row per OpenAPI
// operation, and the side-effect and streaming columns on fixtures.

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
)

func TestHTTPAPIPageMatchesOpenAPI(t *testing.T) {
	page, err := renderHTTPAPI(hermeticRegistry(t), repoTestSources)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := os.ReadFile(contractFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(page, generatedNotice+"\n"+string(contract)) {
		t.Fatal("page does not start with the generated header and the exact bytes of contract.md")
	}
	spec, err := os.ReadFile(openapiFile)
	if err != nil {
		t.Fatal(err)
	}
	// Count operations from the raw document with no method list of our own:
	// every path-item key that holds an object with an operationId is one.
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	var want []string
	for path, item := range doc.Paths {
		for method, raw := range item {
			var op struct {
				OperationID string `json:"operationId"`
			}
			if json.Unmarshal(raw, &op) == nil && op.OperationID != "" {
				want = append(want, strings.ToUpper(method)+" `"+path+"`")
			}
		}
	}
	var got []string
	for _, l := range strings.Split(page, "\n") {
		if cells := strings.Split(l, " | "); len(cells) == 5 && strings.HasPrefix(l, "| ") && openAPIMethods[strings.ToLower(strings.TrimPrefix(cells[0], "| "))] {
			got = append(got, strings.TrimPrefix(cells[0], "| ")+" "+cells[1])
		}
	}
	sort.Strings(want)
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("page has %d operation rows, openapi has %d\npage:    %v\nopenapi: %v", len(got), len(want), got, want)
	}
	if last := lastLine(page); !strings.HasPrefix(last, "**Plugin commands.**") || !strings.Contains(last, "RoutePath") {
		t.Errorf("last line is %q", last)
	}
}

const fixtureSpec = `{"paths":{
 "/v1/commands/logs":{"post":{"operationId":"nself_logs","summary":"Show logs"}},
 "/v1/commands/destroy":{"post":{"operationId":"nself_destroy","summary":"Remove | it"}},
 "/openapi.json":{"get":{"operationId":"openapi","summary":"Read this OpenAPI document"}}}}`

func TestHTTPPageFixture(t *testing.T) {
	stream := "stream"
	cmds := map[string]*cmdregistry.Command{
		"nself_logs":    fixtureCmd("read", &cmdregistry.Flag{Name: "follow", Type: "bool", Output: &stream}),
		"nself_destroy": fixtureCmd("destructive"),
	}
	resolve := func(id string) *cmdregistry.Command { return cmds[id] }
	page, err := renderHTTPPage("# Title\n\nbody\n", []byte(fixtureSpec), resolve)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(page, generatedNotice+"\n# Title\n\nbody\n\n## Operations") {
		t.Errorf("page frame: %q", page[:80])
	}
	want := []string{
		"| GET | `/openapi.json` | Read this OpenAPI document | `read` | no |",
		"| POST | `/v1/commands/destroy` | Remove \\| it | `destructive` | no |",
		"| POST | `/v1/commands/logs` | Show logs | `read` | with `follow` |",
	}
	at := -1
	for _, w := range want {
		i := strings.Index(page, w)
		if i <= at {
			t.Errorf("row %q missing or out of order", w)
		}
		at = i
	}
	// A contract without a trailing newline still gets a clean break.
	if p, err := renderHTTPPage("# Title", []byte(fixtureSpec), resolve); err != nil || !strings.HasPrefix(p, generatedNotice+"\n# Title\n\n## Operations") {
		t.Errorf("no-newline contract: %v", err)
	}
	// Drift: a changed operation summary changes the page; an unknown operation fails.
	changed := strings.Replace(fixtureSpec, "Show logs", "Tail logs", 1)
	if p, _ := renderHTTPPage("# Title\n\nbody\n", []byte(changed), resolve); p == page {
		t.Error("a changed summary did not change the page")
	}
	delete(cmds, "nself_logs")
	if _, err := renderHTTPPage("x\n", []byte(fixtureSpec), resolve); err == nil {
		t.Error("an operation without a registry command must fail")
	}
}

// fixtureCmd is a runnable core document command; confirm is set for the
// remote and destructive classes so it is exposed.
func fixtureCmd(side string, flags ...*cmdregistry.Flag) *cmdregistry.Command {
	cmd := &cmdregistry.Command{Runnable: true, Canon: canon.CanonCore, JSON: canon.JSONEnvelope, Output: canon.OutputDocument, SideEffect: side}
	if side == canon.SideEffectRemote || side == canon.SideEffectDestructive {
		cmd.Confirm = &cmdregistry.Confirm{Flags: []string{"yes"}}
	}
	for _, f := range flags {
		cmd.Flags = append(cmd.Flags, *f)
	}
	return cmd
}

func TestOpenAPIOperationsMethods(t *testing.T) {
	spec := `{"paths":{
 "/a":{"summary":"item metadata","parameters":[{"name":"x"}],"servers":[],"x-note":{"operationId":"not_an_operation"},
  "get":{"operationId":"op_get"},"put":{"operationId":"op_put"},"post":{"operationId":"op_post"},
  "delete":{"operationId":"op_delete"},"options":{"operationId":"op_options"},"head":{"operationId":"op_head"},
  "patch":{"operationId":"op_patch"},"trace":{"operationId":"op_trace"}}}}`
	ops, err := openAPIOperations([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, op := range ops {
		got = append(got, op.Method+":"+op.ID)
	}
	want := "DELETE:op_delete GET:op_get HEAD:op_head OPTIONS:op_options PATCH:op_patch POST:op_post PUT:op_put TRACE:op_trace"
	if strings.Join(got, " ") != want {
		t.Errorf("operations = %v", got)
	}
	for name, bad := range map[string]string{
		"missing id":   `{"paths":{"/a":{"put":{"summary":"no id"}}}}`,
		"duplicate id": `{"paths":{"/a":{"get":{"operationId":"x"}},"/b":{"patch":{"operationId":"x"}}}}`,
		"not an op":    `{"paths":{"/a":{"delete":"nope"}}}`,
	} {
		if _, err := openAPIOperations([]byte(bad)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestHTTPPageAllMethodsAndUnknownID(t *testing.T) {
	spec := `{"paths":{"/v1/a":{"put":{"operationId":"nself_a","summary":"A"},"patch":{"operationId":"nself_b","summary":"B"},"delete":{"operationId":"nself_c","summary":"C"}}}}`
	cmds := map[string]*cmdregistry.Command{"nself_a": fixtureCmd("write"), "nself_b": fixtureCmd("write"), "nself_c": fixtureCmd("write")}
	resolve := func(id string) *cmdregistry.Command { return cmds[id] }
	page, err := renderHTTPPage("# T\n", []byte(spec), resolve)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"| DELETE | `/v1/a` | C |", "| PATCH | `/v1/a` | B |", "| PUT | `/v1/a` | A |"} {
		if !strings.Contains(page, m) {
			t.Errorf("page lacks %q", m)
		}
	}
	unknown := strings.Replace(spec, `"nself_c"`, `"nself_unknown"`, 1)
	if _, err := renderHTTPPage("# T\n", []byte(unknown), resolve); err == nil || !strings.Contains(err.Error(), "nself_unknown") {
		t.Errorf("unknown operationId: err = %v", err)
	}
}

// TestHTTPSideAndStreamingExposure: only what a request can reach is advertised.
func TestHTTPSideAndStreamingExposure(t *testing.T) {
	stream, destructive, write := "stream", "destructive", "write"
	hidden := func(f cmdregistry.Flag) cmdregistry.Flag { f.Hidden = true; return f }
	deprecated := func(f cmdregistry.Flag) cmdregistry.Flag { f.Deprecated = sp("old"); return f }
	cliOnly := func(f cmdregistry.Flag) cmdregistry.Flag { f.CLIOnly = true; return f }
	danger := cmdregistry.Flag{Name: "danger", Type: "bool", SideEffect: &destructive}
	follow := cmdregistry.Flag{Name: "follow", Type: "bool", Output: &stream}

	unreachable := fixtureCmd("write")
	unreachable.Confirm = &cmdregistry.Confirm{Flags: []string{"yes"}, Plan: &cmdregistry.PlanForm{Flag: "plan", IDFlag: "plan-id"}}
	for _, f := range []cmdregistry.Flag{
		hidden(danger), deprecated(cliDanger("d2")), cliOnly(cliDanger("d3")),
		{Name: "yes", Type: "bool", SideEffect: &destructive}, {Name: "plan", Type: "bool", SideEffect: &destructive},
		{Name: "plan-id", Type: "string", SideEffect: &destructive}, {Name: "json", Type: "bool", SideEffect: &destructive},
		{Name: "weird", Type: "map", SideEffect: &destructive},
		hidden(follow), deprecated(cmdregistry.Flag{Name: "f2", Type: "bool", Output: &stream}), cliOnly(cmdregistry.Flag{Name: "f3", Type: "bool", Output: &stream}),
		{Name: "f4", Type: "map", Output: &stream},
	} {
		unreachable.Flags = append(unreachable.Flags, f)
	}
	if got := httpSide(unreachable); got != "write" {
		t.Errorf("side with only unreachable flags = %s, want write", got)
	}
	if got := httpStreaming(unreachable); got != "no" {
		t.Errorf("streaming with only unreachable flags = %s, want no", got)
	}

	reach := fixtureCmd("read", &follow, &cmdregistry.Flag{Name: "apply", Type: "bool", SideEffect: &write})
	if got := httpSide(reach); got != "write" {
		t.Errorf("side with an exposed write flag = %s", got)
	}
	if got := httpStreaming(reach); got != "with `follow`" {
		t.Errorf("streaming = %s", got)
	}
	// A stream flag that itself raises the side effect cannot stream: Exposure refuses it.
	risky := fixtureCmd("read", &cmdregistry.Flag{Name: "tail", Type: "bool", Output: &stream, SideEffect: &write})
	if got := httpStreaming(risky); got != "no" {
		t.Errorf("a stream flag with a write effect streams: %s", got)
	}
	always := fixtureCmd("read")
	always.Output = canon.OutputStream
	if got := httpStreaming(always); got != "yes" {
		t.Errorf("stream command = %s", got)
	}
}

func cliDanger(name string) cmdregistry.Flag {
	d := "destructive"
	return cmdregistry.Flag{Name: name, Type: "bool", SideEffect: &d}
}
