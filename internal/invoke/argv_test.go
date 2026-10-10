package invoke

// Purpose: request decoding, request id, params schema, exposure and argv tables.
// Constraints: every negative case names what it must refuse; none depends on
// the stub child.

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
)

func wantCode(t *testing.T, err error, code string, mentions ...string) {
	t.Helper()
	var ce *errs.CLIError
	if err == nil {
		t.Fatalf("want %s, got no error", code)
	}
	if !errorsAs(err, &ce) || ce.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
	for _, m := range mentions {
		if !strings.Contains(ce.What+ce.Why, m) {
			t.Errorf("error %q should mention %q", ce.What, m)
		}
	}
}

func errorsAs(err error, target **errs.CLIError) bool {
	ce, ok := err.(*errs.CLIError)
	*target = ce
	return ok
}

func req(t *testing.T, doc string) Request {
	t.Helper()
	r, err := DecodeRequest([]byte(doc))
	if err != nil {
		t.Fatalf("DecodeRequest(%s): %v", doc, err)
	}
	return r
}

func TestDecodeRequest(t *testing.T) {
	r := req(t, `{"args":["a"],"flags":{"n":1,"s":"x","b":true,"l":["p","q"]},"confirm":"`+strings.Repeat("ab", 32)+`"}`)
	if len(r.Args) != 1 || r.Flags["n"] != json.Number("1") || r.Confirm == "" {
		t.Fatalf("decoded %+v", r)
	}
	if got := req(t, ""); got.Args != nil || got.Flags != nil {
		t.Fatalf("empty input must be the empty request, got %+v", got)
	}
	bad := map[string]string{
		"unknown member":      `{"args":[],"extra":1}`,
		"wrong args type":     `{"args":"x"}`,
		"args holds number":   `{"args":[1]}`,
		"array":               `[]`,
		"null":                `null`,
		"trailing object":     `{} {}`,
		"trailing garbage":    `{}x`,
		"short confirm":       `{"confirm":"abc"}`,
		"uppercase confirm":   `{"confirm":"` + strings.Repeat("AB", 32) + `"}`,
		"truncated":           `{"args":`,
		"oversized":           `{"args":["` + strings.Repeat("a", MaxRequestBytes) + `"]}`,
		"argv holds object":   `{"argv":[{}]}`,
		"flags is an array":   `{"flags":[]}`,
		"confirm is a number": `{"confirm":1}`,
	}
	for name, doc := range bad {
		if _, err := DecodeRequest([]byte(doc)); err == nil {
			t.Errorf("%s: %q must be E420", name, doc)
		} else {
			wantCode(t, err, "E420")
		}
	}
}

type vectorFile []struct {
	Name        string          `json:"name"`
	Path        string          `json:"path"`
	SecretArgs  []string        `json:"secret_args"`
	SecretFlags []string        `json:"secret_flags"`
	Request     json.RawMessage `json:"request"`
	Want        string          `json:"want"`
}

func vectorCommand(secretArgs, secretFlags []string) *cmdregistry.Command {
	c := &cmdregistry.Command{Args: []cmdregistry.Arg{{Name: "key", Required: true}, {Name: "value", Required: true}}}
	for i := range c.Args {
		for _, s := range secretArgs {
			c.Args[i].Secret = c.Args[i].Secret || s == c.Args[i].Name
		}
	}
	for _, n := range []string{"tok", "name", "count"} {
		f := cmdregistry.Flag{Name: n}
		for _, s := range secretFlags {
			f.Secret = f.Secret || s == n
		}
		c.Flags = append(c.Flags, f)
	}
	return c
}

func TestRequestIDVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/request_id_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors vectorFile
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) < 2 {
		t.Fatalf("need at least two vectors, have %d", len(vectors))
	}
	for _, v := range vectors {
		got := RequestID(v.Path, vectorCommand(v.SecretArgs, v.SecretFlags), req(t, string(v.Request)))
		if got != v.Want {
			t.Errorf("%s: id %s, want %s", v.Name, got, v.Want)
		}
	}
	// Two requests that differ only in a secret value share an id, and the id
	// of a request never changes with the confirm member.
	cmd := vectorCommand([]string{"value"}, []string{"tok"})
	a := RequestID("config set", cmd, req(t, `{"args":["k","one"],"flags":{"tok":"x"}}`))
	b := RequestID("config set", cmd, req(t, `{"args":["k","two"],"flags":{"tok":"y"},"confirm":"`+strings.Repeat("0", 64)+`"}`))
	if a != b {
		t.Errorf("secret-only difference changed the id: %s vs %s", a, b)
	}
	if c := RequestID("config set", cmd, req(t, `{"args":["other","two"],"flags":{"tok":"y"}}`)); c == a {
		t.Error("a non-secret difference must change the id")
	}
	// An unknown command redacts everything: no id is derived from a value.
	if x, y := RequestID("nope", nil, req(t, `{"args":["one"]}`)), RequestID("nope", nil, req(t, `{"args":["two"]}`)); x != y {
		t.Error("with no command every value is treated as secret")
	}
}

func TestRedactRequestAndSecretValues(t *testing.T) {
	cmd := vectorCommand([]string{"value"}, []string{"tok"})
	r := req(t, `{"args":["k","hunter2"],"flags":{"tok":"abc","name":"n"}}`)
	red := RedactRequest(cmd, r)
	if red.Args[1] != Redacted || red.Args[0] != "k" || red.Flags["tok"] != Redacted || red.Flags["name"] != "n" {
		t.Fatalf("redacted %+v", red)
	}
	if r.Args[1] != "hunter2" {
		t.Fatal("RedactRequest must not modify its input")
	}
	got := SecretValues(cmd, r)
	if !reflect.DeepEqual(got, []string{"hunter2", "abc"}) && !reflect.DeepEqual(got, []string{"abc", "hunter2"}) {
		t.Fatalf("secret values %v", got)
	}
}

func TestParamsSchema(t *testing.T) {
	reg := fixtureRegistry(t)
	echo := ParamsSchema(mustCmd(t, reg, "fx echo"))
	b, _ := json.Marshal(echo)
	doc := decodeDoc(t, b)
	if doc["additionalProperties"] != false || doc["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("top level %v", doc)
	}
	props := doc["properties"].(map[string]any)
	if _, has := props["confirm"]; has {
		t.Error("a command without a confirm block must not list confirm")
	}
	args := props["args"].(map[string]any)
	if args["minItems"] != float64(1) || args["items"] == nil || len(args["prefixItems"].([]any)) != 2 {
		t.Errorf("variadic args schema %v", args)
	}
	flags := props["flags"].(map[string]any)
	if flags["additionalProperties"] != false {
		t.Error("flags must close additionalProperties")
	}
	fp := flags["properties"].(map[string]any)
	kinds := map[string]string{"name": "string", "count": "integer", "limit": "integer", "ratio": "number", "on": "boolean", "tags": "array", "items": "array", "wait": "string", "tok": "string"}
	for name, kind := range kinds {
		p, ok := fp[name].(map[string]any)
		if !ok || p["type"] != kind {
			t.Errorf("flag %s schema %v, want type %s", name, fp[name], kind)
		}
	}
	if fp["wait"].(map[string]any)["pattern"] == nil {
		t.Error("duration flag needs the duration pattern")
	}
	for _, absent := range []string{"json", "reveal", "secretish", "old", "follow", "nums", "no-monorepo", "help"} {
		if _, has := fp[absent]; has {
			t.Errorf("flag %s must not be in the params schema", absent)
		}
	}
	// A command with a confirm block lists confirm, never the consent or plan flags.
	apply := ParamsSchema(mustCmd(t, reg, "fx apply"))["properties"].(map[string]any)
	if apply["confirm"] == nil {
		t.Error("a command with a confirm block lists confirm")
	}
	for _, owned := range []string{"force", "plan", "plan-id"} {
		if _, has := apply["flags"].(map[string]any)["properties"].(map[string]any)[owned]; has {
			t.Errorf("invoker-owned flag %s must not be in the schema", owned)
		}
	}
	// A free-form plugin command takes argv and nothing else.
	demo := ParamsSchema(mustCmd(t, reg, "demo"))["properties"].(map[string]any)
	if demo["argv"] == nil || demo["args"] != nil || demo["flags"] != nil {
		t.Errorf("free-form plugin schema %v", demo)
	}
	typed := ParamsSchema(mustCmd(t, reg, "typed"))["properties"].(map[string]any)
	if typed["argv"] != nil || typed["args"] == nil {
		t.Errorf("a plugin command that declares args takes args, got %v", typed)
	}
}

func TestExposure(t *testing.T) {
	reg := fixtureRegistry(t)
	cases := []struct {
		path, transport string
		set             []string
		ok              bool
		reason          string
	}{
		{"fx echo", TransportMCP, nil, true, ""},
		{"fx echo", TransportHTTP, nil, true, ""},
		{"fx secretset", TransportMCP, nil, true, ""},
		{"demo", TransportMCP, nil, true, ""},
		{"fx clionly", TransportMCP, nil, false, "cli-only"},
		{"fx hid", TransportMCP, nil, false, "hidden"},
		{"pend", TransportMCP, nil, false, "pending"},
		{"fx oldecho", TransportMCP, nil, false, "shim"},
		{"help", TransportMCP, nil, false, "builtin"},
		{"fx legacy", TransportMCP, nil, false, "envelope"},
		{"fx interactive", TransportHTTP, nil, false, "interactive"},
		{"fx streamer", TransportMCP, nil, false, "NDJSON"},
		{"fx streamer", TransportHTTP, nil, false, "NDJSON"},
		{"fx streamer", TransportHTTPStream, nil, true, ""},
		{"fx streamwrite", TransportHTTPStream, nil, false, "write"},
		{"fx remote", TransportMCP, nil, false, "no gate"},
		{"fx destroy", TransportHTTP, nil, false, "no gate"},
		{"fx escalate", TransportMCP, nil, true, ""},
		{"fx escalate", TransportMCP, []string{"wipe"}, false, "no gate"},
		{"fx echo", TransportHTTPStream, nil, false, "not a stream"},
		{"fx echo", TransportMCP, []string{"follow"}, false, "NDJSON"},
		{"fx echo", TransportHTTP, []string{"follow"}, false, "NDJSON"},
		{"fx echo", TransportHTTPStream, []string{"follow"}, true, ""},
	}
	for _, c := range cases {
		set := map[string]bool{}
		for _, n := range c.set {
			set[n] = true
		}
		cmd := *mustCmd(t, reg, c.path)
		if c.path == "fx oldecho" {
			cmd.Hidden, cmd.Deprecated = false, nil // the shim rule, not the hidden rule
		}
		ok, reason := Exposure(&cmd, c.transport, set)
		if ok != c.ok || (!ok && !strings.Contains(reason, c.reason)) {
			t.Errorf("%s on %s set=%v: ok=%v reason=%q, want ok=%v reason~%q", c.path, c.transport, c.set, ok, reason, c.ok, c.reason)
		}
	}
	if ok, reason := Exposure(nil, TransportMCP, nil); ok || reason == "" {
		t.Error("a nil command is not exposed")
	}
}

func TestExposedFlagsStreamOverride(t *testing.T) {
	cmd := mustCmd(t, fixtureRegistry(t), "fx echo")
	names := func(transport string) string {
		var out []string
		for _, f := range ExposedFlags(cmd, transport) {
			out = append(out, f.Name)
		}
		return strings.Join(out, ",")
	}
	doc := names(TransportHTTP)
	if names(TransportMCP) != doc || strings.Contains(doc, "follow") {
		t.Errorf("document modes must hide the stream override flag: %s", doc)
	}
	if got := names(TransportHTTPStream); !strings.Contains(got, "follow") || !strings.HasPrefix(got, "count,follow,items") {
		t.Errorf("NDJSON must accept the override flag, sorted: %s", got)
	}
	if want := "count,items,limit,name,on,ratio,tags,tok,wait"; doc != want {
		t.Errorf("exposed flags %s, want %s", doc, want)
	}
	// Setting it in document mode is refused twice: by exposure and by argv.
	if ok, _ := Exposure(cmd, TransportMCP, map[string]bool{"follow": true}); ok {
		t.Error("exposure must refuse the stream override flag in document mode")
	}
	_, err := BuildArgv(cmd, req(t, `{"args":["w"],"flags":{"follow":true}}`))
	wantCode(t, err, "E420", "follow")
	argv, err := BuildArgvFor(cmd, req(t, `{"args":["w"],"flags":{"follow":true}}`), TransportHTTPStream)
	if err != nil || !contains(argv, "--follow") {
		t.Errorf("NDJSON argv %v, %v", argv, err)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestBuildArgv(t *testing.T) {
	reg := fixtureRegistry(t)
	echo := mustCmd(t, reg, "fx echo")
	ok := []struct {
		name, doc string
		want      []string
	}{
		{"no flags", `{"args":["w"]}`, []string{"fx", "echo", "--json", "--", "w"}},
		{"bool true", `{"args":["w"],"flags":{"on":true}}`, []string{"fx", "echo", "--json", "--on", "--", "w"}},
		{"bool false", `{"args":["w"],"flags":{"on":false}}`, []string{"fx", "echo", "--json", "--on=false", "--", "w"}},
		{"slice is one flag per item", `{"args":["w"],"flags":{"items":["a b","c;d"]}}`, []string{"fx", "echo", "--json", "--items=a b", "--items=c;d", "--", "w"}},
		{"stringSlice", `{"args":["w"],"flags":{"tags":["a","b"]}}`, []string{"fx", "echo", "--json", "--tags=a", "--tags=b", "--", "w"}},
		{"flags sorted by name", `{"args":["w"],"flags":{"name":"n","count":3,"limit":0,"ratio":1.5,"wait":"90s"}}`,
			[]string{"fx", "echo", "--json", "--count=3", "--limit=0", "--name=n", "--ratio=1.5", "--wait=90s", "--", "w"}},
		{"whole float is an integer", `{"args":["w"],"flags":{"count":2.0}}`, []string{"fx", "echo", "--json", "--count=2", "--", "w"}},
		{"exponent form of a whole number", `{"args":["w"],"flags":{"count":1e3}}`, []string{"fx", "echo", "--json", "--count=1000", "--", "w"}},
		{"negative integer", `{"args":["w"],"flags":{"count":-4}}`, []string{"fx", "echo", "--json", "--count=-4", "--", "w"}},
		{"spaces newline semicolon substitution stay one element", `{"args":["a b","l1\nl2","x;rm -rf /","$(id)","` + "`id`" + `"],"flags":{"name":"v $(id); echo\n"}}`,
			[]string{"fx", "echo", "--json", "--name=v $(id); echo\n", "--", "a b", "l1\nl2", "x;rm -rf /", "$(id)", "`id`"}},
		{"arg starting with dash lands after --", `{"args":["--json","-x","--"]}`, []string{"fx", "echo", "--json", "--", "--json", "-x", "--"}},
		{"flag value starting with dash stays glued", `{"args":["w"],"flags":{"name":"--tok=evil"}}`, []string{"fx", "echo", "--json", "--name=--tok=evil", "--", "w"}},
		{"variadic", `{"args":["w","m1","m2"]}`, []string{"fx", "echo", "--json", "--", "w", "m1", "m2"}},
		{"empty string value", `{"args":["w"],"flags":{"name":""}}`, []string{"fx", "echo", "--json", "--name=", "--", "w"}},
	}
	for _, c := range ok {
		got, err := BuildArgv(echo, req(t, c.doc))
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v, %v\nwant %q", c.name, got, err, c.want)
		}
	}
	bad := []struct{ name, doc, mention string }{
		{"fraction for an integer", `{"args":["w"],"flags":{"count":1.5}}`, "count"},
		{"string for an integer", `{"args":["w"],"flags":{"count":"1"}}`, "count"},
		{"negative for unsigned", `{"args":["w"],"flags":{"limit":-1}}`, "limit"},
		{"string for a bool", `{"args":["w"],"flags":{"on":"true"}}`, "on"},
		{"number for a string", `{"args":["w"],"flags":{"name":1}}`, "name"},
		{"null value", `{"args":["w"],"flags":{"name":null}}`, "name"},
		{"object value", `{"args":["w"],"flags":{"name":{}}}`, "name"},
		{"string for a slice", `{"args":["w"],"flags":{"items":"a"}}`, "items"},
		{"number in a slice", `{"args":["w"],"flags":{"items":["a",1]}}`, "items"},
		{"bad duration", `{"args":["w"],"flags":{"wait":"soon"}}`, "wait"},
		{"comma in a stringSlice item", `{"args":["w"],"flags":{"tags":["a,b"]}}`, "tags"},
		{"quote in a stringSlice item", `{"args":["w"],"flags":{"tags":["a\"b"]}}`, "tags"},
		{"newline in a stringSlice item", `{"args":["w"],"flags":{"tags":["a\nb"]}}`, "tags"},
		{"unknown flag", `{"args":["w"],"flags":{"nope":1}}`, "nope"},
		{"json flag", `{"args":["w"],"flags":{"json":false}}`, "json"},
		{"help flag", `{"args":["w"],"flags":{"help":true}}`, "help"},
		{"no-monorepo flag", `{"args":["w"],"flags":{"no-monorepo":true}}`, "no-monorepo"},
		{"hidden flag", `{"args":["w"],"flags":{"secretish":"x"}}`, "secretish"},
		{"deprecated flag", `{"args":["w"],"flags":{"old":"x"}}`, "old"},
		{"cli_only flag", `{"args":["w"],"flags":{"reveal":"x"}}`, "reveal"},
		{"unsupported flag type", `{"args":["w"],"flags":{"nums":[1]}}`, "nums"},
		{"flag spelled with dashes", `{"args":["w"],"flags":{"--name":"x"}}`, "--name"},
		{"flag name with equals", `{"args":["w"],"flags":{"name=x":"y"}}`, "name=x"},
		{"NUL in an arg", `{"args":["a\u0000b"]}`, "NUL"},
		{"NUL in a flag value", `{"args":["w"],"flags":{"name":"a\u0000b"}}`, "NUL"},
		{"NUL in a flag name", `{"args":["w"],"flags":{"na\u0000me":"x"}}`, "NUL"},
		{"too few args", `{}`, "too few"},
		{"too many args on a fixed command", `{"args":["a","b"]}`, ""},
		{"argv on a declared command", `{"args":["w"],"argv":["--x"]}`, "argv"},
		{"confirm on a command without a confirm block", `{"args":["w"],"confirm":"` + strings.Repeat("a", 64) + `"}`, "confirm"},
		{"oversized value", `{"args":["` + strings.Repeat("a", MaxElementBytes+1) + `"]}`, "bytes"},
	}
	for _, c := range bad {
		cmd := echo
		if c.name == "too many args on a fixed command" {
			cmd = mustCmd(t, reg, "fx secretset")
			c.doc = `{"args":["a","b","c"]}`
		}
		_, err := BuildArgv(cmd, req(t, c.doc))
		wantCode(t, err, "E420")
		if c.mention != "" {
			wantCode(t, err, "E420", c.mention)
		}
	}
	// A request value never reaches the error text.
	_, err := BuildArgv(echo, req(t, `{"args":["w"],"flags":{"count":"sekrit-value"}}`))
	if err == nil || strings.Contains(err.Error(), "sekrit-value") {
		t.Errorf("error must name the flag, not echo its value: %v", err)
	}
	// Free-form plugin command: items verbatim after --json, no --.
	demo := mustCmd(t, reg, "demo")
	got, err := BuildArgv(demo, req(t, `{"argv":["sub","--x=1","a b"]}`))
	if want := []string{"demo", "--json", "sub", "--x=1", "a b"}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("free-form argv %v, %v", got, err)
	}
	if _, err := BuildArgv(demo, req(t, `{"args":["x"]}`)); err == nil {
		t.Error("a free-form command takes argv, not args")
	}
	if _, err := BuildArgv(demo, req(t, `{"argv":["a\u0000b"]}`)); err == nil {
		t.Error("NUL in argv must be refused")
	}
	// A plugin command that declares args and flags uses them.
	typed := mustCmd(t, reg, "typed")
	got, err = BuildArgv(typed, req(t, `{"args":["t","e1"],"flags":{"mode":"fast"}}`))
	if want := []string{"typed", "--json", "--mode=fast", "--", "t", "e1"}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("typed plugin argv %v, %v", got, err)
	}
	// Values as an MCP client library hands them over: float64, int and []any.
	native := []struct {
		name  string
		flags map[string]any
		want  []string
	}{
		{"whole float64", map[string]any{"count": float64(2)}, []string{"fx", "echo", "--json", "--count=2", "--", "w"}},
		{"int and []any", map[string]any{"limit": 7, "items": []any{"p", "q"}}, []string{"fx", "echo", "--json", "--items=p", "--items=q", "--limit=7", "--", "w"}},
		{"fractional float64", map[string]any{"count": 1.5}, nil},
		{"NaN", map[string]any{"ratio": math.NaN()}, nil},
		{"huge float64 integer", map[string]any{"count": 1e300}, nil},
		{"[]any with a number", map[string]any{"items": []any{1}}, nil},
	}
	for _, c := range native {
		got, err := BuildArgv(echo, Request{Args: []string{"w"}, Flags: c.flags})
		if c.want == nil {
			wantCode(t, err, "E420")
		} else if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q, %v", c.name, got, err)
		}
	}
	// Whole-argv bound.
	many := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		many = append(many, strings.Repeat("b", MaxElementBytes))
	}
	b, _ := json.Marshal(Request{Args: append([]string{"w"}, many...)})
	if _, err := BuildArgv(echo, req(t, string(b))); err == nil {
		t.Error("an argv over MaxArgvBytes must be refused")
	}
	if _, err := BuildArgv(nil, Request{}); err == nil {
		t.Error("nil command must be refused")
	}
}
