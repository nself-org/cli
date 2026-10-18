package commands

// surface_identity_test.go — the four-surface parity gate (P7-SURF-23, ADR 0017).
//
// Purpose: one place in `go test ./...` (and `make contract-check`, both
//          NSELF_V15 modes) that proves the machine surfaces are the CLI:
//          for a sample per side-effect class the CLI `--json` envelope, the
//          HTTP body and the MCP result carry identical data; every runnable
//          document command of the canon is an envelope command; every
//          config-named flag is bound or exempt in both modes; no MCP tool or
//          HTTP route is registered by hand outside the generators.
// Inputs:  a real nself binary built from ./cmd/nself (the self-exec target of
//          both servers), an in-process httpapi server and an in-process MCP
//          stdio session over the real machine registry, a fixture project, a
//          stub `docker` on PATH and a fixture plugin in a temp HOME.
// Constraints: the identity case runs in a fresh process (the command tree is
//          shared with tests that redeclare flags) and pins v1.5 mode itself,
//          so it gives the same verdict with NSELF_V15 unset and =1; no
//          network; unix only (the stubs are shell scripts). The project dir
//          is rebuilt before every run so a state-changing command sees the
//          same project on each surface. Volatile members (timestamps,
//          durations, confirmation nonces, digits inside doctor messages) are
//          masked before the comparison and nothing else is.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config/bindings"
	"github.com/nself-org/cli/internal/httpapi"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/schemas"
	"gopkg.in/yaml.v3"
)

// surfaceCase is one sampled command: how the CLI runs it and what the
// machine surfaces are asked for. confirm marks a remote or destructive
// command whose machine call is a two-step nonce round trip.
type surfaceCase struct {
	name, tool, route string
	cli               []string
	req               map[string]any
	confirm           bool
	maskDigits        bool
	plan              bool     // plan form: the machine answer carries the CLI --plan data as confirmation.plan
	volatileKeys      []string // members that differ per run (random secrets feed the plan id)
}

var surfaceDigits = regexp.MustCompile(`[0-9]+`)

// surfaceCases is the sample: read, plan, write, destructive and plugin
// commands, a command that fails, and (separately) a stream.
func surfaceCases() []surfaceCase {
	return []surfaceCase{
		{name: "status", tool: "nself_status", route: "/v1/commands/status", cli: []string{"status"}},
		{name: "config get", tool: "nself_config_get", route: "/v1/commands/config/get", cli: []string{"config", "get", "PROJECT_NAME"},
			req: map[string]any{"args": []string{"PROJECT_NAME"}}},
		{name: "config list", tool: "nself_config_list", route: "/v1/commands/config/list", cli: []string{"config", "list"}},
		{name: "help capabilities", tool: "nself_help_capabilities", route: "/v1/commands/help/capabilities", cli: []string{"help", "capabilities", "backup"},
			req: map[string]any{"args": []string{"backup"}}},
		{name: "doctor", tool: "nself_doctor", route: "/v1/commands/doctor", cli: []string{"doctor"}, maskDigits: true},
		{name: "build --plan", tool: "nself_build", route: "/v1/commands/build", cli: []string{"build", "--plan"},
			plan: true, volatileKeys: []string{"plan_id", "confirm"}},
		{name: "config set (confirm)", tool: "nself_config_set", route: "/v1/commands/config/set",
			cli: []string{"config", "set", "PROJECT_DESCRIPTION", "surface identity", "--yes=true"},
			req: map[string]any{"args": []string{"PROJECT_DESCRIPTION", "surface identity"}}, confirm: true, volatileKeys: []string{"plan_id"}},
		{name: "reset (confirm)", tool: "nself_reset", route: "/v1/commands/reset", cli: []string{"reset", "--yes=true"}, confirm: true},
		{name: "plugin read", tool: "nself_fx_status", route: "/v1/commands/fx/status", cli: []string{"fx", "status"}},
		{name: "logs (fails)", tool: "nself_logs", route: "/v1/commands/logs", cli: []string{"logs"}},
	}
}

// surfaceVolatile are the members whose value is the clock, not the command.
var surfaceVolatile = map[string]bool{"timestamp": true, "duration": true}

// normalise parses one JSON document and masks the volatile members.
func normalise(t *testing.T, raw []byte, c surfaceCase, maskNonce bool) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, raw)
	}
	var walk func(any) any
	walk = func(x any) any {
		switch x := x.(type) {
		case map[string]any:
			for k, e := range x {
				if surfaceVolatile[k] || (maskNonce && k == "confirm") || slices.Contains(c.volatileKeys, k) {
					x[k] = "<volatile>"
				} else {
					x[k] = walk(e)
				}
			}
		case []any:
			for i, e := range x {
				x[i] = walk(e)
			}
		case string:
			if c.maskDigits {
				return surfaceDigits.ReplaceAllString(x, "#")
			}
		}
		return x
	}
	return walk(v)
}

// surfaceDiff names the first member where two normalised documents differ.
func surfaceDiff(a, b any, path string) string {
	switch a := a.(type) {
	case map[string]any:
		bm, ok := b.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: object vs %T", path, b)
		}
		keys := map[string]bool{}
		for k := range a {
			keys[k] = true
		}
		for k := range bm {
			keys[k] = true
		}
		names := make([]string, 0, len(keys))
		for k := range keys {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			av, aok := a[k]
			bv, bok := bm[k]
			if aok != bok {
				return path + "." + k + ": present on one side only"
			}
			if d := surfaceDiff(av, bv, path+"."+k); d != "" {
				return d
			}
		}
	case []any:
		bl, ok := b.([]any)
		if !ok || len(a) != len(bl) {
			return path + ": different array length"
		}
		for i := range a {
			if d := surfaceDiff(a[i], bl[i], path+"["+strconv.Itoa(i)+"]"); d != "" {
				return d
			}
		}
	default:
		if !reflect.DeepEqual(a, b) {
			return fmt.Sprintf("%s: %v vs %v", path, a, b)
		}
	}
	return ""
}

// surfaceEnv is the environment of the stub-isolated fixture.
type surfaceEnv struct {
	bin, proj, token string
	envName          string // ENV in the fixture .env; empty is dev
	base             string // http://127.0.0.1:port
	mcp              *stdioSession
}

func (e *surfaceEnv) resetProject(t *testing.T) {
	t.Helper()
	entries, _ := os.ReadDir(e.proj)
	for _, f := range entries {
		_ = os.RemoveAll(filepath.Join(e.proj, f.Name()))
	}
	envName := e.envName
	if envName == "" {
		envName = "dev"
	}
	if err := os.WriteFile(filepath.Join(e.proj, ".env"), []byte("PROJECT_NAME=fixture\nENV="+envName+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// runCLI runs the real binary the way a script does and returns its stdout.
func (e *surfaceEnv) runCLI(t *testing.T, argv ...string) []byte {
	t.Helper()
	e.resetProject(t)
	cmd := exec.Command(e.bin, append(append([]string{}, argv...), "--json")...)
	cmd.Dir = e.proj
	cmd.Env = append(os.Environ(), "NSELF_V15=1", "NSELF_NONINTERACTIVE=1", "CI=1", "NO_COLOR=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run() // a failing command still prints its error envelope
	return out.Bytes()
}

func (e *surfaceEnv) postHTTP(t *testing.T, route string, body map[string]any, accept string) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, e.base+route, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.token)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func (e *surfaceEnv) callMCP(t *testing.T, tool string, body map[string]any) toolResult {
	t.Helper()
	b, _ := json.Marshal(surfaceBody(body))
	return callTool(t, e.mcp, tool, string(b))
}

func surfaceBody(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// with returns req plus one member, leaving req untouched.
func with(req map[string]any, k string, v any) map[string]any {
	out := map[string]any{}
	for key, val := range req {
		out[key] = val
	}
	out[k] = v
	return out
}

func envelopeOf(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var env map[string]json.RawMessage
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("not an envelope: %v\n%s", err, raw)
	}
	return env
}

// checkCase compares one sampled command across the three surfaces.
func (e *surfaceEnv) checkCase(t *testing.T, c surfaceCase) {
	t.Helper()
	if c.plan {
		e.checkPlan(t, c)
		return
	}
	cli := e.runCLI(t, c.cli...)
	want := normalise(t, cli, c, false)
	cliEnv := envelopeOf(t, cli)
	_, hasData := cliEnv["data"]

	httpReq, mcpReq := c.req, c.req
	if c.confirm {
		// Step one: neither surface runs it; both answer with a nonce for the identical request.
		e.resetProject(t)
		status, raw := e.postHTTP(t, c.route, surfaceBody(c.req), "")
		hc := envelopeOf(t, raw)
		if status != http.StatusPreconditionRequired {
			t.Fatalf("%s: HTTP without confirm answered %d: %s", c.name, status, raw)
		}
		res := e.callMCP(t, c.tool, c.req)
		if res.IsError || string(res.StructuredContent["confirmation_required"]) != "true" {
			t.Fatalf("%s: MCP without confirm: %+v", c.name, res)
		}
		var httpData, mcpData struct{ Confirm string }
		_ = json.Unmarshal(hc["data"], &httpData)
		sc, _ := json.Marshal(res.StructuredContent)
		_ = json.Unmarshal(sc, &mcpData)
		if httpData.Confirm == "" || mcpData.Confirm == "" {
			t.Fatalf("%s: no confirm nonce: http %q mcp %q", c.name, httpData.Confirm, mcpData.Confirm)
		}
		if !reflect.DeepEqual(normalise(t, hc["data"], c, true), normalise(t, sc, c, true)) {
			t.Errorf("%s: confirmation differs\nhttp %s\nmcp  %s", c.name, hc["data"], sc)
		}
		httpReq, mcpReq = with(c.req, "confirm", httpData.Confirm), with(c.req, "confirm", mcpData.Confirm)
	}

	e.resetProject(t)
	status, raw := e.postHTTP(t, c.route, surfaceBody(httpReq), "")
	if got := normalise(t, raw, c, false); !reflect.DeepEqual(got, want) {
		t.Errorf("%s: HTTP body differs from CLI --json at %s", c.name, surfaceDiff(want, got, ""))
	}
	if hasData && status != http.StatusOK {
		t.Errorf("%s: HTTP status %d for a data envelope", c.name, status)
	}
	if !hasData && status < 400 {
		t.Errorf("%s: HTTP status %d for an error envelope", c.name, status)
	}

	e.resetProject(t)
	res := e.callMCP(t, c.tool, mcpReq)
	if len(res.Content) != 1 {
		t.Fatalf("%s: MCP result has %d content items", c.name, len(res.Content))
	}
	if got := normalise(t, []byte(res.Content[0].Text), c, false); !reflect.DeepEqual(got, want) {
		t.Errorf("%s: MCP text differs from CLI --json at %s", c.name, surfaceDiff(want, got, ""))
	}
	if res.IsError == hasData {
		t.Errorf("%s: isError %v but the CLI envelope has data %v", c.name, res.IsError, hasData)
	}
	if hasData {
		sc, _ := json.Marshal(res.StructuredContent)
		wantData, gotData := normalise(t, cliEnv["data"], c, false), normalise(t, sc, c, false)
		if !reflect.DeepEqual(gotData, wantData) {
			t.Errorf("%s: MCP structuredContent differs from the CLI data at %s", c.name, surfaceDiff(wantData, gotData, ""))
		}
	}
}

// checkPlan covers a command with a plan form. The machine surfaces never pass
// --plan themselves (the invoker owns it); a staging project makes the plan
// require confirmation, so the answer is confirmation_required with kind plan
// and the plan the CLI prints for `--plan`.
func (e *surfaceEnv) checkPlan(t *testing.T, c surfaceCase) {
	t.Helper()
	e.envName = "staging"
	defer func() { e.envName = "" }()
	cli := e.runCLI(t, c.cli...)
	cliEnv := envelopeOf(t, cli)
	var cliPlan struct {
		PlanID   string `json:"plan_id"`
		Requires bool   `json:"requires_confirmation"`
	}
	_ = json.Unmarshal(cliEnv["data"], &cliPlan)
	if cliPlan.PlanID == "" || !cliPlan.Requires {
		t.Fatalf("%s: the staging plan does not require confirmation: %s", c.name, cli)
	}
	want := normalise(t, cliEnv["data"], c, false)

	e.resetProject(t)
	status, raw := e.postHTTP(t, c.route, map[string]any{}, "")
	var hc struct{ Data json.RawMessage }
	_ = json.Unmarshal(raw, &hc)
	var hd struct {
		Kind string
		Plan json.RawMessage
	}
	_ = json.Unmarshal(hc.Data, &hd)
	if status != http.StatusPreconditionRequired || hd.Kind != "plan" {
		t.Fatalf("%s: HTTP answered %d kind %q: %s", c.name, status, hd.Kind, raw)
	}
	if got := normalise(t, hd.Plan, c, false); !reflect.DeepEqual(got, want) {
		t.Errorf("%s: HTTP confirmation.plan differs from CLI --plan at %s", c.name, surfaceDiff(want, got, ""))
	}

	e.resetProject(t)
	res := e.callMCP(t, c.tool, map[string]any{})
	sc, _ := json.Marshal(res.StructuredContent)
	var md struct {
		Kind string
		Plan json.RawMessage
	}
	_ = json.Unmarshal(sc, &md)
	if res.IsError || md.Kind != "plan" {
		t.Fatalf("%s: MCP answered %+v", c.name, res)
	}
	if got := normalise(t, md.Plan, c, false); !reflect.DeepEqual(got, want) {
		t.Errorf("%s: MCP confirmation.plan differs from CLI --plan at %s", c.name, surfaceDiff(want, got, ""))
	}
}

// checkStream compares the follow form of `logs`: NDJSON over HTTP equals the
// CLI's lines, and MCP refuses it because it never carries a stream. With the
// stub docker the stream is one error line, so this proves the HTTP stream
// path hands the CLI's bytes through; multi-line NDJSON framing is proven in
// internal/httpapi TestNDJSONStream.
func (e *surfaceEnv) checkStream(t *testing.T) {
	t.Helper()
	cli := strings.TrimSpace(string(e.runCLI(t, "logs", "--follow")))
	e.resetProject(t)
	_, raw := e.postHTTP(t, "/v1/commands/logs", map[string]any{"flags": map[string]any{"follow": true}}, "application/x-ndjson")
	got := strings.TrimSpace(string(raw))
	if cli == "" || strings.Count(cli, "\n") != strings.Count(got, "\n") {
		t.Fatalf("logs --follow: CLI lines %q, HTTP lines %q", cli, got)
	}
	cl, hl := strings.Split(cli, "\n"), strings.Split(got, "\n")
	for i := range cl {
		if !reflect.DeepEqual(normalise(t, []byte(cl[i]), surfaceCase{}, false), normalise(t, []byte(hl[i]), surfaceCase{}, false)) {
			t.Errorf("logs --follow line %d: CLI %s, HTTP %s", i, cl[i], hl[i])
		}
	}
	res := e.callMCP(t, "nself_logs", map[string]any{"flags": map[string]any{"follow": true}})
	if !res.IsError {
		t.Errorf("MCP served a streaming logs call: %+v", res)
	}
}

// installSurfacePlugin mounts a read-only fixture plugin `fx` whose binary
// prints one envelope.
func installSurfacePlugin(t *testing.T, plugins string) {
	t.Helper()
	manifest := `{"binaryName": "nself-fx", "category": "infrastructure",
  "commands": {"command": "fx", "binary": "nself-fx", "summary": "fixture commands", "side_effect": "read", "json": "envelope",
    "subcommands": [{"name": "status", "summary": "Read the fixture status", "side_effect": "read", "json": "envelope"}]},
  "description": "fixture", "license": "free", "licenseType": "free", "manifest_version": 2, "maturity": "implemented",
  "minNselfVersion": "1.5.0", "name": "fx", "pluginType": "cli", "requires": {"nself": ">=1.5.0"},
  "service": {"kind": "cli"}, "status": "stable", "tier": "free", "version": "1.0.0"}`
	for path, body := range map[string]string{
		filepath.Join(plugins, "fx", "plugin.json"): manifest,
		filepath.Join(plugins, "bin", "nself-fx"):   "#!/bin/sh\nprintf '{\"schema_version\":\"1\",\"command\":\"fx status\",\"data\":{\"plugin\":\"fx\",\"ok\":true}}\\n'\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// startSurfaceHTTP serves the machine registry on a loopback port.
func startSurfaceHTTP(t *testing.T, proj, token string) string {
	t.Helper()
	reg, err := machineRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := httpapi.New(httpapi.Options{
		Registry: reg, ProjectDir: proj, Port: ln.Addr().(*net.TCPAddr).Port, Auth: httpapi.BearerAuth(token), Log: io.Discard,
		DataSchema: func(path string) ([]byte, bool) { return schemas.Lookup(strings.TrimPrefix(path, "schemas/")) },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = srv.Serve(ctx, ln) }()
	t.Cleanup(func() { cancel(); <-done })
	return "http://" + ln.Addr().String()
}

func TestSurfaceIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture stubs are shell scripts")
	}
	if testing.Short() {
		t.Skip("builds the nself binary")
	}
	if !inFreshProcess(t, "TestSurfaceIdentity") {
		return
	}
	root, _ := filepath.Abs("../..")
	bin := filepath.Join(t.TempDir(), "nself")
	build := exec.Command("go", "build", "-mod=vendor", "-o", bin, "./cmd/nself")
	build.Dir = root
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building nself: %v\n%s", err, out)
	}

	home := t.TempDir()
	stubs := filepath.Join(home, "stubs")
	if err := os.MkdirAll(stubs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stubs, "docker"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NSELF_PLUGIN_DIR", filepath.Join(home, "plugins"))
	t.Setenv("NSELF_PLUGIN_CACHE", filepath.Join(home, "cache"))
	t.Setenv(invoke.SelfExecOverrideEnv, bin)
	proj := filepath.Join(home, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(chdir(t, proj))
	installSurfacePlugin(t, filepath.Join(home, "plugins"))
	compattest.Set(t, true)
	resetMachineRegistry()

	e := &surfaceEnv{bin: bin, proj: proj, token: "surface-identity-token"}
	e.resetProject(t) // the MCP server refuses to start outside a project
	e.base = startSurfaceHTTP(t, proj, e.token)
	s, err := newMCPServer("stdio", true)
	if err != nil {
		t.Fatal(err)
	}
	e.mcp = startStdio(t, s)

	cases := surfaceCases()
	if len(cases) != 10 {
		t.Fatalf("the sample is %d commands, want 10", len(cases))
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { e.checkCase(t, c) })
	}
	t.Run("logs --follow (stream)", e.checkStream)
	t.Log("CHECKED-TestSurfaceIdentity")
}

// handSurfacePatterns find a tool or route registered outside the generators.
var (
	handToolPattern  = regexp.MustCompile(`\.(AddTools?|SetTools|AddSessionTools?)\(|mcp\.NewTool\(|mcp\.NewToolWith\w+\(|mcp\.Tool\{|server\.ServerTool\{`)
	handRoutePattern = regexp.MustCompile(`\.(Mount|Handle|HandleFunc)\(|http\.NewServeMux\(`)
)

// handToolAllowed are the only files that may register an MCP tool: the
// generator, its wiring, and the legacy hand registrations P7-SHIP-09 deletes.
var handToolAllowed = map[string]bool{
	"internal/mcpgen/tools.go": true, "cmd/commands/mcp_generated.go": true,
	"cmd/commands/mcp.go": true, "cmd/commands/mcp_sentry.go": true,
}

// handRouteAllowed is the one file that owns the HTTP API mux and its Mount.
var handRouteAllowed = map[string]bool{"internal/httpapi/server.go": true}

// handSurfaceFindings reports each non-comment line of src (a non-test Go
// file at the repo-relative path rel) that registers a tool or a route when
// rel is not allowed to.
func handSurfaceFindings(rel, src string, tools, routes bool) []string {
	var out []string
	for i, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if tools && !handToolAllowed[rel] && handToolPattern.MatchString(line) {
			out = append(out, rel+":"+strconv.Itoa(i+1)+": hand MCP tool registration: "+strings.TrimSpace(line))
		}
		if routes && !handRouteAllowed[rel] && handRoutePattern.MatchString(line) {
			out = append(out, rel+":"+strconv.Itoa(i+1)+": hand HTTP route registration: "+strings.TrimSpace(line))
		}
	}
	return out
}

// TestSurfaceNoHandSurface: no MCP tool is registered outside mcpgen and the
// legacy funcs, and no route is added to the HTTP API mux outside its server
// file. Negatives: a planted tool and a planted route fail, a reasoned
// allowed file does not.
func TestSurfaceNoHandSurface(t *testing.T) {
	plantedTool := "package commands\nfunc rogue(s *server.MCPServer) { s.AddTool(mcp.NewTool(\"nself_rogue\"), nil) }\n"
	plantedRoute := "package httpapi\nfunc (s *Server) rogue() { s.mux.HandleFunc(\"/v1/rogue\", nil) }\n"
	if len(handSurfaceFindings("cmd/commands/rogue.go", plantedTool, true, false)) == 0 {
		t.Error("a planted hand MCP tool was not found")
	}
	if len(handSurfaceFindings("internal/httpapi/rogue.go", plantedRoute, false, true)) == 0 {
		t.Error("a planted hand HTTP route was not found")
	}
	if got := handSurfaceFindings("internal/mcpgen/tools.go", plantedTool, true, false); len(got) != 0 {
		t.Errorf("the generator file is allowed to register tools: %v", got)
	}
	if got := handSurfaceFindings("cmd/commands/x.go", "// s.AddTool(mcp.NewTool(\"x\"))\n", true, true); len(got) != 0 {
		t.Errorf("a comment is not a registration: %v", got)
	}

	var files, found []string
	for _, dir := range []string{"../../cmd", "../../internal"} {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return nil
			}
			rel := filepath.ToSlash(strings.TrimPrefix(path, "../../"))
			files = append(files, rel)
			// The HTTP API lives in internal/httpapi and is wired by internal/builtin/admin.
			routes := strings.HasPrefix(rel, "internal/httpapi/") || strings.HasPrefix(rel, "internal/builtin/admin/")
			found = append(found, handSurfaceFindings(rel, string(b), true, routes)...)
			return nil
		})
	}
	if len(files) < 500 {
		t.Fatalf("only %d source files were scanned", len(files))
	}
	sort.Strings(found)
	for _, f := range found {
		t.Error(f)
	}
}

// TestSurfaceEnvelopeCoverage: 100% of the runnable document commands of every
// canon fragment are envelope commands unless the fragment reclassifies them
// stream or interactive with a written reason.
func TestSurfaceEnvelopeCoverage(t *testing.T) {
	fragments, err := filepath.Glob("../../internal/canon/domains/*.yaml")
	if err != nil || len(fragments) < 15 {
		t.Fatalf("%d canon fragments found: %v", len(fragments), err)
	}
	reg, err := buildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]cmdregistry.Command{}
	for _, row := range reg.Commands {
		byPath[row.Path] = row
	}
	for _, path := range fragments {
		findings, err := fragmentEnvelopeCoverageFindings(path, byPath)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(path)
		var frag canon.Fragment
		if err := yaml.Unmarshal(raw, &frag); err != nil {
			t.Fatal(err)
		}
		for _, f := range findings {
			// A v1.4-only row has no place in the v1.5 registry this gate reads.
			if key, ok := strings.CutSuffix(f, ": missing registry row"); ok && frag.Commands[key].Mode == canon.ModeV14 {
				continue
			}
			t.Errorf("%s: %s", filepath.Base(path), f)
		}
	}

	// Negatives: a document without an envelope, and an unreasoned reclassification.
	rows := map[string]cmdregistry.Command{"nself sample": {Path: "nself sample", Runnable: true, Canon: canon.CanonSubcommand, JSON: canon.JSONNone}}
	for name, doc := range map[string]string{
		"document without an envelope": "schema_version: 1\ncommands:\n  sample: {side_effect: read}\n",
		"stream without a reason":      "schema_version: 1\ncommands:\n  sample: {side_effect: read, output: stream}\n",
	} {
		if len(envelopeCoverageFindings([]byte(doc), rows)) != 1 {
			t.Errorf("negative %q was accepted", name)
		}
	}
	reasoned := "schema_version: 1\ncommands:\n  sample: {side_effect: read, output: stream} # follows until interrupted\n"
	if got := envelopeCoverageFindings([]byte(reasoned), rows); len(got) != 0 {
		t.Errorf("a reasoned stream row was rejected: %v", got)
	}
}

// TestSurfaceBindingsComplete: every flag named like a configuration key is
// bound or exempt on the v1.5 surface, and an unbound one fails the gate. (The
// bindings name v1.5 flags such as `config secrets --env`; v1.4 has no such flag.)
func TestSurfaceBindingsComplete(t *testing.T) {
	b, err := bindings.Load()
	if err != nil {
		t.Fatal(err)
	}
	refs := registryFlagRefs(t, true)
	if len(refs) < 100 {
		t.Fatalf("only %d flags read from the registry", len(refs))
	}
	if err := b.Check(refs); err != nil {
		t.Error(err)
	}
	bad := append(append([]bindings.FlagRef{}, refs...), bindings.FlagRef{Command: "init", Flag: "base-domain"})
	if err := b.Check(bad); err == nil || !strings.Contains(err.Error(), "BASE_DOMAIN") {
		t.Errorf("an unbound config-named flag passed the gate: %v", err)
	}
}
