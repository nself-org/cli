package mcpgen_test

// Purpose: a small registry with every shape the generator must handle: the
// alias targets of legacy_aliases.yaml (so the dual-shape and alias tests run
// against real flag and argument names), a stream flag, gated commands, an
// installed plugin with the sentry-cli commands, and a plugin subcommand with
// no declared side effect.
// Constraints: built with cmdregistry.Build the way internal/invoke builds its
// fixture; plugin nodes carry the mount annotations; nothing touches the disk.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/mcpgen"
	"github.com/nself-org/cli/schemas"
	"github.com/spf13/cobra"
)

const fixtureCanon = `
schema_version: 1
verbs: [status, logs, config, doctor, build, db, backup, deploy, stop, start, restart, add, foo-bar]
commands:
  help: {canon: builtin, side_effect: read}
  help capabilities: {side_effect: read}
  status: {canon: core, side_effect: read}
  status urls: {side_effect: read}
  foo-bar: {canon: core, side_effect: read}
  logs: {canon: core, side_effect: read, flags: {follow: {output: stream}}}
  config: {canon: core}
  config get: {side_effect: read}
  config show: {side_effect: read}
  config set: {side_effect: destructive, confirm: {flags: [yes]}}
  config env: {}
  config env list: {side_effect: read}
  config services: {}
  config services list: {side_effect: read, json: legacy}
  doctor: {canon: core, side_effect: read}
  build:
    canon: core
    side_effect: write
    flags: {wipe: {side_effect: destructive}}
    confirm: {flags: [yes], plan: {flag: plan, id_flag: plan-id}}
  stop: {canon: core, side_effect: write, json: none}
  start: {canon: core, side_effect: write, json: none}
  restart: {canon: core, side_effect: write, json: none}
  add: {canon: core, side_effect: remote, json: none}
  config plugins: {}
  config plugins list: {side_effect: read, json: none}
  db: {canon: core}
  db migrate: {}
  db migrate status: {side_effect: read}
  db hasura: {}
  db hasura schema: {side_effect: read}
  db hasura permissions: {side_effect: read}
  backup: {canon: core}
  backup list: {side_effect: read}
  deploy: {canon: core}
  deploy status: {side_effect: read}
`

func noop(*cobra.Command, []string) error { return nil }

func leaf(use string, flags func(*cobra.Command)) *cobra.Command {
	c := &cobra.Command{Use: use, Short: "fixture " + use, RunE: noop}
	if flags != nil {
		flags(c)
	}
	return c
}

func hub(use string, kids ...*cobra.Command) *cobra.Command {
	c := &cobra.Command{Use: use, Short: "fixture " + use}
	c.AddCommand(kids...)
	return c
}

// pluginNode is a mounted installed-plugin command (envelope, document).
func pluginNode(slug, side string, args, flags string, kids ...*cobra.Command) *cobra.Command {
	n := &cobra.Command{Use: slug, Short: "plugin " + slug, RunE: noop, DisableFlagParsing: true, Annotations: map[string]string{
		"nself.plugin": "sentry-cli", "nself.mount.source": "installed", "nself.output": "document", "nself.json": "envelope",
	}}
	if side != "" {
		n.Annotations["nself.side_effect"] = side
	}
	if args != "" {
		n.Annotations["nself.args"] = args
	}
	if flags != "" {
		n.Annotations["nself.flags"] = flags
	}
	n.AddCommand(kids...)
	return n
}

// fixtureRegistry builds the v1.5 registry. withSentry mounts the sentry-cli
// commands; withFixturePlugin mounts the plugin with the undeclared command.
func fixtureRegistry(t testing.TB, withSentry, withFixturePlugin bool) *cmdregistry.Registry {
	t.Helper()
	reg, err := fixtureBuild(t, withSentry, withFixturePlugin)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return reg
}

// fixtureBuild is fixtureRegistry with extra top-level nodes, returning the
// builder's error (the adversarial suite feeds it hostile plugin nodes).
func fixtureBuild(t testing.TB, withSentry, withFixturePlugin bool, extra ...*cobra.Command) (*cmdregistry.Registry, error) {
	t.Helper()
	file, err := canon.Parse([]byte(fixtureCanon))
	if err != nil {
		t.Fatalf("fixture canon: %v", err)
	}
	root := &cobra.Command{Use: "nself", Short: "fixture"}
	root.PersistentFlags().Bool("json", false, "json")
	root.PersistentFlags().Bool("no-monorepo", false, "no monorepo")
	yes := func(c *cobra.Command) { c.Flags().Bool("yes", false, "consent") }
	root.AddCommand(
		leaf("status", nil), leaf("foo-bar", nil), leaf("doctor", nil), leaf("stop", nil), leaf("start", nil), leaf("restart", nil), leaf("add [name]", nil),
		leaf("logs [service]", func(c *cobra.Command) {
			c.Flags().Int("tail", 10, "lines")
			c.Flags().Bool("follow", false, "follow")
		}),
		leaf("build", func(c *cobra.Command) {
			yes(c)
			c.Flags().Bool("plan", false, "plan form")
			c.Flags().String("plan-id", "", "plan id")
			c.Flags().Bool("wipe", false, "wipe")
		}),
		hub("config",
			leaf("get <key>", nil), leaf("show", nil), leaf("set <key> <value>", yes),
			hub("env", leaf("list", nil)), hub("services", leaf("list", nil)), hub("plugins", leaf("list", func(c *cobra.Command) { c.Flags().Bool("installed", false, "installed only") }))),
		hub("db", hub("migrate", leaf("status", nil)), hub("hasura", leaf("schema", nil), leaf("permissions", nil))),
		hub("backup", leaf("list", nil)), hub("deploy", leaf("status", nil)),
	)
	if st, _, _ := root.Find([]string{"status"}); st != nil {
		st.AddCommand(leaf("urls", nil))
	}
	if withSentry {
		root.AddCommand(pluginNode("sentry", "read", "", "", pluginNode("monitors", "read", "", "",
			pluginNode("list", "read", "", ""),
			pluginNode("add", "write", "", `[{"name":"url","type":"string"},{"name":"name","type":"string"},{"name":"kind","type":"string"},{"name":"interval","type":"duration"}]`)),
			pluginNode("incidents", "read", "", "",
				pluginNode("list", "read", "", `[{"name":"status","type":"string"}]`),
				pluginNode("ack", "write", `[{"name":"id","required":true}]`, "")),
			pluginNode("status", "read", "", `[{"name":"url","type":"string"}]`)))
	}
	if withFixturePlugin {
		undeclared := pluginNode("undeclared", "", `[{"name":"x"}]`, "")
		long := pluginNode(strings.Repeat("verylongcommand", 5), "read", "", "") // nself_fixp_ + 75 characters
		root.AddCommand(pluginNode("fixp", "read", "", "", pluginNode("reader", "read", `[{"name":"x"}]`, ""), undeclared, long))
	}
	root.AddCommand(extra...)
	root.InitDefaultHelpCmd()
	if h, _, err := root.Find([]string{"help"}); err == nil && h != nil && h.Name() == "help" {
		h.AddCommand(leaf("capabilities <query>...", func(c *cobra.Command) { c.Flags().Int("limit", 5, "maximum hits") }))
	}
	types := map[string]any{}
	for _, p := range []string{"help capabilities", "foo-bar", "status", "status urls", "logs", "config get", "config show", "config set", "config env list",
		"doctor", "build", "db migrate status", "db hasura schema", "db hasura permissions", "backup list", "deploy status"} {
		types[p] = struct{}{}
	}
	return cmdregistry.Build(root, file, types, cmdregistry.BuildOptions{V15: true})
}

// harness runs generated tools against the stub child.
type harness struct {
	t      *testing.T
	tools  map[string]server.ServerTool
	reg    *cmdregistry.Registry
	guard  *invoke.Guard
	argv   string
	invoke mcpgen.Invoker
}

func newHarness(t *testing.T, v15, sentry, plug bool, stubKV ...string) *harness {
	t.Helper()
	t.Setenv(invoke.SelfExecOverrideEnv, stubPath)
	t.Setenv("STUBCLI_MODE", "")
	for i := 0; i+1 < len(stubKV); i += 2 {
		t.Setenv(stubKV[i], stubKV[i+1])
	}
	h := &harness{t: t, reg: fixtureRegistry(t, sentry, plug), guard: invoke.NewGuard(t.TempDir() + "/audit.jsonl")}
	h.invoke = func(ctx context.Context, path string, req invoke.Request) (invoke.Result, error) {
		return invoke.Invoke(ctx, h.reg, path, req, invoke.Options{Surface: "mcp-stdio", Guard: h.guard, Dir: t.TempDir()})
	}
	list, err := mcpgen.Tools(mcpgen.Options{Registry: h.reg, Invoke: h.invoke, Schema: schemas.Lookup, V15: v15})
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	h.tools = map[string]server.ServerTool{}
	for _, st := range list {
		if _, dup := h.tools[st.Tool.Name]; dup {
			t.Fatalf("tool %s listed twice", st.Tool.Name)
		}
		h.tools[st.Tool.Name] = st
	}
	return h
}

// call invokes a tool with a JSON arguments object.
func (h *harness) call(name, args string) *mcp.CallToolResult {
	h.t.Helper()
	st, ok := h.tools[name]
	if !ok {
		h.t.Fatalf("no tool %s", name)
	}
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	if args != "" {
		req.Params.RawArguments = json.RawMessage(args)
		req.Params.Arguments = json.RawMessage(args)
	}
	res, err := st.Handler(context.Background(), req)
	if err != nil {
		h.t.Fatalf("%s: handler error %v", name, err)
	}
	return res
}

// envelope decodes the text content of a result.
func envelope(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if len(res.Content) != 1 {
		t.Fatalf("content %+v", res.Content)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &doc); err != nil {
		t.Fatalf("text content is not an envelope: %v", err)
	}
	return doc
}

func errCode(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if !res.IsError {
		t.Fatalf("want an error result, got %+v", res)
	}
	e, _ := envelope(t, res)["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}
