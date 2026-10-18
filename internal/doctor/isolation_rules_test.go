package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// isoFixture is one testdata/isolation/*.json file: a live export_metadata
// document, the isolation columns per table, and the findings it must yield.
type isoFixture struct {
	Description string              `json:"description"`
	Strict      bool                `json:"strict"`
	Unchecked   []string            `json:"unchecked"`
	Columns     map[string][]string `json:"columns"`
	Metadata    json.RawMessage     `json:"metadata"`
	Want        []struct {
		Rule, Table, Role, Op string
		Fail                  bool
	} `json:"want"`
}

func loadIsoFixtures(t *testing.T) map[string]isoFixture {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "isolation", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no isolation fixtures: %v", err)
	}
	out := map[string]isoFixture{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var f isoFixture
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		out[strings.TrimSuffix(filepath.Base(p), ".json")] = f
	}
	return out
}

func (f isoFixture) columns() isoColumns {
	cols := isoColumns{}
	for id, names := range f.Columns {
		cols[id] = map[string]bool{}
		for _, n := range names {
			cols[id][n] = true
		}
	}
	return cols
}

func isoKeys(fs []isoFinding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, fmt.Sprintf("%s %s %s %s fail=%v", f.Rule, f.Table, f.Role, f.Op, f.Fail))
	}
	sort.Strings(out)
	return out
}

// TestIsolationRules runs ISO-01..ISO-04 over every fixture and compares the
// exact findings (rule, table, role, operation, severity).
func TestIsolationRules(t *testing.T) {
	fixtures := loadIsoFixtures(t)
	for _, must := range []string{"template-ok", "iso02-select-missing", "iso02-insert-no-check", "iso03-tenant-missing",
		"iso04-interchange-select", "iso01-missing-column", "single-app"} {
		if _, ok := fixtures[must]; !ok {
			t.Fatalf("fixture %s missing", must)
		}
	}
	for name, fx := range fixtures {
		t.Run(name, func(t *testing.T) {
			meta, err := parseIsolationMetadata(fx.Metadata)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(meta.Unchecked, ",") != strings.Join(fx.Unchecked, ",") {
				t.Errorf("unchecked sources = %v, want %v", meta.Unchecked, fx.Unchecked)
			}
			var want []string
			for _, w := range fx.Want {
				want = append(want, fmt.Sprintf("%s %s %s %s fail=%v", w.Rule, w.Table, w.Role, w.Op, w.Fail))
			}
			sort.Strings(want)
			got := isoKeys(isolationRules(meta.Tables, fx.columns(), fx.Strict))
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("%s\ngot:\n%s\nwant:\n%s", fx.Description, strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

// TestIsolationTemplateFile keeps the guard in step with the shipped template:
// the real multi-app metadata.json must produce no finding.
func TestIsolationTemplateFile(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "templates", "clone", "multi-app", "hasura", "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	meta, err := parseIsolationMetadata(b)
	if err != nil {
		t.Fatal(err)
	}
	cols := isoColumns{"public.notes": {isoColSAID: true}, "public.projects": {isoColSAID: true}, "public.app_members": {isoColSAID: true}}
	if got := isolationRules(meta.Tables, cols, true); len(got) != 0 {
		t.Fatalf("template shape has findings: %v", isoKeys(got))
	}
}

// TestIsolationFilterShapes covers the matcher on trees the fixtures do not:
// implicit AND, nested _and/_or, wrong variable, wrong column, non-string value.
func TestIsolationFilterShapes(t *testing.T) {
	js := func(s string) any {
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	cases := []struct {
		name   string
		filter string
		pins   bool
	}{
		{"direct", `{"source_account_id":{"_eq":"X-Hasura-Source-Account-Id"}}`, true},
		{"lowercase variable", `{"source_account_id":{"_eq":"x-hasura-source-account-id"}}`, true},
		{"implicit and", `{"title":{"_eq":"a"},"source_account_id":{"_eq":"X-Hasura-Source-Account-Id"}}`, true},
		{"and branch", `{"_and":[{"title":{"_eq":"a"}},{"source_account_id":{"_eq":"X-Hasura-Source-Account-Id"}}]}`, true},
		{"or one branch", `{"_or":[{"source_account_id":{"_eq":"X-Hasura-Source-Account-Id"}},{"title":{"_eq":"a"}}]}`, false},
		{"or empty", `{"_or":[]}`, false},
		{"empty", `{}`, false},
		{"nil", `null`, false},
		{"wrong variable", `{"source_account_id":{"_eq":"X-Hasura-User-Id"}}`, false},
		{"wrong column", `{"tenant_id":{"_eq":"X-Hasura-Source-Account-Id"}}`, false},
		{"neq", `{"source_account_id":{"_neq":"X-Hasura-Source-Account-Id"}}`, false},
		{"exists only", `{"_exists":{"_table":{"schema":"public","name":"m"},"_where":{"source_account_id":{"_eq":"X-Hasura-Source-Account-Id"}}}}`, false},
		{"literal", `{"source_account_id":{"_eq":"app_a"}}`, false},
	}
	for _, c := range cases {
		if got := pins(js(c.filter), isoColSAID, isoClaimSAID); got != c.pins {
			t.Errorf("%s: pins = %v, want %v", c.name, got, c.pins)
		}
	}
	if !interchange(js(`{"_and":[{"_or":[{"tenant_id":{"_eq":"X-Hasura-Source-Account-Id"}}]}]}`)) {
		t.Error("nested interchange not found")
	}
	if interchange(js(`{"source_account_id":{"_eq":"X-Hasura-Source-Account-Id"},"tenant_id":{"_eq":"X-Hasura-Tenant-Id"}}`)) {
		t.Error("correct pair reported as interchange")
	}
}
