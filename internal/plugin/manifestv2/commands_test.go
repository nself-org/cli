package manifestv2_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

func sub(d map[string]any, i int) map[string]any {
	return d["commands"].(map[string]any)["subcommands"].([]any)[i].(map[string]any)
}

func TestCommandsValidation(t *testing.T) {
	cases := []struct {
		name, field string
		edit        func(d map[string]any)
	}{
		{"command", "commands.command", func(d map[string]any) { d["commands"].(map[string]any)["command"] = "Bad_Name" }},
		{"binary", "commands.binary", func(d map[string]any) { d["commands"].(map[string]any)["binary"] = "surfacer" }},
		{"segment", "commands.subcommands[1].name", func(d map[string]any) { sub(d, 1)["name"] = "server List" }},
		{"empty-segment", "commands.subcommands[1].name", func(d map[string]any) { sub(d, 1)["name"] = "server  list" }},
		{"side-effect", "commands.side_effect", func(d map[string]any) { d["commands"].(map[string]any)["side_effect"] = "nuke" }},
		{"duplicate", "commands.subcommands[1].name", func(d map[string]any) { sub(d, 1)["name"] = "apply" }},
	}
	for _, c := range cases {
		_, err := manifestv2.Parse(mutate(t, "v2/surface.json", c.edit))
		wantCode(t, err, "E106", c.field)
	}
	m, err := manifestv2.Load("testdata/v2/surface.json")
	if err != nil {
		t.Fatal(err)
	}
	c := m.Commands
	if c.Summary != "Surface fixture" || c.SideEffect != "write" || c.Output != "document" || c.JSON != "legacy" {
		t.Fatalf("optional root fields did not decode: %+v", c)
	}
}

func TestCommandsV1Mapping(t *testing.T) {
	load := func(n string) *manifestv2.Manifest {
		m, err := manifestv2.Load("testdata/v1/" + n + ".json")
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	if m := load("sentry-cli"); m.Commands == nil || m.Commands.Command != "sentry" || m.Commands.Binary != "nself-sentry" {
		t.Errorf("sentry-cli: %+v", m.Commands)
	} else if len(m.CLICommands) != 2 || m.CLICommands[1].Name != "sentry-server" {
		t.Errorf("sentry-server must stay compatibility-only: %+v", m.CLICommands)
	}
	if m := load("tenant"); m.Commands == nil || m.Commands.Command != "tenant" || len(m.CLICommands) != 2 || m.CLICommands[1].Name != "billing" {
		t.Errorf("tenant: %+v %+v", m.Commands, m.CLICommands)
	}
	if m := load("vpn"); m.Commands != nil || len(m.CLICommands) == 0 {
		t.Errorf("vpn (binaryName null) must map to commands: null and keep its cliCommands: %+v", m.Commands)
	}
	if m := load("ci"); m.Commands != nil {
		t.Errorf("ci publishes no binary: %+v", m.Commands)
	}
}

func TestSurfaceFields(t *testing.T) {
	m, err := manifestv2.Load("testdata/v2/surface.json")
	if err != nil {
		t.Fatal(err)
	}
	c := m.Commands
	if c.Surface != "cli-only" || c.Confirm == nil || c.Confirm.Flags == nil || len(c.Confirm.Flags) != 0 {
		t.Fatalf("root surface/confirm: %+v", c)
	}
	apply := c.Subcommands[0]
	if !apply.Args[0].Secret || !apply.Flags[2].Secret || !apply.Flags[2].CLIOnly ||
		!reflect.DeepEqual(apply.Confirm.Flags, []string{"yes"}) || apply.Confirm.Plan.IDFlag != "plan-id" {
		t.Fatalf("apply: %+v", apply)
	}
	out, err := manifestv2.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	again, err := manifestv2.Parse(out)
	if err != nil || !reflect.DeepEqual(m, again) || again.Commands.Confirm.Flags == nil {
		t.Fatalf("round trip lost surface fields: %v", err)
	}
	raw, err := os.ReadFile("../../../schemas/plugin-manifest.v2.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	subProps := doc["properties"].(map[string]any)["commands"].(map[string]any)["properties"].(map[string]any)["subcommands"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	for _, k := range []string{"confirm", "surface"} {
		if _, ok := subProps[k]; !ok {
			t.Errorf("schema subcommand lacks %q", k)
		}
	}
	args := subProps["args"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	flags := subProps["flags"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	for _, k := range []string{"secret"} {
		if args[k] == nil || flags[k] == nil {
			t.Errorf("schema arg/flag lacks %q", k)
		}
	}
	if flags["cli_only"] == nil {
		t.Error("schema flag lacks cli_only")
	}
}

func TestConfirmFlagValidation(t *testing.T) {
	cases := []struct {
		name, field string
		edit        func(s map[string]any)
	}{
		{"undeclared", "commands.subcommands[0].confirm.flags[0]", func(s map[string]any) { s["confirm"].(map[string]any)["flags"] = []any{"nope"} }},
		{"not-bool", "commands.subcommands[0].confirm.flags[0]", func(s map[string]any) { s["confirm"].(map[string]any)["flags"] = []any{"plan-id"} }},
		{"plan-flag", "commands.subcommands[0].confirm.plan.flag", func(s map[string]any) { s["confirm"].(map[string]any)["plan"].(map[string]any)["flag"] = "nope" }},
		{"id-flag-not-string", "commands.subcommands[0].confirm.plan.id_flag", func(s map[string]any) { s["confirm"].(map[string]any)["plan"].(map[string]any)["id_flag"] = "yes" }},
	}
	for _, c := range cases {
		data := mutate(t, "v2/surface.json", func(d map[string]any) { c.edit(sub(d, 0)) })
		_, err := manifestv2.Parse(data)
		wantCode(t, err, "E106", c.field)
	}
	root := mutate(t, "v2/surface.json", func(d map[string]any) {
		d["commands"].(map[string]any)["confirm"] = map[string]any{"flags": []any{"yes"}}
	})
	_, err := manifestv2.Parse(root)
	wantCode(t, err, "E106", "commands.confirm.flags[0]")
	if _, err := manifestv2.Load("testdata/v2/surface.json"); err != nil {
		t.Fatalf("empty confirm.flags must be valid: %v", err)
	}
}

func TestCoreVerbsMatchCanon(t *testing.T) {
	f, err := canon.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.Verbs, manifestv2.CoreVerbs) {
		t.Fatalf("manifestv2.CoreVerbs %v != canon verbs %v", manifestv2.CoreVerbs, f.Verbs)
	}
}
