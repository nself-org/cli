package mcpgen_test

// Purpose: the legacy-name table, the dual-shape merged tools and the aliases.
// Constraints: parameter maps are proven twice: Row.Request against the table
// (independent of exposure) and the argv a call produces against the stub child.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nself-org/cli/internal/mcpgen"
)

// legacyNames are the 26 hand tool names of cmd/commands/mcp.go and mcp_sentry.go.
var legacyNames = []string{
	"nself_status", "nself_doctor", "nself_urls", "nself_logs", "nself_service_list", "nself_env_list",
	"nself_config_get", "nself_config_show", "nself_config_set", "nself_build", "nself_start", "nself_stop",
	"nself_restart", "nself_db_migrate_status", "nself_backup_list", "nself_deploy_status", "nself_plugin_list",
	"nself_plugin_install", "nself_get_schema", "nself_get_permissions", "nself_run_migration",
	"sentry_monitors_list", "sentry_monitors_add", "sentry_incidents_list", "sentry_incidents_ack", "sentry_status",
}

func TestLegacyAliasTable(t *testing.T) {
	table, err := mcpgen.LoadTable()
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Rows) != 26 {
		t.Fatalf("%d rows, want 26", len(table.Rows))
	}
	kinds := map[string]int{}
	for _, n := range legacyNames {
		r, ok := table.Lookup(n)
		if !ok {
			t.Errorf("no row for %s", n)
			continue
		}
		kinds[r.Kind]++
		for _, p := range r.Params {
			if r.Kind == mcpgen.KindMerged && (p.Name == "args" || p.Name == "flags" || p.Name == "argv" || p.Name == "confirm") {
				t.Errorf("%s: legacy property %s collides with a machine member", n, p.Name)
			}
		}
	}
	if kinds[mcpgen.KindMerged] != 13 || kinds[mcpgen.KindAlias] != 12 || kinds[mcpgen.KindRemoved] != 1 {
		t.Errorf("kinds %v, want 13 merged, 12 alias, 1 removed", kinds)
	}

	// Real registry: every target path exists, every mapped flag exists and
	// every mapped positional has a slot; the merged name equals the target's generated name.
	reg := hermeticRegistry(t)
	fx := fixtureRegistry(t, true, false)
	for _, r := range table.Rows {
		if r.Target == "" {
			continue
		}
		use := reg
		if r.Plugin != "" {
			use = fx
		}
		cmd, ok := use.Lookup(r.Target)
		if !ok {
			t.Errorf("%s: target %q is not in the v1.5 registry", r.Name, r.Target)
			continue
		}
		if r.Kind == mcpgen.KindMerged && r.Name != toolNameOf(r.Target) {
			t.Errorf("%s: merged row but the target's generated name is %s", r.Name, toolNameOf(r.Target))
		}
		for _, f := range r.Flags {
			found := false
			for _, cf := range cmd.Flags {
				found = found || cf.Name == f.Flag
			}
			if !found {
				t.Errorf("%s: flag %q does not exist on %q", r.Name, f.Flag, r.Target)
			}
		}
		if len(r.Args) > len(cmd.Args) && !(len(cmd.Args) > 0 && cmd.Args[len(cmd.Args)-1].Variadic) {
			t.Errorf("%s: %d positional values but %q takes %d", r.Name, len(r.Args), r.Target, len(cmd.Args))
		}
	}

	// Parameter maps, one row each.
	for _, c := range []struct{ name, in, want string }{
		{"nself_logs", `{"service":"db","lines":7}`, `{"args":["db"],"flags":{"tail":7}}`},
		{"nself_logs", `{"lines":9}`, `{"flags":{"tail":9}}`},
		{"nself_logs", `{"service":"db"}`, `{"args":["db"],"flags":{"tail":50}}`},
		{"nself_config_get", `{"key":"A"}`, `{"args":["A"]}`},
		{"nself_config_show", `{}`, `{}`},
		{"nself_config_set", `{"key":"A","value":"b"}`, `{"args":["A","b"]}`},
		{"nself_urls", `{}`, `{}`},
		{"nself_service_list", `{}`, `{}`},
		{"nself_env_list", `{}`, `{}`},
		{"nself_plugin_list", `{"installed":true}`, `{"flags":{"installed":true}}`},
		{"nself_plugin_install", `{"name":"mlflow"}`, `{"args":["mlflow"]}`},
		{"nself_get_schema", `{}`, `{}`},
		{"nself_get_permissions", `{}`, `{}`},
		{"sentry_monitors_list", `{}`, `{}`},
		{"sentry_monitors_add", `{"url":"https://a.test","name":"A","kind":"tcp","interval_seconds":30}`,
			`{"flags":{"interval":"30s","kind":"tcp","name":"A","url":"https://a.test"}}`},
		{"sentry_incidents_list", `{"status":"open"}`, `{"flags":{"status":"open"}}`},
		{"sentry_incidents_ack", `{"id":"i1"}`, `{"args":["i1"]}`},
		{"sentry_status", `{"url":"https://s.test/status.json"}`, `{"flags":{"url":"https://s.test/status.json"}}`},
	} {
		r, _ := table.Lookup(c.name)
		members := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(c.in), &members); err != nil {
			t.Fatal(err)
		}
		req, err := r.Request(members, false)
		if err != nil {
			t.Errorf("%s %s: %v", c.name, c.in, err)
			continue
		}
		got, _ := json.Marshal(req)
		if string(got) != c.want {
			t.Errorf("%s %s\n got %s\nwant %s", c.name, c.in, got, c.want)
		}
	}
	// Required members, types and unsupported members are E420 naming the member.
	for _, c := range []struct{ name, in, mention string }{
		{"nself_config_get", `{"env":"prod"}`, "key"},
		{"nself_config_set", `{"key":"A"}`, "value"},
		{"nself_config_get", `{"key":"A","env":"prod"}`, "env"},
		{"nself_logs", `{"lines":"many"}`, "lines"},
		{"nself_logs", `{"lines":1.5}`, "lines"},
		{"nself_plugin_install", `{}`, "name"},
		{"nself_urls", `{"x":1}`, "x"},
		{"sentry_incidents_ack", `{"id":""}`, "id"},
	} {
		r, _ := table.Lookup(c.name)
		members := map[string]json.RawMessage{}
		_ = json.Unmarshal([]byte(c.in), &members)
		_, err := r.Request(members, false)
		if err == nil || !strings.Contains(err.Error(), c.mention) || !strings.Contains(err.Error(), "E420") {
			t.Errorf("%s %s: error %v is not an E420 naming %q", c.name, c.in, err, c.mention)
		}
	}
}

func toolNameOf(path string) string {
	return "nself_" + strings.NewReplacer(" ", "_", "-", "_").Replace(path)
}

func TestMergedToolShapes(t *testing.T) {
	h := newHarness(t, true, true, false)
	for _, n := range legacyNames {
		if _, ok := h.tools[n]; !ok {
			t.Errorf("legacy name %s does not resolve in v1.5 mode", n)
		}
	}
	deprecations := func(res *mcp.CallToolResult) []any {
		meta, _ := envelope(t, res)["meta"].(map[string]any)
		d, _ := meta["deprecations"].([]any)
		return d
	}
	// {} is the machine shape: no legacy default, no deprecation.
	res := h.call("nself_logs", `{}`)
	if res.IsError || len(deprecations(res)) != 0 || contains(argvOf(t, res), "--tail=50") {
		t.Fatalf("machine shape: %v", argvOf(t, res))
	}
	// A legacy property makes the legacy shape: the legacy default and a deprecation note.
	res = h.call("nself_logs", `{"service":"db"}`)
	argv := argvOf(t, res)
	if res.IsError || !contains(argv, "--tail=50") || argv[len(argv)-1] != "db" || len(deprecations(res)) != 1 {
		t.Fatalf("legacy shape: %v %v", argv, deprecations(res))
	}
	if d := deprecations(res)[0].(map[string]any); d["removal_at"] != "v1.6.0" {
		t.Errorf("deprecation %v", d)
	}
	// Both shapes return equal data.
	legacy := argvOf(t, h.call("nself_config_get", `{"key":"A"}`))
	machine := argvOf(t, h.call("nself_config_get", `{"args":["A"]}`))
	if strings.Join(legacy, " ") != strings.Join(machine, " ") {
		t.Errorf("legacy argv %v, machine argv %v", legacy, machine)
	}
	if len(deprecations(h.call("nself_config_get", `{"args":["A"]}`))) != 0 {
		t.Error("the machine shape carries a deprecation")
	}
	// Mixing shapes, a missing required member, an unsupported one: E420.
	for _, in := range []string{`{"key":"A","args":["A"]}`, `{"key":"A","flags":{}}`, `{"env":"prod"}`, `{"key":"A","env":"prod"}`} {
		res := h.call("nself_config_get", in)
		if errCode(t, res) != "E420" {
			t.Errorf("%s returned %s", in, errCode(t, res))
		}
	}
	if msg := envelope(t, h.call("nself_config_get", `{"env":"prod"}`))["error"].(map[string]any)["message"].(string); !strings.Contains(msg, "key") {
		t.Errorf("message %q does not name key", msg)
	}
	// A legacy call that needs confirmation can still confirm.
	res = h.call("nself_config_set", `{"key":"A","value":"b"}`)
	sc, _ := res.StructuredContent.(map[string]any)
	if res.IsError || sc["confirmation_required"] != true {
		t.Fatalf("legacy config_set: %+v", res)
	}
	res = h.call("nself_config_set", `{"key":"A","value":"b","confirm":"`+sc["confirm"].(string)+`"}`)
	if res.IsError || !contains(argvOf(t, res), "--yes=true") {
		t.Fatalf("confirmed legacy config_set: %+v", res)
	}
}

func TestAliasCalls(t *testing.T) {
	h := newHarness(t, true, true, false)
	res := h.call("nself_urls", `{}`)
	argv := argvOf(t, res)
	if res.IsError || argv[0] != "status" || argv[1] != "urls" {
		t.Fatalf("nself_urls: %v", argv)
	}
	if meta, _ := envelope(t, res)["meta"].(map[string]any); meta == nil {
		t.Error("an alias call carries no deprecation")
	}
	for _, n := range []string{"nself_urls", "nself_env_list", "nself_get_schema", "nself_get_permissions", "nself_run_migration", "sentry_status"} {
		if d := h.tools[n].Tool.Description; !strings.HasPrefix(d, mcpgen.DeprecatedPrefix) {
			t.Errorf("%s description %q does not start with Deprecated:", n, d)
		}
	}
	// A target that is not exposed yet refuses at the invoker, fail closed.
	if code := errCode(t, h.call("nself_plugin_install", `{"name":"mlflow"}`)); code != "E421" {
		t.Errorf("nself_plugin_install returned %s", code)
	}
	// Sentry maps onto the mounted plugin commands.
	res = h.call("sentry_monitors_add", `{"url":"https://a.test","interval_seconds":30}`)
	argv = argvOf(t, res)
	if res.IsError || len(argv) < 3 || strings.Join(argv[:3], " ") != "sentry monitors add" || !contains(argv, "--interval=30s") || !contains(argv, "--url=https://a.test") {
		t.Fatalf("sentry_monitors_add: %v %+v", argv, res)
	}
	res = h.call("sentry_incidents_ack", `{"id":"abc"}`)
	if argv = argvOf(t, res); len(argv) < 2 || argv[len(argv)-1] != "abc" || argv[len(argv)-2] != "--" {
		t.Errorf("sentry_incidents_ack: %v", argv)
	}
	// nself_run_migration is removed: E433, fail closed, nothing runs (the stub would sleep).
	h2 := newHarness(t, true, true, false, "STUBCLI_MODE", "sleep")
	res = h2.call("nself_run_migration", `{"sql":"SELECT 1","confirm":true}`)
	if errCode(t, res) != "E433" {
		t.Fatalf("nself_run_migration: %+v", res)
	}
	if !strings.Contains(res.Content[0].(mcp.TextContent).Text, "db migrate create") {
		t.Error("E433 does not name nself db migrate create")
	}
}

func TestAliasWithoutPlugin(t *testing.T) {
	h := newHarness(t, true, false, false)
	for _, n := range []string{"sentry_monitors_list", "sentry_monitors_add", "sentry_incidents_list", "sentry_incidents_ack", "sentry_status"} {
		args := `{}`
		switch n {
		case "sentry_monitors_add":
			args = `{"url":"https://a.test"}`
		case "sentry_incidents_ack":
			args = `{"id":"1"}`
		}
		res := h.call(n, args)
		if code := errCode(t, res); code != "E404" {
			t.Errorf("%s returned %s", n, code)
		}
		if !strings.Contains(res.Content[0].(mcp.TextContent).Text, "nself add sentry-cli") {
			t.Errorf("%s does not name nself add sentry-cli", n)
		}
	}
}

func TestLegacyShapeHardening(t *testing.T) {
	h := newHarness(t, true, false, false)
	// A repeated member is refused, not resolved to the last value.
	if code := errCode(t, h.call("nself_config_get", `{"key":"A","key":"B"}`)); code != "E420" {
		t.Errorf("repeated member returned %s", code)
	}
	for _, in := range []string{`[]`, `"x"`, `7`} {
		if code := errCode(t, h.call("nself_config_get", in)); code != "E420" {
			t.Errorf("arguments %s returned %s", in, code)
		}
	}
	// Adding deprecation notes keeps the child's bytes: < > & are not escaped.
	t.Setenv("STUBCLI_MODE", "raw")
	t.Setenv("STUBCLI_STDOUT", `{"schema_version":"1","command":"stub","data":{"v":"<b>&</b>"}}`)
	res := h.call("nself_config_get", `{"key":"A"}`)
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, `"<b>&</b>"`) || !strings.Contains(text, "deprecations") {
		t.Errorf("text content %s", text)
	}
}
