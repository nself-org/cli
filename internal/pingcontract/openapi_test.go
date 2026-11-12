package pingcontract

// Contract checks against the pinned ping_api OpenAPI (testdata). No network.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/plugin"
	"gopkg.in/yaml.v3"
)

type spec struct {
	Paths      map[string]map[string]yaml.Node `yaml:"paths"`
	Components struct {
		Schemas map[string]map[string]any `yaml:"schemas"`
	} `yaml:"components"`
	raw map[string]any
}

func loadSpec(t testing.TB) *spec {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "ping_api.openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := &spec{}
	if err := yaml.Unmarshal(b, s); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(b, &s.raw); err != nil {
		t.Fatal(err)
	}
	return s
}

// knownGaps are CLI-called endpoints missing from the pinned OpenAPI. Each
// is routed to P7-PLUG-27 (generated spec). Remove the line when the spec
// gains the path; the test fails on a stale entry.
var knownGaps = map[string]string{
	"GET /license/events": "SSE stream used by `nself license tail` is not in api-routes.yaml (P7-PLUG-27)",
}

func TestPinnedSpecChecksum(t *testing.T) {
	sums, err := os.ReadFile(filepath.Join("testdata", "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(sums)), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			t.Fatalf("bad SHA256SUMS line %q", line)
		}
		b, err := os.ReadFile(filepath.Join("testdata", f[1]))
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(b)
		if hex.EncodeToString(h[:]) != f[0] {
			t.Errorf("%s: checksum differs from SHA256SUMS; refresh both together (see README.md)", f[1])
		}
	}
}

// pathMatches compares segments; a {param} segment matches anything.
func pathMatches(tmpl, p string, prefix bool) bool {
	ts, ps := strings.Split(strings.Trim(tmpl, "/"), "/"), strings.Split(strings.Trim(p, "/"), "/")
	if len(ps) > len(ts) || (!prefix && len(ps) != len(ts)) {
		return false
	}
	for i := range ps {
		if !strings.HasPrefix(ts[i], "{") && ts[i] != ps[i] {
			return false
		}
	}
	return true
}

func (s *spec) has(e endpoint) bool {
	for tmpl, ops := range s.Paths {
		if _, ok := ops[strings.ToLower(e.Method)]; ok && pathMatches(tmpl, e.Path, e.Prefix) {
			return true
		}
	}
	return false
}

// missing returns the endpoints absent from the spec and not on the gap list.
func (s *spec) missing(eps []endpoint) (unknown []string, stale []string) {
	hit := map[string]bool{}
	for _, e := range eps {
		key := e.Method + " " + e.Path
		if s.has(e) {
			continue
		}
		hit[key] = true
		if _, ok := knownGaps[key]; !ok {
			unknown = append(unknown, key+" ("+e.Where+")")
		}
	}
	for k := range knownGaps {
		if !hit[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(unknown)
	return unknown, stale
}

func TestEveryScannedEndpointIsInTheSpec(t *testing.T) {
	unknown, stale := loadSpec(t).missing(scanRepo(t, repoRoot(t)))
	for _, u := range unknown {
		t.Errorf("CLI calls %s, absent from the pinned OpenAPI: add it to ping_api (P7-PLUG-27) or list it in knownGaps with a reason", u)
	}
	for _, k := range stale {
		t.Errorf("knownGaps lists %s but it is now found or no longer called: remove the entry", k)
	}
}

// TestNewCallSiteWithAbsentPathFails is the fixture test: a call site added
// to the CLI that targets a path the spec lacks must be reported.
func TestNewCallSiteWithAbsentPathFails(t *testing.T) {
	src := `package x
import "net/http"
func f(pingURL string) { http.NewRequest(http.MethodGet, pingURL+"/license/does-not-exist", nil) }
`
	eps, err := scanSource("new.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	unknown, _ := loadSpec(t).missing(eps)
	if len(unknown) != 1 || !strings.HasPrefix(unknown[0], "GET /license/does-not-exist") {
		t.Fatalf("absent path was not reported: %v", unknown)
	}
	// A path the spec has must pass.
	ok, _ := scanSource("ok.go", []byte(strings.ReplaceAll(src, "/license/does-not-exist", "/license/revocation-list")))
	if u, _ := loadSpec(t).missing(ok); len(u) != 0 {
		t.Fatalf("present path was reported: %v", u)
	}
}

// ---- response fixtures built from the schemas ----

func (s *spec) schema(name string) map[string]any { return s.Components.Schemas[name] }

func refName(m map[string]any) string {
	r, _ := m["$ref"].(string)
	return strings.TrimPrefix(r, "#/components/schemas/")
}

// fixture builds a value from a schema: every property, obviously fake data.
func (s *spec) fixture(sc map[string]any) any {
	if n := refName(sc); n != "" {
		return s.fixture(s.schema(n))
	}
	if one, ok := sc["oneOf"].([]any); ok && len(one) > 0 {
		return s.fixture(one[0].(map[string]any))
	}
	if ex, ok := sc["example"]; ok {
		return ex
	}
	if en, ok := sc["enum"].([]any); ok && len(en) > 0 {
		return en[0]
	}
	typ := sc["type"]
	if list, ok := typ.([]any); ok {
		for _, x := range list {
			if x != "null" {
				typ = x
				break
			}
		}
	}
	switch typ {
	case "object":
		out := map[string]any{}
		props, _ := sc["properties"].(map[string]any)
		for k, v := range props {
			out[k] = s.fixture(v.(map[string]any))
		}
		return out
	case "array":
		item, _ := sc["items"].(map[string]any)
		return []any{s.fixture(item)}
	case "integer", "number":
		return 1
	case "boolean":
		return true
	}
	return "fake-value"
}

// props lists the property names of an object schema, following $ref/oneOf.
func (s *spec) props(sc map[string]any) []string {
	if n := refName(sc); n != "" {
		return s.props(s.schema(n))
	}
	if one, ok := sc["oneOf"].([]any); ok && len(one) > 0 {
		return s.props(one[0].(map[string]any))
	}
	var out []string
	p, _ := sc["properties"].(map[string]any)
	for k := range p {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// target is a CLI type that decodes one endpoint's 200 response.
type target struct {
	Name       string
	Method     string
	Path       string
	Schema     string // component schema name (first oneOf branch for validate)
	Value      func() any
	SrcFile    string // set for unexported types: tags are read from source
	SrcType    string
	KnownDrift []string // CLI json fields the schema does not declare
}

var targets = []target{
	{Name: "license.ValidateResponse", Method: "POST", Path: "/license/validate", Schema: "ValidateResponse",
		Value: func() any { return &license.ValidateResponse{} },
		// P7-PLUG-27/web gap: the CLI reads fields the ValidateResponse schema does
		// not declare (the server sends "error" and "entitlements"; the CLI reads
		// "reason" and "plugins"; signature/key_id/plugins_allowed are legacy or
		// bundle-form fields).
		KnownDrift: []string{"key_id", "plugins", "plugins_allowed", "reason", "signature"}},
	{Name: "plugin.LicenseValidateResponse", Method: "POST", Path: "/license/validate", Schema: "ValidateResponse",
		Value: func() any { return &plugin.LicenseValidateResponse{} },
		// P7-PLUG-27/web gap: same drift as above; "expires" is not "expires_at".
		KnownDrift: []string{"expires", "plugins", "reason"}},
	{Name: "license.RevocationList", Method: "GET", Path: "/license/revocation-list", Schema: "RevocationListResponse",
		Value: func() any { return &license.RevocationList{} },
		// The spec now matches the route (web 7b96054c): no drift.
	},
	{Name: "update pingVersionResponse", Method: "GET", Path: "/version", Schema: "VersionResponse",
		SrcFile: "cmd/commands/update_release_api.go", SrcType: "pingVersionResponse"},
}

func jsonTags(rt reflect.Type) []string {
	var out []string
	for i := 0; i < rt.NumField(); i++ {
		tag := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
		if tag != "" && tag != "-" {
			out = append(out, tag)
		}
	}
	return out
}

func astTags(t testing.TB, file, typeName string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(repoRoot(t), file), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != typeName {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return false
		}
		for _, fld := range st.Fields.List {
			if fld.Tag == nil {
				continue
			}
			tag := reflect.StructTag(strings.Trim(fld.Tag.Value, "`")).Get("json")
			if name := strings.Split(tag, ",")[0]; name != "" && name != "-" {
				out = append(out, name)
			}
		}
		return false
	})
	if len(out) == 0 {
		t.Fatalf("no json fields found for %s in %s", typeName, file)
	}
	return out
}

func (tg target) tags(t testing.TB) []string {
	if tg.SrcFile != "" {
		return astTags(t, tg.SrcFile, tg.SrcType)
	}
	return jsonTags(reflect.TypeOf(tg.Value()).Elem())
}

func diff(a, b []string) []string {
	have := map[string]bool{}
	for _, x := range b {
		have[x] = true
	}
	var out []string
	for _, x := range a {
		if !have[x] {
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

// TestResponseFixturesDecodeIntoCLITypes decodes a schema-built fixture into
// each CLI type. When the CLI type is complete (it declares every schema
// property) unknown fields are rejected; otherwise the CLI is checked the
// other way: every field it reads must exist in the schema, except fields on
// the target's KnownDrift list.
func TestResponseFixturesDecodeIntoCLITypes(t *testing.T) {
	s := loadSpec(t)
	for _, tg := range targets {
		t.Run(tg.Name, func(t *testing.T) {
			sc := map[string]any{"$ref": "#/components/schemas/" + tg.Schema}
			body, err := json.Marshal(s.fixture(sc))
			if err != nil {
				t.Fatal(err)
			}
			schemaProps, cliTags := s.props(sc), tg.tags(t)
			if tg.Value != nil {
				dec := json.NewDecoder(bytes.NewReader(body))
				if len(diff(schemaProps, cliTags)) == 0 {
					dec.DisallowUnknownFields()
				}
				if err := dec.Decode(tg.Value()); err != nil {
					t.Fatalf("fixture %s does not decode: %v", body, err)
				}
			}
			drift := diff(cliTags, schemaProps)
			if got, want := strings.Join(drift, ","), strings.Join(sortedCopy(tg.KnownDrift), ","); got != want {
				t.Errorf("CLI reads fields the schema %s does not declare: got [%s], known [%s]. Add the field to ping_api (P7-PLUG-27) or update KnownDrift with a reason", tg.Schema, got, want)
			}
		})
	}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
