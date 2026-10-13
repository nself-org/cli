package commands

// Purpose: prove the generated MCP surface end to end over a real stdio
// session: the tool list equals the committed golden and the registry, calls
// run through the invoker and its gate, the legacy names resolve, resources
// and prompts answer, and v1.4 mode keeps the hand tools primary.
// Constraints: every case runs in a fresh copy of this test binary (the real
// command tree is shared with tests that redeclare flags); the child that
// executes a command is the stub from internal/invoke/testdata/stubcli; HOME,
// the plugin dir and the plugin cache are temp dirs; no network.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/mcpgen"
)

// stdioSession drives a server over in-memory pipes the way a client drives stdio.
type stdioSession struct {
	t   *testing.T
	in  io.WriteCloser
	out *bufio.Reader
	id  int
}

func startStdio(t *testing.T, s *server.MCPServer) *stdioSession {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.NewStdioServer(s).Listen(ctx, inR, outW)
		_ = outW.Close()
	}()
	t.Cleanup(func() {
		cancel()
		_ = inW.Close()
		_ = outR.Close()
		<-done
	})
	sess := &stdioSession{t: t, in: inW, out: bufio.NewReaderSize(outR, 16<<20)}
	sess.call("initialize", `{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}`)
	return sess
}

// call sends a request and returns the result member (or the error member).
func (s *stdioSession) call(method, params string) json.RawMessage {
	s.t.Helper()
	s.id++
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`+"\n", s.id, method, params)
	if _, err := io.WriteString(s.in, line); err != nil {
		s.t.Fatal(err)
	}
	for {
		raw, err := s.out.ReadBytes('\n')
		if err != nil {
			s.t.Fatalf("%s: %v", method, err)
		}
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(raw, &msg) != nil || len(msg.ID) == 0 {
			continue // a notification
		}
		if len(msg.Error) > 0 {
			return append(json.RawMessage(`{"jsonrpc_error":`), append(msg.Error, '}')...)
		}
		return msg.Result
	}
}

// stdioFixture prepares the hermetic environment of one case: a project, an
// empty plugin dir, a plugin cache, the stub child and the v1.5 mode.
func stdioFixture(t *testing.T, v15 bool) (home string) {
	t.Helper()
	repoRoot, _ = filepath.Abs("../..") // the project dir below is the working directory
	stub := buildStubChild(t)           // before HOME moves: the Go build cache lives under it
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_DIR", filepath.Join(home, "plugins"))
	t.Setenv("NSELF_PLUGIN_CACHE", filepath.Join(home, "cache"))
	proj := filepath.Join(home, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte("PROJECT_NAME=fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(chdir(t, proj))
	t.Setenv(invoke.SelfExecOverrideEnv, stub)
	t.Setenv("STUBCLI_MODE", "")
	compattest.Set(t, v15)
	resetMachineRegistry()
	return home
}

// repoRoot is the absolute cli root, valid after the fixture changes directory.
var repoRoot string

var stubBuilt string

func buildStubChild(t *testing.T) string {
	t.Helper()
	if stubBuilt != "" {
		return stubBuilt
	}
	dir, err := os.MkdirTemp("", "mcp-stub-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir); stubBuilt = "" })
	stubBuilt = filepath.Join(dir, "stubcli")
	if runtime.GOOS == "windows" {
		stubBuilt += ".exe"
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-mod=vendor", "-o", stubBuilt, "./internal/invoke/testdata/stubcli")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the stub child: %v\n%s", err, out)
	}
	return stubBuilt
}

// toolsOf returns the tools/list result as name -> canonical JSON.
func toolsOf(t *testing.T, sess *stdioSession) map[string]string {
	t.Helper()
	var res struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(sess.call("tools/list", `{}`), &res); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, raw := range res.Tools {
		var head struct{ Name string }
		_ = json.Unmarshal(raw, &head)
		if _, dup := out[head.Name]; dup {
			t.Errorf("tool %s is listed twice", head.Name)
		}
		var buf bytes.Buffer
		_ = json.Compact(&buf, raw)
		out[head.Name] = buf.String()
	}
	return out
}

// toolResult is the part of a tools/call result the tests read.
type toolResult struct {
	IsError           bool                       `json:"isError"`
	StructuredContent map[string]json.RawMessage `json:"structuredContent"`
	Content           []struct{ Text string }    `json:"content"`
	RPCError          json.RawMessage            `json:"jsonrpc_error"`
}

func callTool(t *testing.T, sess *stdioSession, name, args string) toolResult {
	t.Helper()
	var res toolResult
	raw := sess.call("tools/call", `{"name":"`+name+`","arguments":`+args+`}`)
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, raw)
	}
	return res
}

func TestMCPGeneratedStdio(t *testing.T) {
	if !inFreshProcess(t, "TestMCPGeneratedStdio") {
		return
	}
	stdioFixture(t, true)
	s, err := newMCPServer("stdio", true)
	if err != nil {
		t.Fatal(err)
	}
	sess := startStdio(t, s)
	listed := toolsOf(t, sess)

	// Every tool is ToolName(p) of an exposed registry command or a legacy alias row.
	reg, err := machineRegistry()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for i := range reg.Commands {
		if ok, _ := invoke.Exposure(&reg.Commands[i], invoke.TransportMCP, nil); ok {
			want[cmdregistry.ToolName(reg.Commands[i].Path)] = true
		}
	}
	table, err := mcpgen.LoadTable()
	if err != nil {
		t.Fatal(err)
	}
	for name := range listed {
		if _, legacy := table.Lookup(name); !want[name] && !legacy {
			t.Errorf("tool %s is neither an exposed command nor a legacy alias row", name)
		}
	}
	for name := range want {
		if _, ok := listed[name]; !ok {
			t.Errorf("exposed command tool %s is not listed", name)
		}
	}

	// The wire tool list is the committed golden, tool for tool.
	var golden struct {
		Tools []json.RawMessage `json:"tools"`
	}
	b, err := os.ReadFile(filepath.Join(repoRoot, "internal/mcpgen/testdata/tools.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden.Tools) != len(listed) {
		t.Errorf("golden has %d tools, the server lists %d", len(golden.Tools), len(listed))
	}
	for _, raw := range golden.Tools {
		var head struct{ Name string }
		_ = json.Unmarshal(raw, &head)
		var buf bytes.Buffer
		_ = json.Compact(&buf, raw)
		if listed[head.Name] != buf.String() {
			t.Errorf("tool %s differs from the golden", head.Name)
		}
	}

	// A read tool runs the child and maps the envelope.
	res := callTool(t, sess, "nself_status", `{}`)
	var argv []string
	_ = json.Unmarshal(res.StructuredContent["argv"], &argv)
	if res.IsError || len(argv) == 0 || argv[0] != "status" || argv[1] != "--json" {
		t.Fatalf("nself_status: %+v argv %v", res, argv)
	}
	if len(res.Content) != 1 || !strings.Contains(res.Content[0].Text, `"schema_version"`) {
		t.Errorf("text content is not the envelope: %+v", res.Content)
	}

	// A destructive tool without confirm returns confirmation_required, isError false.
	res = callTool(t, sess, "nself_config_set", `{"args":["FOO","bar"]}`)
	if res.IsError || string(res.StructuredContent["confirmation_required"]) != "true" {
		t.Fatalf("nself_config_set: %+v", res)
	}

	// A mounted fixture plugin: its read subcommand is listed with readOnlyHint.
	// (Covered with a real mount in TestMCPAliasesV15.)

	// A bad request is an error envelope, not a transport error.
	res = callTool(t, sess, "nself_status", `{"bogus":1}`)
	if !res.IsError || res.RPCError != nil || !strings.Contains(res.Content[0].Text, "E420") {
		t.Errorf("bogus member: %+v", res)
	}
	// Resources and prompts.
	var rl struct {
		Resources []struct{ URI string }
	}
	_ = json.Unmarshal(sess.call("resources/list", `{}`), &rl)
	uris := map[string]bool{}
	for _, r := range rl.Resources {
		uris[r.URI] = true
	}
	for _, u := range []string{"nself://registry", "nself://errors", "nself://plugins/catalog", "nself://config", "nself://services", "nself://env", "nself://urls"} {
		if !uris[u] {
			t.Errorf("resource %s is not listed", u)
		}
	}
	var pl struct{ Prompts []struct{ Name string } }
	_ = json.Unmarshal(sess.call("prompts/list", `{}`), &pl)
	if len(pl.Prompts) != 3 {
		t.Errorf("%d prompts", len(pl.Prompts))
	}
	t.Log("CHECKED-TestMCPGeneratedStdio")
}

func readURI(t *testing.T, sess *stdioSession, uri string) string {
	t.Helper()
	var res struct {
		Contents  []struct{ Text string }
		RPCError  json.RawMessage `json:"jsonrpc_error"`
		ErrorText string
	}
	raw := sess.call("resources/read", `{"uri":"`+uri+`"}`)
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("%s: %v\n%s", uri, err, raw)
	}
	if res.RPCError != nil || len(res.Contents) != 1 {
		t.Fatalf("%s: %s", uri, raw)
	}
	return res.Contents[0].Text
}

func TestMCPResourcesV15(t *testing.T) {
	if !inFreshProcess(t, "TestMCPResourcesV15") {
		return
	}
	home := stdioFixture(t, true)
	cat, err := os.ReadFile(filepath.Join(repoRoot, "cmd/commands/testdata/capabilities/catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "cache", "catalog.json"), cat, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := newMCPServer("stdio", true)
	if err != nil {
		t.Fatal(err)
	}
	sess := startStdio(t, s)

	// nself://registry equals the data of help --json.
	out, _ := runJSON(t)
	var env struct{ Data json.RawMessage }
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if !sameJSON(t, []byte(readURI(t, sess, "nself://registry")), env.Data) {
		t.Error("nself://registry differs from the data of help --json")
	}
	// nself://errors lists E433 and the others.
	if body := readURI(t, sess, "nself://errors"); !strings.Contains(body, `"E433"`) || !strings.Contains(body, `"E420"`) {
		t.Error("nself://errors misses codes")
	}
	// nself://schemas/<name> returns the committed bytes.
	for uri, file := range map[string]string{
		"nself://schemas/envelope.v1.schema.json":        "schemas/envelope.v1.schema.json",
		"nself://schemas/commands/status.v1.schema.json": "schemas/commands/status.v1.schema.json",
	} {
		want, err := os.ReadFile(filepath.Join(repoRoot, file))
		if err != nil {
			t.Fatal(err)
		}
		if got := readURI(t, sess, uri); got != string(want) {
			t.Errorf("%s differs from the committed bytes", uri)
		}
	}
	// nself://plugins/catalog is the cached catalogue.
	if got := readURI(t, sess, "nself://plugins/catalog"); got != string(cat) {
		t.Error("nself://plugins/catalog is not the cached catalogue")
	}
	// The legacy URIs return their command's data (the stub echoes argv) when the
	// command is exposed; nself://services keeps its hand handler until
	// `config services list` has an envelope.
	if body := readURI(t, sess, "nself://services"); !strings.Contains(body, "postgres") {
		t.Errorf("nself://services lost its hand handler: %.200s", body)
	}
	for uri, path := range map[string]string{"nself://config": "config show",
		"nself://env": "config env explain", "nself://urls": "status urls"} {
		var data struct{ Argv []string }
		body := readURI(t, sess, uri)
		if err := json.Unmarshal([]byte(body), &data); err != nil {
			t.Fatalf("%s: %v\n%s", uri, err, body)
		}
		if got := strings.Join(data.Argv[:len(strings.Fields(path))], " "); got != path {
			t.Errorf("%s ran %q, want %q", uri, got, path)
		}
	}
	t.Log("CHECKED-TestMCPResourcesV15")
}

func sameJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xa, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return bytes.Equal(xa, yb)
}

// legacyHandNames lists the tool names the hand registrations add.
func legacyHandNames(t *testing.T) []string {
	t.Helper()
	s := server.NewMCPServer("test", "0", server.WithToolCapabilities(true))
	registerMCPTools(s)
	var names []string
	for name := range s.ListTools() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestMCPAliasesV15(t *testing.T) {
	if !inFreshProcess(t, "TestMCPAliasesV15") {
		return
	}
	home := stdioFixture(t, true)
	hand := legacyHandNames(t)
	table, err := mcpgen.LoadTable()
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, r := range table.Legacy() {
		rows = append(rows, r.Name)
	}
	sort.Strings(rows)
	if strings.Join(hand, ",") != strings.Join(rows, ",") || len(hand) != 26 {
		t.Fatalf("the alias table and the 26 hand tools differ:\nhand  %v\nrows  %v", hand, rows)
	}

	// Without the plugin the five sentry names answer E404 naming nself add sentry-cli.
	s, err := newMCPServer("stdio", true)
	if err != nil {
		t.Fatal(err)
	}
	sess := startStdio(t, s)
	listed := toolsOf(t, sess)
	for _, name := range hand {
		d := ""
		if raw, ok := listed[name]; !ok {
			t.Errorf("legacy name %s does not resolve in v1.5 mode", name)
		} else {
			var tool struct{ Description string }
			_ = json.Unmarshal([]byte(raw), &tool)
			d = tool.Description
		}
		if r, _ := table.Lookup(name); r != nil && r.Kind != mcpgen.KindMerged && !strings.HasPrefix(d, "Deprecated:") {
			t.Errorf("%s: description %q does not start with Deprecated:", name, d)
		}
	}
	res := callTool(t, sess, "sentry_status", `{}`)
	if !res.IsError || !strings.Contains(res.Content[0].Text, "E404") || !strings.Contains(res.Content[0].Text, "nself add sentry-cli") {
		t.Errorf("sentry_status without the plugin: %+v", res)
	}
	res = callTool(t, sess, "nself_run_migration", `{"sql":"SELECT 1"}`)
	if !res.IsError || !strings.Contains(res.Content[0].Text, "E433") {
		t.Errorf("nself_run_migration: %+v", res)
	}

	// With a fixture sentry-cli plugin installed the aliases run its commands.
	installSentryFixture(t, filepath.Join(home, "plugins"))
	resetMachineRegistry()
	s, err = newMCPServer("stdio", true)
	if err != nil {
		t.Fatal(err)
	}
	sess = startStdio(t, s)
	listed = toolsOf(t, sess)
	if _, ok := listed["nself_sentry_status"]; !ok {
		t.Fatalf("the fixture plugin's commands are not tools: %v", len(listed))
	}
	// An undeclared side effect is destructive and has no confirm block: not listed.
	if _, ok := listed["nself_sentry_wipe"]; ok {
		t.Error("a mounted plugin subcommand with an undeclared side effect is listed")
	}
	var status struct {
		Annotations struct {
			ReadOnlyHint *bool `json:"readOnlyHint"`
		} `json:"annotations"`
	}
	_ = json.Unmarshal([]byte(listed["nself_sentry_status"]), &status)
	if status.Annotations.ReadOnlyHint == nil || !*status.Annotations.ReadOnlyHint {
		t.Errorf("the declared read subcommand has no readOnlyHint: %s", listed["nself_sentry_status"])
	}
	res = callTool(t, sess, "sentry_incidents_ack", `{"id":"inc-1"}`)
	var argv []string
	_ = json.Unmarshal(res.StructuredContent["argv"], &argv)
	if res.IsError || strings.Join(argv[:3], " ") != "sentry incidents ack" || argv[len(argv)-1] != "inc-1" {
		t.Fatalf("sentry_incidents_ack: %+v argv %v", res, argv)
	}
	t.Log("CHECKED-TestMCPAliasesV15")
}

// installSentryFixture installs a minimal sentry-cli plugin whose commands are
// the ones the five legacy sentry tools map to.
func installSentryFixture(t *testing.T, dir string) {
	t.Helper()
	manifest := `{
  "binaryName": "nself-sentry", "category": "infrastructure",
  "commands": {"command": "sentry", "binary": "nself-sentry", "summary": "sentry commands", "side_effect": "read", "json": "envelope",
    "subcommands": [
      {"name": "monitors list", "summary": "List monitors", "side_effect": "read", "json": "envelope"},
      {"name": "monitors add", "summary": "Add a monitor", "side_effect": "write", "json": "envelope",
       "flags": [{"name": "url", "type": "string"}, {"name": "name", "type": "string"}, {"name": "kind", "type": "string"}, {"name": "interval", "type": "duration"}]},
      {"name": "incidents list", "summary": "List incidents", "side_effect": "read", "json": "envelope", "flags": [{"name": "status", "type": "string"}]},
      {"name": "incidents ack", "summary": "Acknowledge an incident", "side_effect": "write", "json": "envelope", "args": [{"name": "id", "required": true}]},
      {"name": "status", "summary": "Read the status", "side_effect": "read", "json": "envelope", "flags": [{"name": "url", "type": "string"}]},
      {"name": "wipe", "summary": "Declares no side effect", "json": "envelope"}
    ]},
  "description": "fixture", "license": "free", "licenseType": "free", "manifest_version": 2, "maturity": "implemented",
  "minNselfVersion": "1.5.0", "name": "sentry-cli", "pluginType": "cli", "requires": {"nself": ">=1.5.0"},
  "service": {"kind": "cli"}, "status": "stable", "tier": "free", "version": "1.0.0"
}`
	plug := filepath.Join(dir, "sentry-cli")
	if err := os.MkdirAll(plug, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plug, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin := "nself-sentry"
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", bin), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestMCPGeneratedFailureByMode(t *testing.T) {
	if !inFreshProcess(t, "TestMCPGeneratedFailureByMode") {
		return
	}
	stdioFixture(t, false)
	// A registry that cannot be built: v1.4 keeps the hand tools, v1.5 refuses to start.
	machineCache.mu.Lock()
	machineCache.done, machineCache.reg, machineCache.err = true, nil, fmt.Errorf("fixture: broken plugin manifest")
	machineCache.mu.Unlock()
	s, err := newMCPServer("stdio", false)
	if err != nil {
		t.Fatalf("v1.4 mode must keep serving: %v", err)
	}
	if n := len(s.ListTools()); n != 26 {
		t.Errorf("v1.4 fallback serves %d tools, want the 26 hand tools", n)
	}
	if _, err := newMCPServer("stdio", true); err == nil {
		t.Error("v1.5 mode started without a registry")
	}
	t.Log("CHECKED-TestMCPGeneratedFailureByMode")
}

func TestMCPV14KeepsHandTools(t *testing.T) {
	if !inFreshProcess(t, "TestMCPV14KeepsHandTools") {
		return
	}
	stdioFixture(t, false)
	s, err := newMCPServer("stdio", false)
	if err != nil {
		t.Fatal(err)
	}
	sess := startStdio(t, s)
	listed := toolsOf(t, sess)
	table, _ := mcpgen.LoadTable()
	hand := legacyHandNames(t)
	for _, name := range hand {
		if _, ok := listed[name]; !ok {
			t.Errorf("v1.4 mode lost the hand tool %s", name)
		}
	}
	// Generated tools appear only under names no legacy tool uses.
	generated := 0
	for name := range listed {
		if _, legacy := table.Lookup(name); !legacy {
			generated++
		}
	}
	if generated == 0 {
		t.Error("v1.4 mode adds no generated tool")
	}
	// The hand tool is the one that answers: its description is the hand text.
	var tool struct{ Description string }
	_ = json.Unmarshal([]byte(listed["nself_status"]), &tool)
	if !strings.HasPrefix(tool.Description, "Health status of every service") {
		t.Errorf("nself_status is not the hand tool: %q", tool.Description)
	}
	// The initialize result carries experimental: true.
	init := sess.call("ping", `{}`)
	_ = init
	b, _ := json.Marshal(s.HandleMessage(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":99,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`)))
	if !strings.Contains(string(b), `"experimental":{"experimental":true}`) {
		t.Errorf("initialize does not carry experimental: true: %s", b)
	}
	t.Log("CHECKED-TestMCPV14KeepsHandTools")
}

var (
	promptSpelling = regexp.MustCompile(`\bnself ((?:[a-z][a-z0-9-]*)(?: [a-z][a-z0-9-]*)*)`)
	promptTool     = regexp.MustCompile(`\b(nself_[a-z0-9_]+)\b`)
	promptResource = regexp.MustCompile(`nself://[a-z/]+`)
)

// promptTexts renders every prompt with sample arguments.
func promptTexts(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, h := range map[string]struct {
		args map[string]string
		get  func(context.Context, mcp.GetPromptRequest) (*mcp.GetPromptResult, error)
	}{
		"diagnose-failure": {map[string]string{"service": "db"}, mcpDiagnoseFailurePrompt()},
		"add-service":      {map[string]string{"name": "mlflow"}, mcpAddServicePrompt()},
		"prepare-deploy":   {map[string]string{"env": "prod"}, mcpPrepareDeployPrompt()},
	} {
		req := mcp.GetPromptRequest{}
		req.Params.Arguments = h.args
		res, err := h.get(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range res.Messages {
			out[name] += m.Content.(mcp.TextContent).Text + "\n"
		}
	}
	return out
}

func TestMCPPromptSpellings(t *testing.T) {
	if !inFreshProcess(t, "TestMCPPromptSpellings") {
		return
	}
	stdioFixture(t, true)
	reg, err := machineRegistry()
	if err != nil {
		t.Fatal(err)
	}
	texts := promptTexts(t)
	if len(texts) != 3 {
		t.Fatalf("%d prompts", len(texts))
	}
	s, err := newMCPServer("stdio", true)
	if err != nil {
		t.Fatal(err)
	}
	sess := startStdio(t, s) // one stdio session per server
	v15Tools := toolsOf(t, sess)
	hand := map[string]bool{}
	for _, n := range legacyHandNames(t) {
		hand[n] = true
	}
	var rl struct{ Resources []struct{ URI string } }
	_ = json.Unmarshal(sess.call("resources/list", `{}`), &rl)
	resources := map[string]bool{}
	for _, r := range rl.Resources {
		resources[r.URI] = true
	}
	checked := 0
	for name, text := range texts {
		for _, m := range promptSpelling.FindAllStringSubmatch(text, -1) {
			path := strings.TrimSpace(m[1])
			if _, ok := reg.Lookup(path); !ok {
				t.Errorf("prompt %s names `nself %s`, which is not in the v1.5 registry", name, path)
			}
			checked++
		}
		for _, m := range promptTool.FindAllStringSubmatch(text, -1) {
			if _, ok := v15Tools[m[1]]; !ok {
				t.Errorf("prompt %s names the tool %s, which the v1.5 server does not register", name, m[1])
			}
			if !hand[m[1]] {
				t.Errorf("prompt %s names the tool %s, which the v1.4 server does not register", name, m[1])
			}
			checked++
		}
		for _, u := range promptResource.FindAllString(text, -1) {
			if !resources[u] {
				t.Errorf("prompt %s names the resource %s, which the v1.5 server does not list", name, u)
			}
			checked++
		}
	}
	if checked < 10 {
		t.Fatalf("only %d spellings were checked", checked)
	}
	t.Log("CHECKED-TestMCPPromptSpellings")
}

// TestNoHandToolsOutsideLegacy: mcp.NewTool( is called only by the legacy
// registration files; every other tool is generated.
func TestNoHandToolsOutsideLegacy(t *testing.T) {
	allowed := map[string]bool{"mcp.go": true, "mcp_sentry.go": true}
	for _, dir := range []string{".", "../../internal"} {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, rerr := os.ReadFile(path)
			if rerr == nil && bytes.Contains(b, []byte("mcp.NewTool(")) && !allowed[d.Name()] {
				t.Errorf("%s registers a hand tool", path)
			}
			return nil
		})
	}
}
