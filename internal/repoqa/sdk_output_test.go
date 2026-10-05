package repoqa

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
)

// This file pins sdk/go/output (the plugin copy of the machine contract, a
// separate module that cannot import internal/) to internal/output. Both sides
// read the same fixtures under sdk/go/output/testdata: the SDK's own tests
// assert it renders each case's expected bytes and that CheckEnvelope accepts
// the valid fixtures and rejects the invalid ones; the tests here assert that
// internal/output renders the same expected bytes, that the JSON Schema
// agrees with every fixture verdict, and that the SDK's exit constants equal
// internal/errs. A change to either renderer that is not mirrored fails one
// of the two.

const sdkOutputDir = "sdk/go/output"

type sdkCase struct {
	Input struct {
		Kind    string          `json:"kind"`
		GoType  string          `json:"go_type"`
		Command string          `json:"command"`
		Type    string          `json:"type"`
		Data    json.RawMessage `json:"data"`
		Fields  []struct {
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value"`
		} `json:"fields"`
		Meta  *output.Meta `json:"meta"`
		Error *struct {
			Code        string `json:"code"`
			Message     string `json:"message"`
			Cause       string `json:"cause"`
			Remediation string `json:"remediation"`
			DocsURL     string `json:"docs_url"`
			ExitCode    int    `json:"exit_code"`
			Class       string `json:"class"`
		} `json:"error"`
	} `json:"input"`
	Expected    []string `json:"expected"`
	ExpectError bool     `json:"expect_error"`
}

// sdkTypedSample mirrors typedSample in sdk/go/output/output_test.go: typed Go
// data (struct field order, nil versus empty slices and maps) goes through both
// renderers, not only map[string]any.
type sdkTypedSample struct {
	Zed        string         `json:"zed"`
	Alpha      int            `json:"alpha"`
	Note       string         `json:"note"`
	NilSlice   []string       `json:"nil_slice"`
	EmptySlice []string       `json:"empty_slice"`
	NilMap     map[string]int `json:"nil_map"`
	EmptyMap   map[string]int `json:"empty_map"`
	Skip       string         `json:"skip,omitempty"`
	Ptr        *int           `json:"ptr"`
	Inner      struct {
		B int `json:"b"`
		A int `json:"a"`
	} `json:"inner"`
}

func sdkJSONValue(t *testing.T, raw []byte) any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// sdkExitFor maps an exit class to the internal/errs status.
func sdkExitFor(class string) int {
	switch class {
	case "infra":
		return errs.ExitInfraError
	case "auth":
		return errs.ExitAuthError
	case "destructive_blocked":
		return errs.ExitDestructiveBlocked
	}
	return errs.ExitUserError
}

// sdkRenderInternal renders case c with internal/output, the way the CLI does.
func sdkRenderInternal(t *testing.T, c sdkCase) string {
	t.Helper()
	output.ResetState()
	t.Cleanup(output.ResetState)
	in := c.Input
	if in.Meta != nil {
		for _, d := range in.Meta.Deprecations {
			output.AddDeprecation(d.Old, d.New, d.RemovalAt)
		}
		for _, w := range in.Meta.Warnings {
			output.AddWarning(w)
		}
	}
	var out bytes.Buffer
	w := output.Writer{Out: &out, Err: &bytes.Buffer{}}
	compact := false
	switch in.Kind {
	case "data":
		var data any = sdkJSONValue(t, in.Data)
		if in.GoType == "typedSample" {
			var ts sdkTypedSample
			must(t, json.Unmarshal(in.Data, &ts))
			data = ts
		} else if in.GoType != "" {
			t.Fatalf("unknown go_type %q", in.GoType)
		}
		must(t, output.EmitData(w, in.Command, data))
	case "error", "stream_error":
		e := in.Error
		code := e.ExitCode // internal/output has no class input: the status decides
		if code == 0 {
			code = sdkExitFor(e.Class)
		}
		cli := &errs.CLIError{Code: e.Code, What: e.Message, Why: e.Cause, Fix: e.Remediation,
			DocsPath: strings.TrimPrefix(e.DocsURL, "https://nself.org/docs/")}
		must(t, output.EmitError(w, in.Command, &errs.ExitError{Code: code, Err: cli}))
		compact = in.Kind == "stream_error"
	case "stream_record", "stream_end":
		typ := in.Type
		if in.Kind == "stream_end" {
			typ = "result"
		}
		obj := fmt.Sprintf(`{"type":%q`, typ)
		for _, f := range in.Fields {
			k, _ := json.Marshal(f.Key)
			obj += "," + string(k) + ":" + string(f.Value)
		}
		must(t, output.EmitData(w, in.Command, json.RawMessage(obj+"}")))
		compact = true
	default:
		t.Fatalf("unknown case kind %q", in.Kind)
	}
	if !compact {
		return out.String()
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, out.Bytes()); err != nil {
		t.Fatal(err)
	}
	return buf.String() + "\n"
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// sdkLoadSchema resolves schemas/<file>, reading $ref targets through index.json.
func sdkLoadSchema(t *testing.T, root, file string) *jsonschema.Resolved {
	t.Helper()
	dir := filepath.Join(root, "schemas")
	read := func(f string) *jsonschema.Schema {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var s jsonschema.Schema
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		return &s
	}
	var idx struct{ Schemas []struct{ Path, ID string } }
	b, err := os.ReadFile(filepath.Join(dir, "index.json"))
	must(t, err)
	must(t, json.Unmarshal(b, &idx))
	loader := func(u *url.URL) (*jsonschema.Schema, error) {
		for _, e := range idx.Schemas {
			if e.ID == u.String() {
				return read(e.Path), nil
			}
		}
		return nil, fmt.Errorf("no schema with id %s", u)
	}
	rs, err := read(file).Resolve(&jsonschema.ResolveOptions{Loader: loader})
	must(t, err)
	return rs
}

func sdkSchemaVerdict(rs *jsonschema.Resolved, doc []byte) error {
	var v any
	if err := json.Unmarshal(doc, &v); err != nil {
		return err
	}
	return rs.Validate(v)
}

func sdkFixtures(t *testing.T, root, sub string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, sdkOutputDir, "testdata", sub, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures under %s/testdata/%s (err=%v): a verify that runs nothing fails", sdkOutputDir, sub, err)
	}
	return files
}

// TestSDKOutputCases: internal/output renders every shared case to the same
// bytes sdk/go/output is asserted to render, and those bytes (every line of a
// stream) validate against schemas/envelope.v1.schema.json. Valid fixtures are
// accepted and invalid ones rejected by the schema, the verdicts the SDK's
// CheckEnvelope is asserted to give by its own test.
func TestSDKOutputCases(t *testing.T) {
	root := repoRoot(t)
	rs := sdkLoadSchema(t, root, "envelope.v1.schema.json")

	cases := sdkFixtures(t, root, "cases")
	kinds := map[string]bool{}
	for _, f := range cases {
		b, err := os.ReadFile(f)
		must(t, err)
		var c sdkCase
		must(t, json.Unmarshal(b, &c))
		kinds[c.Input.Kind] = true
		got := sdkRenderInternal(t, c)
		if c.ExpectError {
			// internal/output writes this input anyway; the SDK refuses it
			// because the document it would write is not a valid envelope.
			if sdkSchemaVerdict(rs, []byte(got)) == nil {
				t.Errorf("%s: SDK refuses an input the schema accepts:\n%s", filepath.Base(f), got)
			}
			continue
		}
		want := strings.Join(c.Expected, "\n") + "\n"
		if got != want {
			t.Errorf("%s: internal/output drifted from the shared case\n--- internal ---\n%s\n--- expected ---\n%s", filepath.Base(f), got, want)
		}
		if err := sdkSchemaVerdict(rs, []byte(want)); err != nil {
			t.Errorf("%s: expected bytes do not validate: %v", filepath.Base(f), err)
		}
	}
	for _, k := range []string{"data", "error", "stream_record", "stream_end", "stream_error"} {
		if !kinds[k] {
			t.Errorf("no shared case of kind %q", k)
		}
	}

	for _, f := range sdkFixtures(t, root, "valid") {
		b, _ := os.ReadFile(f)
		if err := sdkSchemaVerdict(rs, b); err != nil {
			t.Errorf("valid fixture %s rejected by the schema: %v", filepath.Base(f), err)
		}
	}
	for _, f := range sdkFixtures(t, root, "invalid") {
		b, _ := os.ReadFile(f)
		if sdkSchemaVerdict(rs, b) == nil {
			t.Errorf("invalid fixture %s accepted by the schema", filepath.Base(f))
		}
	}
}

// sdkConsts reads the untyped string and int constants declared in a file.
func sdkConsts(t *testing.T, file string) map[string]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	must(t, err)
	out := map[string]string{}
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, s := range g.Specs {
			vs := s.(*ast.ValueSpec)
			for i, n := range vs.Names {
				if lit, ok := vs.Values[i].(*ast.BasicLit); ok {
					v := lit.Value
					if u, err := strconv.Unquote(v); err == nil {
						v = u
					}
					out[n.Name] = v
				}
			}
		}
	}
	return out
}

// TestSDKOutputExitContract: the SDK's exit statuses and class names equal
// internal/errs, so ExitCodeFor(class) and ClassFor(status) cannot drift.
func TestSDKOutputExitContract(t *testing.T) {
	c := sdkConsts(t, filepath.Join(repoRoot(t), sdkOutputDir, "exit.go"))
	for name, want := range map[string]int{
		"ExitUser": errs.ExitUserError, "ExitInfra": errs.ExitInfraError,
		"ExitAuth": errs.ExitAuthError, "ExitDestructiveBlocked": errs.ExitDestructiveBlocked,
	} {
		if c[name] != strconv.Itoa(want) {
			t.Errorf("sdk %s = %q, internal/errs = %d", name, c[name], want)
		}
	}
	for name, status := range map[string]int{
		"ClassUser": 1, "ClassInfra": 2, "ClassAuth": 3, "ClassDestructiveBlocked": 4, "ClassOther": 70,
	} {
		if c[name] != errs.ClassFor(status) {
			t.Errorf("sdk %s = %q, errs.ClassFor(%d) = %q", name, c[name], status, errs.ClassFor(status))
		}
	}
	env := sdkConsts(t, filepath.Join(repoRoot(t), sdkOutputDir, "envelope.go"))
	if env["SchemaVersion"] != output.SchemaVersion {
		t.Errorf("sdk SchemaVersion = %q, internal = %q", env["SchemaVersion"], output.SchemaVersion)
	}
}

// TestSDKOutputExitTable: testdata/exit-classes.json, the table the SDK test
// asserts ExitCodeFor and ClassFor against, equals internal/errs.
func TestSDKOutputExitTable(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), sdkOutputDir, "testdata", "exit-classes.json"))
	must(t, err)
	var tab struct {
		Classes []struct {
			Class string `json:"class"`
			Exit  int    `json:"exit"`
		} `json:"classes"`
		Other struct {
			Class       string `json:"class"`
			ExitExample int    `json:"exit_example"`
		} `json:"other"`
	}
	must(t, json.Unmarshal(b, &tab))
	if len(tab.Classes) != 4 {
		t.Fatalf("table has %d classes, want 4", len(tab.Classes))
	}
	for _, r := range tab.Classes {
		if r.Exit != sdkExitFor(r.Class) || errs.ClassFor(r.Exit) != r.Class {
			t.Errorf("table row %s=%d disagrees with internal/errs (%d, %q)", r.Class, r.Exit, sdkExitFor(r.Class), errs.ClassFor(r.Exit))
		}
	}
	if errs.ClassFor(tab.Other.ExitExample) != tab.Other.Class {
		t.Errorf("errs.ClassFor(%d) = %q, table %q", tab.Other.ExitExample, errs.ClassFor(tab.Other.ExitExample), tab.Other.Class)
	}
}

// TestSDKOutputStdlibOnly: sdk/go/output imports the standard library only.
func TestSDKOutputStdlibOnly(t *testing.T) {
	dir := filepath.Join(repoRoot(t), sdkOutputDir)
	files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	var bad []string
	n := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		n++
		af, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		must(t, err)
		for _, imp := range af.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.Contains(strings.SplitN(p, "/", 2)[0], ".") {
				bad = append(bad, filepath.Base(f)+" imports "+p)
			}
		}
	}
	if n == 0 {
		t.Fatal("no sdk output sources found")
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("sdk/go/output must be stdlib only:\n%s", strings.Join(bad, "\n"))
	}
}
