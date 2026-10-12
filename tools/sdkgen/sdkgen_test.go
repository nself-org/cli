package main

// Purpose: verify schema mapping, naming, rendering and the -check contract of sdkgen.
// Inputs: a small OpenAPI fixture in testdata. Outputs: pass/fail assertions.
// Constraints: no network; all writes go to t.TempDir().

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureDir(t *testing.T) (spec, out string) {
	t.Helper()
	dir := t.TempDir()
	out = filepath.Join(dir, "pkg")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	readme := "# pkg\n" + readmeBegin + "\nold\n" + readmeEnd + "\ntail\n"
	if err := os.WriteFile(filepath.Join(out, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join("testdata", "mini.openapi.json"), out
}

func TestSdkgenTypeScript(t *testing.T) {
	spec, out := fixtureDir(t)
	if code, err := run("ts", spec, out, true); code != 1 || err == nil {
		t.Fatalf("check on an empty package: run = (%d, %v), want (1, stale)", code, err)
	}
	if code, err := run("ts", spec, out, false); code != 0 || err != nil {
		t.Fatalf("write: run = (%d, %v)", code, err)
	}
	if code, err := run("ts", spec, out, true); code != 0 || err != nil {
		t.Fatalf("check after write: run = (%d, %v), want clean", code, err)
	}
	types := readFile(t, filepath.Join(out, "src/generated/types.ts"))
	client := readFile(t, filepath.Join(out, "src/generated/client.ts"))
	for _, want := range []string{
		header,
		"export type WidgetGetArgs = [string, string?];",
		`"dry-run"?: boolean;`,
		`mode?: "a" | "b";`,
		"export type WidgetListArgs = Array<string>;",
		"export interface WidgetGetData {",
		"id: number;",
		"tags: Array<string>;",
		"pair?: [string?, number?];",
		"owner?: string | null;",
		"extra?: Record<string, unknown>;",
		"child?: WidgetListData;",
		"ok?: unknown;",
		"export type WidgetListData = Array<{\n  name?: string;\n}>;",
		"export interface WidgetGetRequest {\n  args: WidgetGetArgs;",
		"export interface WidgetListRequest {\n  args?: WidgetListArgs;\n  flags?: WidgetListFlags;\n  confirm?: string;",
		"export interface WidgetGetRequest {\n  args: WidgetGetArgs;\n  flags?: WidgetGetFlags;\n}",
		"export interface ErrorDetail {",
		"Get a widget | with pipe *​/",
	} {
		if !strings.Contains(types, want) {
			t.Errorf("types.ts lacks %q\n%s", want, types)
		}
	}
	for _, want := range []string{
		header,
		"widgetGet(request: T.WidgetGetRequest, options?: CallOptions): Promise<T.WidgetGetData>;",
		"widgetList(request?: T.WidgetListRequest, options?: CallOptions): Promise<T.WidgetListData>;",
		`widgetGet: (request, options) => transport.call<T.WidgetGetData>("/v1/commands/widget/get", request, options),`,
	} {
		if !strings.Contains(client, want) {
			t.Errorf("client.ts lacks %q\n%s", want, client)
		}
	}
	if strings.Contains(client, `"/openapi.json"`) {
		t.Error("client.ts must not generate the GET /openapi.json operation")
	}
	readme := readFile(t, filepath.Join(out, "README.md"))
	if !strings.Contains(readme, "| `widgetGet` | `POST /v1/commands/widget/get` | Get a widget \\| with pipe") || !strings.HasSuffix(readme, readmeEnd+"\ntail\n") {
		t.Errorf("README region not rendered:\n%s", readme)
	}
	// Drift: a hand edit to a generated file is reported.
	if err := os.WriteFile(filepath.Join(out, "src/generated/client.ts"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, err := run("ts", spec, out, true); code != 1 || err == nil || !strings.Contains(err.Error(), "client.ts") {
		t.Fatalf("drift not reported: (%d, %v)", code, err)
	}
}

func TestSdkgenRejects(t *testing.T) {
	spec, out := fixtureDir(t)
	if code, _ := run("py", spec, out, false); code != 2 {
		t.Errorf("-lang py: code %d, want 2", code)
	}
	if err := os.Remove(filepath.Join(out, "README.md")); err != nil {
		t.Fatal(err)
	}
	if code, _ := run("ts", spec, out, false); code != 2 {
		t.Errorf("missing README: code %d, want 2", code)
	}
	if _, err := ReplaceReadmeRegion([]byte("no markers"), nil); err == nil {
		t.Error("README without markers must fail")
	}
}

func TestNaming(t *testing.T) {
	for in, want := range map[string]string{
		"nself_config_get":       "configGet",
		"nself_db_migrate_down":  "dbMigrateDown",
		"nself_status":           "status",
		"nself_backup_init_key":  "backupInitKey",
		"nself_license_clear_x1": "licenseClearX1",
	} {
		if got := methodName(in); got != want {
			t.Errorf("methodName(%q) = %q, want %q", in, got, want)
		}
	}
	if typeName("error") != "ErrorDetail" || typeName("nself_config_get_data") != "ConfigGetData" {
		t.Error("typeName mapping changed")
	}
	if propKey("dry-run") != `"dry-run"` || propKey("name") != "name" {
		t.Error("propKey quoting changed")
	}
}

func TestCollisionsFail(t *testing.T) {
	m := &Model{Schemas: map[string]*Schema{}, Ops: []Operation{
		{ID: "nself_a_b", Path: "/v1/commands/a/b"},
		{ID: "nself_aB", Path: "/v1/commands/aB"},
	}}
	if _, _, err := Render(m); err == nil || !strings.Contains(err.Error(), "both map") {
		t.Fatalf("collision not reported: %v", err)
	}
	m = &Model{Schemas: map[string]*Schema{"nself_a_b_data": {}, "nself_a-b_data": {}}}
	if _, _, err := Render(m); err == nil || !strings.Contains(err.Error(), "both map to type") {
		t.Fatalf("component collision not reported: %v", err)
	}
	m = &Model{Schemas: map[string]*Schema{}, Ops: []Operation{{ID: "nself_stream", Path: "/v1/commands/stream"}}}
	if _, _, err := Render(m); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("reserved member not reported: %v", err)
	}
}

func TestTupleEdges(t *testing.T) {
	min1 := 1
	s := &Schema{
		Type:        []byte(`"array"`),
		Items:       []byte("false"),
		MinItems:    &min1,
		PrefixItems: []*Schema{{Type: []byte(`"string"`)}, {Type: []byte(`["string","null"]`)}, {Enum: []json.RawMessage{json.RawMessage(`"a"`), json.RawMessage(`"b"`)}}},
	}
	if got, want := tsType(s, 0), `[string, (string | null)?, ("a" | "b")?]`; got != want {
		t.Errorf("tuple = %s, want %s", got, want)
	}
	rest := &Schema{Type: []byte(`"array"`), Items: []byte(`{"type":"string"}`), PrefixItems: []*Schema{{Type: []byte(`"string"`)}}, MinItems: &min1}
	if got, want := tsType(&Schema{Type: []byte(`"array"`)}, 0), "Array<unknown>"; got != want {
		t.Errorf("bare array = %s, want %s", got, want)
	}
	if got, want := tsType(&Schema{Type: []byte(`"array"`), Items: []byte("false"), MaxItems: new(int)}, 0), "[]"; got != want {
		t.Errorf("empty tuple = %s, want %s", got, want)
	}
	if got, want := tsType(rest, 0), `[string, ...Array<string>]`; got != want {
		t.Errorf("rest tuple = %s, want %s", got, want)
	}
}

func TestRequiredArgsHonoured(t *testing.T) {
	req := &Schema{Required: []string{"args"}, Properties: map[string]*Schema{"args": {Type: []byte(`"array"`), Items: []byte(`{"type":"string"}`)}}}
	decls, optional, err := requestDecls("X", Operation{ID: "nself_x", Request: req})
	if err != nil || optional {
		t.Fatalf("args listed in required must make the request mandatory: optional=%v err=%v", optional, err)
	}
	if !strings.Contains(decls[2].Body, "args: XArgs;") {
		t.Errorf("request body = %q", decls[2].Body)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
