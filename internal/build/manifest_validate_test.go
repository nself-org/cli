package build

// Purpose: tests for ValidateManifestFile/Bytes: finding codes, lines, paths,
// the v1.4 warn / v1.5 fail switch, x- extensions, the four reference-app
// manifests, and agreement with the generated JSON Schema.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

const fixtureDir = "testdata/manifest"

func readManifestFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtureDir, name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return b
}

func validateFixture(t *testing.T, name string) []Finding {
	t.Helper()
	return ValidateManifestFile(filepath.Join(fixtureDir, name))
}

// summary renders findings as "line code path" for exact comparison.
func summary(fs []Finding) []string {
	out := []string{}
	for _, f := range fs {
		out = append(out, fmt.Sprintf("%d %s %s", f.Line, f.Code, f.Path))
	}
	return out
}

func wantSeverity() string {
	if compat.V15() {
		return SeverityError
	}
	return SeverityWarning
}

func TestManifestValidate(t *testing.T) {
	cases := []struct {
		name string
		file string
		want []string
	}{
		{"clean x- keys at every depth", "x-extensions.yaml", []string{}},
		{"flat plugin list", "flat-list.yaml", []string{}},
		{"null values", "null-values.yaml", []string{}},
		{"null list item", "null-item.yaml", []string{"4 E435 plugins.free[1]"}},
		{"typo key", "typo-key.yaml", []string{"2 E436 plugin"}},
		{"nested unknown key", "nested-unknown.yaml", []string{"4 E436 plugins.required"}},
		{"plugins scalar", "type-error.yaml", []string{"2 E435 plugins"}},
		{"int app and string bundles", "int-app.yaml", []string{"1 E435 app", "2 E435 bundles"}},
		{"root is a list", "root-list.yaml", []string{"1 E435 "}},
		{"root null", "null-root.yaml", []string{}},
		{"merge map", "merge-map.yaml", []string{}},
		{"merge list of anchors", "merge-seq.yaml", []string{}},
		{"merge: unknown and wrong-typed merged keys, explicit key wins", "merge-bad.yaml", []string{"3 E436 plugins.paid"}},
		{"merge at the top level", "merge-top.yaml", []string{"3 E436 project"}},
		{"duplicate top-level key", "dup-key.yaml", []string{"3 E435 app"}},
		{"duplicate nested key", "dup-nested.yaml", []string{"3 E435 plugins.free"}},
		{"coerced scalars", "scalar-coerced.yaml", []string{"1 E435 app", "2 E435 bundle", "3 E435 bundles[0]", "3 E435 bundles[1]"}},
	}
	compattest.Both(t, func(t *testing.T) {
		for _, c := range cases {
			fs := validateFixture(t, c.file)
			got := summary(fs)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("%s: got %v want %v", c.name, got, c.want)
			}
			for _, f := range fs {
				if f.Severity != wantSeverity() || f.File != c.file || f.Fix == "" || f.Message == "" {
					t.Errorf("%s: bad finding %+v (want severity %s)", c.name, f, wantSeverity())
				}
			}
		}
	})
}

// A second YAML document is ignored by build: always a warning, in both modes.
func TestManifestValidateMultiDocument(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		fs := validateFixture(t, "multidoc.yaml")
		if len(fs) != 1 || fs[0].Severity != SeverityWarning || fs[0].Line != 3 || !strings.Contains(fs[0].Message, "ignored") {
			t.Errorf("multi-document findings: %+v", fs)
		}
	})
}

// The wording of a coerced scalar must tell the user to quote it.
func TestManifestValidateQuoteHint(t *testing.T) {
	f := validateFixture(t, "scalar-coerced.yaml")[0]
	if !strings.Contains(f.Fix, `quote it: "true"`) || !strings.Contains(f.Message, "bool") {
		t.Errorf("finding = %+v", f)
	}
}

func TestManifestValidateNeverSwallowsErrors(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		missing := ValidateManifestFile(filepath.Join(t.TempDir(), "nself.yaml"))
		if len(missing) != 1 || missing[0].Code != CodeManifestType || !strings.Contains(missing[0].Message, "cannot read") {
			t.Errorf("unreadable file must be a finding, got %+v", missing)
		}
		bad := ValidateManifestBytes("nself.yaml", []byte("app: [unclosed\n"))
		if len(bad) != 1 || bad[0].Code != CodeManifestType || !strings.Contains(bad[0].Message, "invalid YAML") {
			t.Errorf("syntax error must be a finding, got %+v", bad)
		}
		if got := ValidateManifestBytes("nself.yaml", nil); got != nil {
			t.Errorf("empty file is a zero manifest, got %+v", got)
		}
	})
}

func TestManifestValidateFixes(t *testing.T) {
	fs := validateFixture(t, "nclaw.yaml")
	var project, typo Finding
	for _, f := range fs {
		if f.Path == "project" {
			project = f
		}
	}
	if !strings.Contains(project.Fix, `did you mean "app"`) {
		t.Errorf("project fix = %q", project.Fix)
	}
	typo = validateFixture(t, "typo-key.yaml")[0]
	if !strings.Contains(typo.Fix, `did you mean "plugins"`) || !strings.Contains(typo.Fix, "x-plugin") {
		t.Errorf("typo fix = %q", typo.Fix)
	}
}

// TestManifestRefAppFixtures pins the verdict for the four reference-app files
// (copied at the commits named in the Ticket).
func TestManifestRefAppFixtures(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		keys := func(file string) []string {
			var out []string
			for _, f := range validateFixture(t, file) {
				out = append(out, f.Code+" "+f.Path)
			}
			return out
		}
		eq := func(file string, want []string) {
			t.Helper()
			if got := keys(file); !reflect.DeepEqual(got, want) {
				t.Errorf("%s: got %v want %v", file, got, want)
			}
		}
		eq("ntask.yaml", []string{"E436 display_name", "E436 version", "E436 tier", "E436 auth_mode", "E436 services"})
		eq("nsentry.yaml", []string{"E436 display_name", "E436 version", "E436 tier", "E436 auth_mode", "E436 services"})
		eq("nclaw.yaml", []string{"E436 project", "E436 services", "E435 plugins.pro[0]", "E435 plugins.pro[1]", "E435 plugins.pro[2]", "E435 plugins.pro[3]", "E435 plugins.pro[4]", "E435 plugins.pro[5]", "E435 plugins.pro[6]", "E435 plugins.pro[7]", "E435 plugins.pro[8]"})
		eq("nchat.yaml", []string{"E436 project", "E436 title", "E436 description", "E436 version", "E436 services", "E436 plugins.required", "E436 custom_services", "E436 env_example"})
		for _, f := range validateFixture(t, "nclaw.yaml") {
			if f.Code == CodeManifestType && f.Path == "plugins.pro[0]" && f.Line != 31 {
				t.Errorf("plugins.pro[0] line = %d", f.Line)
			}
		}
	})
}

// TestLoadProjectManifestUnchanged proves build parsing is untouched: the same
// files load (or fail) as before, ignoring unknown and x- keys.
func TestLoadProjectManifestUnchanged(t *testing.T) {
	load := func(file string) (*ProjectManifest, error) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "nself.yaml"), readManifestFixture(t, file), 0o600); err != nil {
			t.Fatal(err)
		}
		return LoadProjectManifest(dir)
	}
	m, err := load("x-extensions.yaml")
	if err != nil || m.App != "demo" || m.Bundle != "task" || !reflect.DeepEqual(m.Plugins.Free, []string{"cron", "notify"}) || !reflect.DeepEqual(m.Plugins.Pro, []string{"ai"}) {
		t.Errorf("x-extensions load: %+v %v", m, err)
	}
	if m, err = load("type-error.yaml"); err != nil || m.App != "demo" {
		t.Errorf("plugins: 7 is ignored by build today, got %+v %v", m, err)
	}
	if m, err = load("ntask.yaml"); err != nil || m.App != "ntask" || m.Bundle != "ntask" {
		t.Errorf("ntask load: %+v %v", m, err)
	}
	if _, err = load("nclaw.yaml"); err == nil {
		t.Error("nclaw plugins.pro map entries must still fail in build")
	}
	if m, err = load("merge-seq.yaml"); err != nil || !reflect.DeepEqual(m.Plugins.Free, []string{"cron"}) || !reflect.DeepEqual(m.Plugins.Pro, []string{"ai"}) {
		t.Errorf("build honours merge keys, got %+v %v", m, err)
	}
	if _, err = load("dup-key.yaml"); err == nil {
		t.Error("build rejects duplicate keys")
	}
	if m, err = load("multidoc.yaml"); err != nil || m.App != "a" {
		t.Errorf("build reads the first document only, got %+v %v", m, err)
	}
	if m, err = load("null-root.yaml"); err != nil || m == nil {
		t.Errorf("build accepts a null document, got %+v %v", m, err)
	}
	if m, err = load("scalar-coerced.yaml"); err != nil || m.App != "true" || m.Bundle != "1.5" {
		t.Errorf("build coerces scalars to strings, got %+v %v", m, err)
	}
	if m, err = load("nchat.yaml"); err != nil || m.App != "" {
		t.Errorf("nchat load: %+v %v", m, err)
	}
}

// nodeJSON converts a YAML node to a JSON-ready value by tag, so the schema
// sees the same types the Go validator sees (a timestamp is not a string).
func nodeJSON(n *yaml.Node) any {
	switch n.Kind {
	case yaml.DocumentNode:
		return nodeJSON(n.Content[0])
	case yaml.AliasNode:
		return nodeJSON(n.Alias)
	case yaml.MappingNode:
		m := map[string]any{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			m[n.Content[i].Value] = nodeJSON(n.Content[i+1])
		}
		return m
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			out = append(out, nodeJSON(c))
		}
		return out
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return nil
	}
	if _, ok := v.(time.Time); ok {
		return map[string]any{}
	}
	switch x := v.(type) {
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return v
}

func loadSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "schemas", "nself-yaml.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("nself-yaml.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("nself-yaml.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestManifestValidateAgreesWithSchema: for every fixture the Go validator and
// the generated JSON Schema give the same verdict (valid / invalid).
func TestManifestValidateAgreesWithSchema(t *testing.T) {
	schema := loadSchema(t)
	files, err := filepath.Glob(filepath.Join(fixtureDir, "*.yaml"))
	if err != nil || len(files) < 10 {
		t.Fatalf("expected at least 10 fixtures, found %d (%v)", len(files), err)
	}
	extra := map[string]string{
		"inline-timestamp-app": "app: 2024-01-01\n",
		"inline-bool-bundle":   "bundle: true\n",
		"inline-x-in-plugins":  "plugins:\n  x-a: 1\n  free: [a]\n",
		"inline-unknown-deep":  "plugins:\n  paid: [a]\n",
		"inline-list-of-map":   "plugins: [{a: b}]\n",
		"inline-bundles-null":  "bundles: [a, ~]\n",
		"inline-alias":         "x-base: &b [cron]\nplugins: *b\n",
		"inline-null-root":     "null\n",
	}
	cases := map[string][]byte{}
	// The schema validates a JSON tree, which has no merge keys, duplicate
	// keys or second documents; for those files the Go validator is
	// authoritative (see manifest_validate.go), so they are not compared.
	skip := func(name string) bool {
		return strings.HasPrefix(name, "merge-") || strings.HasPrefix(name, "dup-") || strings.HasPrefix(name, "multidoc")
	}
	skipped := 0
	for _, f := range files {
		if skip(filepath.Base(f)) {
			skipped++
			continue
		}
		cases[filepath.Base(f)] = readManifestFixture(t, filepath.Base(f))
	}
	if skipped < 5 {
		t.Errorf("expected the merge/dup/multidoc fixtures to be skipped, skipped %d", skipped)
	}
	for k, v := range extra {
		cases[k] = []byte(v)
	}
	var names []string
	for k := range cases {
		names = append(names, k)
	}
	sort.Strings(names)
	valid, invalid := 0, 0
	for _, name := range names {
		data := cases[name]
		var doc yaml.Node
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if doc.Kind == 0 {
			continue
		}
		goValid := len(ValidateManifestBytes(name, data)) == 0
		schemaValid := schema.Validate(nodeJSON(&doc)) == nil
		if goValid != schemaValid {
			t.Errorf("%s: Go validator valid=%v, JSON Schema valid=%v", name, goValid, schemaValid)
		}
		if goValid {
			valid++
		} else {
			invalid++
		}
	}
	if valid < 3 || invalid < 6 {
		t.Errorf("vacuous agreement run: %d valid, %d invalid", valid, invalid)
	}
}
