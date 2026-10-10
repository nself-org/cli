package invoke

// Purpose: shared fixtures for the invoke tests: the stub child, a registry
// built from a small cobra tree and a canon document.
// Constraints: the stub is built once in TestMain; no network, no real nself.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/schemas"
	"github.com/spf13/cobra"
)

var stubPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "invoke-stub-")
	if err != nil {
		panic(err)
	}
	name := "stubcli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	stubPath = filepath.Join(dir, name)
	build := exec.Command("go", "build", "-o", stubPath, "./testdata/stubcli")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		panic("building the stub child: " + err.Error() + "\n" + string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// useStub points the invoker at the stub child with the given STUBCLI_* env.
func useStub(t *testing.T, kv ...string) {
	t.Helper()
	t.Setenv(SelfExecOverrideEnv, stubPath)
	t.Setenv("STUBCLI_MODE", "")
	t.Setenv("STUBCLI_STDERR", "")
	t.Setenv("STUBCLI_EXIT", "")
	t.Setenv("STUBCLI_STDOUT", "")
	for i := 0; i+1 < len(kv); i += 2 {
		t.Setenv(kv[i], kv[i+1])
	}
}

const fixtureCanon = `
schema_version: 1
verbs: [fx]
commands:
  fx: {canon: core}
  fx echo:
    side_effect: read
    flags:
      tok: {secret: true}
      reveal: {cli_only: true}
      follow: {output: stream}
  fx secretset: {side_effect: write, secret_args: [value]}
  fx clionly: {side_effect: read, surface: cli-only}
  fx hid: {side_effect: read}
  fx legacy: {side_effect: read, json: legacy}
  fx interactive: {side_effect: read, output: interactive}
  fx streamer: {side_effect: read, output: stream}
  fx streamwrite: {side_effect: write, output: stream}
  fx remote: {side_effect: remote}
  fx destroy:
    side_effect: destructive
    confirm: {flags: [yes]}
  fx apply:
    side_effect: write
    confirm: {flags: [force], plan: {flag: plan, id_flag: plan-id}}
  fx oldecho: {canon: deprecated-shim, target: fx echo, side_effect: read}
  fx escalate:
    side_effect: read
    flags:
      wipe: {side_effect: destructive}
  pend: {canon: pending, side_effect: read}
  help: {canon: builtin, side_effect: read}
`

func noop(*cobra.Command, []string) error { return nil }

// fixtureTree builds the cobra tree the fixture canon describes.
func fixtureTree() *cobra.Command {
	root := &cobra.Command{Use: "nself", Short: "fixture"}
	root.PersistentFlags().Bool("json", false, "json")
	root.PersistentFlags().Bool("no-monorepo", false, "no monorepo")
	fx := &cobra.Command{Use: "fx", Short: "fixture hub"}
	echo := &cobra.Command{Use: "echo <word> [more...]", Short: "echo", RunE: noop}
	echo.Flags().String("name", "", "a name")
	echo.Flags().Int("count", 0, "a count")
	echo.Flags().Uint("limit", 0, "a limit")
	echo.Flags().Float64("ratio", 0, "a ratio")
	echo.Flags().Bool("on", false, "a switch")
	echo.Flags().StringSlice("tags", nil, "tags")
	echo.Flags().StringArray("items", nil, "items")
	echo.Flags().Duration("wait", 0, "a wait")
	echo.Flags().String("tok", "", "a token")
	echo.Flags().String("reveal", "", "reveal")
	echo.Flags().String("secretish", "", "hidden flag")
	_ = echo.Flags().MarkHidden("secretish")
	echo.Flags().String("old", "", "old flag")
	_ = echo.Flags().MarkDeprecated("old", "use name")
	echo.Flags().Bool("follow", false, "follow")
	echo.Flags().Bool("json", false, "local json")
	echo.Flags().IntSlice("nums", nil, "unsupported type")
	secretset := &cobra.Command{Use: "secretset <key> <value>", Short: "set", RunE: noop}
	secretset.Flags().String("tok", "", "t")
	hid := &cobra.Command{Use: "hid", Short: "hidden", Hidden: true, RunE: noop}
	fx.AddCommand(echo, secretset, hid)
	for _, use := range []string{"clionly", "legacy", "interactive", "streamer", "streamwrite", "remote"} {
		fx.AddCommand(&cobra.Command{Use: use, Short: use, RunE: noop})
	}
	destroy := &cobra.Command{Use: "destroy", Short: "destroy", RunE: noop}
	destroy.Flags().Bool("yes", false, "consent")
	apply := &cobra.Command{Use: "apply", Short: "apply", RunE: noop}
	apply.Flags().Bool("force", false, "consent")
	apply.Flags().Bool("plan", false, "plan form")
	apply.Flags().String("plan-id", "", "plan id")
	apply.Flags().String("target", "", "target")
	escalate := &cobra.Command{Use: "escalate", Short: "escalate", RunE: noop}
	escalate.Flags().Bool("wipe", false, "wipe")
	old := &cobra.Command{Use: "oldecho", Short: "old", Hidden: true, Deprecated: "use fx echo", RunE: noop}
	fx.AddCommand(destroy, apply, escalate, old)
	root.AddCommand(fx, &cobra.Command{Use: "pend", Short: "pending", RunE: noop})
	root.InitDefaultHelpCmd()
	return root
}

// fixtureRegistry builds the registry of the fixture tree plus one installed
// plugin command that takes free-form argv and one that declares args and flags.
func fixtureRegistry(t testing.TB) *cmdregistry.Registry {
	t.Helper()
	file, err := canon.Parse([]byte(fixtureCanon))
	if err != nil {
		t.Fatalf("fixture canon: %v", err)
	}
	root := fixtureTree()
	root.AddCommand(pluginNode("demo", "", ""), pluginNode("typed",
		`[{"name":"target","required":true},{"name":"extra","variadic":true}]`,
		`[{"name":"mode","type":"string","usage":"mode"}]`))
	types := map[string]any{}
	for _, p := range []string{"fx echo", "fx secretset", "fx clionly", "fx hid", "fx streamer", "fx streamwrite",
		"fx remote", "fx destroy", "fx apply", "fx escalate", "fx interactive", "pend"} {
		types[p] = struct{}{}
	}
	reg, err := cmdregistry.Build(root, file, types, cmdregistry.BuildOptions{V15: true})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return reg
}

// pluginNode is a mounted installed-plugin command: read, document, envelope.
func pluginNode(slug, args, flags string) *cobra.Command {
	n := &cobra.Command{Use: slug, Short: slug, RunE: noop, DisableFlagParsing: true, Annotations: map[string]string{
		"nself.plugin": slug, "nself.mount.source": "installed", "nself.side_effect": "read",
		"nself.output": "document", "nself.json": "envelope",
	}}
	if args != "" {
		n.Annotations["nself.args"] = args
	}
	if flags != "" {
		n.Annotations["nself.flags"] = flags
	}
	return n
}

func mustCmd(t testing.TB, reg *cmdregistry.Registry, path string) *cmdregistry.Command {
	t.Helper()
	c, ok := reg.Lookup(path)
	if !ok {
		t.Fatalf("fixture has no command %q", path)
	}
	return c
}

// decodeDoc parses an envelope into a generic map.
func decodeDoc(t testing.TB, b []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, b)
	}
	return doc
}

func errorOf(t *testing.T, res Result) map[string]any {
	t.Helper()
	doc := decodeDoc(t, res.Stdout)
	e, ok := doc["error"].(map[string]any)
	if !ok || doc["data"] != nil {
		t.Fatalf("want an error envelope, got %s", res.Stdout)
	}
	if doc["schema_version"] != "1" {
		t.Errorf("envelope version %v", doc["schema_version"])
	}
	return e
}

func invokeEcho(t *testing.T, doc string, o Options) Result {
	t.Helper()
	reg := fixtureRegistry(t)
	res, err := Invoke(context.Background(), reg, "fx echo", req(t, doc), o)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	return res
}

func TestInvokeTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("child signalling differs on Windows")
	}
	useStub(t, "STUBCLI_MODE", "sleep")
	start := time.Now()
	res := invokeEcho(t, `{"args":["w"]}`, Options{Timeout: time.Second})
	if took := time.Since(start); took > 6*time.Second {
		t.Errorf("child outlived the timeout by too much: %s", took)
	}
	e := errorOf(t, res)
	if e["code"] != "E423" || res.ErrorCode != "E423" || res.ExitCode != 2 || e["class"] != "infra" {
		t.Errorf("timeout: %v exit=%d", e, res.ExitCode)
	}
	if res.RequestID == "" {
		t.Error("every result carries the request id")
	}
}

func TestInvokeNonJSONStdout(t *testing.T) {
	const secret = "fixture-secret-value-0123456789"
	reg := fixtureRegistry(t)
	// The secret arg value is echoed on stderr, straddling the 2 KiB tail cut.
	stderr := strings.Repeat("x", 100) + secret + strings.Repeat("y", 2030)
	useStub(t, "STUBCLI_MODE", "text", "STUBCLI_STDERR", stderr)
	res, err := Invoke(context.Background(), reg, "fx secretset", req(t, `{"args":["k","`+secret+`"]}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	e := errorOf(t, res)
	cause, _ := e["cause"].(string)
	if e["code"] != "E422" || res.ExitCode != 2 {
		t.Fatalf("want E422/exit 2, got %v exit=%d", e, res.ExitCode)
	}
	if strings.Contains(string(res.Stdout), secret) || strings.Contains(string(res.Stdout), secret[len(secret)-10:]) {
		t.Fatalf("the secret reached the envelope: %s", res.Stdout)
	}
	if !strings.Contains(cause, Redacted) || !strings.HasSuffix(strings.TrimSpace(cause), strings.Repeat("y", 20)) || len(cause) > 4096 {
		t.Errorf("cause should be the redacted stderr tail, got %d bytes: ...%s", len(cause), tailOf([]byte(cause), 60))
	}
	// A secret flag value is scrubbed the same way, JSON-escaped spelling included.
	useStub(t, "STUBCLI_MODE", "text", "STUBCLI_STDERR", `bad token "tk\"q" and tk"q`)
	res = invokeEcho(t, `{"args":["w"],"flags":{"tok":"tk\"q"}}`, Options{})
	if c, _ := errorOf(t, res)["cause"].(string); strings.Contains(c, `tk"q`) || strings.Contains(c, `tk\"q`) || !strings.Contains(c, Redacted) {
		t.Errorf("flag secret not scrubbed: %q", c)
	}
	for name, env := range map[string][]string{
		"empty stdout":   {"STUBCLI_MODE", "raw", "STUBCLI_STDOUT", ""},
		"two documents":  {"STUBCLI_MODE", "two"},
		"array":          {"STUBCLI_MODE", "raw", "STUBCLI_STDOUT", "[1]"},
		"trailing text":  {"STUBCLI_MODE", "raw", "STUBCLI_STDOUT", `{"a":1} tail`},
		"truncated JSON": {"STUBCLI_MODE", "raw", "STUBCLI_STDOUT", `{"a":`},
		"killed by cap":  {"STUBCLI_MODE", "big"},
	} {
		useStub(t, env...)
		res := invokeEcho(t, `{"args":["w"]}`, Options{})
		if errorOf(t, res)["code"] != "E422" {
			t.Errorf("%s: %s", name, res.Stdout)
		}
	}
	useStub(t) // the real error path of a binary that cannot start
	t.Setenv(SelfExecOverrideEnv, filepath.Join(t.TempDir(), "gone"))
	if errorOf(t, invokeEcho(t, `{"args":["w"]}`, Options{}))["code"] != "E422" {
		t.Error("a child that cannot start is E422")
	}
}

func TestInvokeMetaPassthrough(t *testing.T) {
	useStub(t, "STUBCLI_MODE", "meta")
	res := invokeEcho(t, `{"args":["w"]}`, Options{})
	want := `{"schema_version":"1","command":"stub","data":{"ok":true},"meta":{"deprecations":[{"old":"a b","new":"c d","removal_at":"v1.6.0"}]}}` + "\n"
	if string(res.Stdout) != want || res.ExitCode != 0 || res.ErrorCode != "" {
		t.Errorf("meta must pass through byte for byte: %q exit=%d code=%q", res.Stdout, res.ExitCode, res.ErrorCode)
	}
	// The child's own error envelope and exit code pass through untouched.
	doc := `{"schema_version":"1","command":"fx echo","error":{"code":"E400","message":"nope","exit_code":1,"class":"user"}}`
	useStub(t, "STUBCLI_MODE", "raw", "STUBCLI_STDOUT", doc, "STUBCLI_EXIT", "1")
	res = invokeEcho(t, `{"args":["w"]}`, Options{})
	if string(res.Stdout) != doc || res.ExitCode != 1 || res.ErrorCode != "" || len(res.Stderr) != 0 {
		t.Errorf("child error envelope: %q exit=%d", res.Stdout, res.ExitCode)
	}
}

func TestInvokeEchoesArgvToChild(t *testing.T) {
	useStub(t)
	res := invokeEcho(t, `{"args":["--json","a b"],"flags":{"name":"v;$(id)","on":true}}`, Options{Dir: t.TempDir()})
	var d stubData
	if err := json.Unmarshal(res.Stdout, &d); err != nil {
		t.Fatalf("%v: %s", err, res.Stdout)
	}
	want := []string{"fx", "echo", "--json", "--name=v;$(id)", "--on", "--", "--json", "a b"}
	if !reflect.DeepEqual(d.Data.Argv, want) {
		t.Errorf("child argv %q, want %q", d.Data.Argv, want)
	}
}

func TestInvokeRefusals(t *testing.T) {
	// A refusal never starts a child: point the invoker at a binary that would fail.
	useStub(t)
	t.Setenv(SelfExecOverrideEnv, filepath.Join(t.TempDir(), "must-not-run"))
	reg := fixtureRegistry(t)
	cases := []struct {
		name, path, doc, code, mention string
		exit                           int
	}{
		{"unknown path", "fx nope", `{}`, "E432", "", 1},
		{"cli-only", "fx clionly", `{}`, "E421", "cli-only", 1},
		{"hidden", "fx hid", `{}`, "E421", "hidden", 1},
		{"legacy json", "fx legacy", `{}`, "E421", "envelope", 1},
		{"remote", "fx remote", `{}`, "E421", "no gate", 1},
		{"destructive", "fx destroy", `{"confirm":"` + strings.Repeat("a", 64) + `"}`, "E421", "no gate", 1},
		{"flag-escalated destructive", "fx escalate", `{"flags":{"wipe":true}}`, "E421", "no gate", 1},
		{"stream over mcp", "fx streamer", `{}`, "E421", "NDJSON", 1},
		{"stream override flag", "fx echo", `{"args":["w"],"flags":{"follow":true}}`, "E421", "NDJSON", 1},
		{"unknown flag", "fx echo", `{"args":["w"],"flags":{"zzz":1}}`, "E420", "zzz", 1},
		{"consent flag is invoker-owned", "fx apply", `{"flags":{"force":true}}`, "E420", "force", 1},
		{"plan id flag is invoker-owned", "fx apply", `{"flags":{"plan-id":"` + strings.Repeat("a", 64) + `"}}`, "E420", "plan-id", 1},
		{"missing arg", "fx echo", `{}`, "E420", "too few", 1},
		{"path with root prefix works", "nself fx clionly", `{}`, "E421", "cli-only", 1},
	}
	for _, c := range cases {
		res, err := Invoke(context.Background(), reg, c.path, req(t, c.doc), Options{})
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		e := errorOf(t, res)
		why, _ := e["cause"].(string)
		msg, _ := e["message"].(string)
		if e["code"] != c.code || res.ErrorCode != c.code || res.ExitCode != c.exit || !strings.Contains(why+msg, c.mention) || len(res.RequestID) != 64 {
			t.Errorf("%s: code=%v exit=%d id=%q cause=%q", c.name, e["code"], res.ExitCode, res.RequestID, why)
		}
		if doc := decodeDoc(t, res.Stdout); doc["command"] != strings.TrimPrefix(c.path, "nself ") {
			t.Errorf("%s: envelope command %v", c.name, doc["command"])
		}
	}
	// A secret value that fails validation is never echoed back.
	res, _ := Invoke(context.Background(), reg, "fx echo", req(t, `{"args":["w"],"flags":{"tok":987654321}}`), Options{})
	if errorOf(t, res)["code"] != "E420" || strings.Contains(string(res.Stdout), "987654321") {
		t.Errorf("secret echoed in an E420: %s", res.Stdout)
	}
	if res, err := Invoke(context.Background(), nil, "fx echo", Request{}, Options{}); err != nil || errorOf(t, res)["code"] != "E432" {
		t.Errorf("a nil registry has no commands: %v %s", err, res.Stdout)
	}
}

func TestInvokeStreamAndCancel(t *testing.T) {
	useStub(t, "STUBCLI_MODE", "stream")
	reg := fixtureRegistry(t)
	var sink bytes.Buffer
	res, err := Invoke(context.Background(), reg, "fx streamer", Request{}, Options{Transport: TransportHTTPStream, Stdout: &sink, Timeout: time.Nanosecond})
	if err != nil || res.ExitCode != 0 || strings.Count(sink.String(), "\n") != 3 || res.ErrorCode != "" {
		t.Errorf("ndjson: err=%v exit=%d sink=%q (an NDJSON request has no timeout)", err, res.ExitCode, sink.String())
	}
	if runtime.GOOS == "windows" {
		return
	}
	useStub(t, "STUBCLI_MODE", "sleep")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := Invoke(ctx, reg, "fx echo", req(t, `{"args":["w"]}`), Options{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("client gone: want the context error, got %v", err)
	}
}

// TestInvokeRealBinary builds ./cmd/nself and proves Invoke returns the same
// bytes and exit code as the CLI itself on a fixture project.
func TestInvokeRealBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the real binary")
	}
	bin := filepath.Join(t.TempDir(), "nself")
	build := exec.Command("go", "build", "-mod=vendor", "-o", bin, "../../cmd/nself")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building nself: %v\n%s", err, out)
	}
	home, project := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_ALLOW_SOURCE_DIR", "1")
	t.Setenv("NSELF_PLUGIN_DIR", filepath.Join(home, "plugins"))
	t.Setenv(SelfExecOverrideEnv, bin)
	initCmd := exec.Command(bin, "init", "--non-interactive", "--quiet")
	initCmd.Dir = project
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("nself init: %v\n%s", err, out)
	}
	reg := configRegistry(t)
	for _, c := range []struct {
		name, doc string
		direct    []string
		wantCode  int
	}{
		{"known key", `{"args":["BASE_DOMAIN"]}`, []string{"config", "get", "BASE_DOMAIN", "--json"}, 0},
		{"missing key", `{"args":["NOT_A_KEY_AT_ALL"]}`, []string{"config", "get", "NOT_A_KEY_AT_ALL", "--json"}, 1},
		{"arg that looks like a flag", `{"args":["--no-such-flag-x"]}`, []string{"config", "get", "--json", "--", "--no-such-flag-x"}, 1},
	} {
		direct := exec.Command(bin, c.direct...)
		direct.Dir = project
		direct.Env = append(os.Environ(), "NSELF_V15=1")
		var out bytes.Buffer
		direct.Stdout = &out
		code := 0
		if err := direct.Run(); err != nil {
			var ee *exec.ExitError
			if !errors.As(err, &ee) {
				t.Fatal(err)
			}
			code = ee.ExitCode()
		}
		res, err := Invoke(context.Background(), reg, "config get", req(t, c.doc), Options{Dir: project})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(res.Stdout, out.Bytes()) || res.ExitCode != code || code != c.wantCode || res.ErrorCode != "" {
			t.Errorf("%s: invoke exit=%d code=%q\n%s\ndirect exit=%d\n%s", c.name, res.ExitCode, res.ErrorCode, res.Stdout, code, out.Bytes())
		}
	}
}

// configRegistry is a one-command registry for `config get <key>`.
func configRegistry(t *testing.T) *cmdregistry.Registry {
	t.Helper()
	file, err := canon.Parse([]byte("schema_version: 1\nverbs: [config]\ncommands:\n  config: {canon: core}\n  config get: {side_effect: read}\n  help: {canon: builtin, side_effect: read}\n"))
	if err != nil {
		t.Fatal(err)
	}
	root := &cobra.Command{Use: "nself", Short: "fixture"}
	cfg := &cobra.Command{Use: "config", Short: "config"}
	cfg.AddCommand(&cobra.Command{Use: "get <key>", Short: "get", RunE: noop})
	root.AddCommand(cfg)
	root.InitDefaultHelpCmd()
	reg, err := cmdregistry.Build(root, file, map[string]any{"config get": struct{}{}}, cmdregistry.BuildOptions{V15: true})
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestConfirmationMatchesSchema(t *testing.T) {
	b, err := json.Marshal(Confirmation{ConfirmationRequired: true, Kind: "request", Confirm: strings.Repeat("a", 64), SideEffect: "destructive", Reason: "r"})
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := schemas.Lookup("invoke/confirmation.v1.schema.json")
	if !ok {
		t.Fatal("confirmation schema is not embedded")
	}
	var sch struct {
		Required   []string                  `json:"required"`
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(raw, &sch); err != nil {
		t.Fatal(err)
	}
	last := -1
	for _, name := range sch.Required {
		at := strings.Index(string(b), `"`+name+`"`)
		if at <= last || sch.Properties[name] == nil {
			t.Errorf("member %s is out of contract order or missing from the schema in %s", name, b)
		}
		last = at
	}
	if len(sch.Required) != 6 || !strings.Contains(string(b), `"plan":null`) {
		t.Errorf("required=%v doc=%s", sch.Required, b)
	}
}
