package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// isoProject writes a minimal project dir and stubs the two I/O seams.
func isoProject(t *testing.T, meta []byte, metaErr error, psql string, psqlErr error) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=fx\nENV=dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(meta), "database_url") { // a live export always states it
		meta = []byte(strings.Replace(string(meta), `"name": "default"`, `"name": "default", "configuration": {"connection_info": {"database_url": {"from_env": "HASURA_GRAPHQL_DATABASE_URL"}}}`, 1))
	}
	savedMeta, savedExec := exportLiveMetadata, isoExecCapture
	t.Cleanup(func() { exportLiveMetadata, isoExecCapture = savedMeta, savedExec })
	exportLiveMetadata = func(context.Context, *config.Config) ([]byte, error) { return meta, metaErr }
	isoExecCapture = func(_ context.Context, container string, cmd []string) (string, string, error) {
		sql := cmd[len(cmd)-1]
		if container != "fx_postgres" || cmd[0] != "psql" || (sql != isoColumnsSQL && sql != isoRelationsSQL) {
			t.Errorf("unexpected exec: %s %v", container, cmd)
		}
		if sql == isoRelationsSQL && psqlErr == nil { // every tracked table exists unless a test removes it
			parsed, err := parseIsolationMetadata(meta)
			if err != nil {
				return "", "", nil
			}
			var rows []string
			for _, tb := range parsed.Tables {
				if tb.Name != "missing_relation" {
					rows = append(rows, tb.Schema+isoFieldSep+tb.Name)
				}
			}
			return strings.Join(rows, "\n") + "\n", "", nil
		}
		return psql, "", psqlErr
	}
	return dir
}

// psqlRows renders a fixture's columns the way psql -A -t -F US prints them.
func psqlRows(f isoFixture) string {
	var rows []string
	for id, names := range f.Columns {
		schema, table, _ := strings.Cut(id, ".")
		for _, n := range names {
			rows = append(rows, schema+isoFieldSep+table+isoFieldSep+n)
		}
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n") + "\n"
}

// TestIsolationChecks runs the whole entry point over the fixtures: the
// section is "isolation", failures name table, role and operation, and a clean
// project gives only passes.
func TestIsolationChecks(t *testing.T) {
	fixtures := loadIsoFixtures(t)
	t.Run("template shape passes", func(t *testing.T) {
		fx := fixtures["template-ok"]
		dir := isoProject(t, fx.Metadata, nil, psqlRows(fx), nil)
		res := IsolationChecks(context.Background(), dir)
		if len(res) != 4 {
			t.Fatalf("want ISO-01..04 passes, got %+v", res)
		}
		for _, r := range res {
			if r.Section != "isolation" || r.Status != "pass" {
				t.Errorf("want isolation pass, got %+v", r)
			}
		}
	})
	t.Run("missing select filter names table role op rule", func(t *testing.T) {
		fx := fixtures["iso02-select-missing"]
		dir := isoProject(t, fx.Metadata, nil, psqlRows(fx), nil)
		var fails []CheckResult
		for _, r := range IsolationChecks(context.Background(), dir) {
			if r.Status == "fail" {
				fails = append(fails, r)
			}
		}
		if len(fails) != 1 || !strings.Contains(fails[0].Name, "ISO-02") || !strings.Contains(fails[0].Message, "public.notes") ||
			!strings.Contains(fails[0].Message, "role=user") || !strings.Contains(fails[0].Message, "op=select") {
			t.Fatalf("want one ISO-02 fail naming public.notes user select, got %+v", fails)
		}
	})
	t.Run("ISO-01 is a warning, a fail when strict", func(t *testing.T) {
		fx := fixtures["iso01-missing-column"]
		for _, c := range []struct{ env, want string }{{"", "warn"}, {"1", "fail"}} {
			t.Setenv(isolationStrict, c.env)
			dir := isoProject(t, fx.Metadata, nil, psqlRows(fx), nil)
			got := ""
			for _, r := range IsolationChecks(context.Background(), dir) {
				if strings.HasPrefix(r.Name, "ISO-01: public.lookups") {
					got = r.Status
				}
			}
			if got != c.want {
				t.Errorf("strict=%q: ISO-01 status %q, want %q", c.env, got, c.want)
			}
		}
	})
	t.Run("single-app project has nothing to isolate", func(t *testing.T) {
		fx := fixtures["single-app"]
		dir := isoProject(t, fx.Metadata, nil, "", nil)
		res := IsolationChecks(context.Background(), dir)
		if len(res) != 1 || res[0].Status != "pass" || !strings.Contains(res[0].Message, "nothing to isolate") {
			t.Fatalf("got %+v", res)
		}
	})
	t.Run("a second source is reported, not passed", func(t *testing.T) {
		fx := fixtures["second-source"]
		dir := isoProject(t, fx.Metadata, nil, psqlRows(fx), nil)
		got := ""
		for _, r := range IsolationChecks(context.Background(), dir) {
			if r.Status == "fail" {
				got += r.Name + "|" + r.Message
			}
		}
		if !strings.Contains(got, "ISO: source reporting") || !strings.Contains(got, `"reporting"`) {
			t.Fatalf("want a fail naming source reporting, got %q", got)
		}
	})
	t.Run("default source must be proven to read the project database", func(t *testing.T) {
		fx := fixtures["template-ok"]
		for _, c := range []struct{ url, want, contains string }{
			{`"postgresql://u:p@postgres:5432/other"`, "fail", `"other"`},
			{`"postgresql://u:p@other-server:5432/nself"`, "fail", "other-server"},
			{`"postgresql://u:p@postgres:5433/nself"`, "fail", "postgres:5433"},
			{`"not a url"`, "fail", "not understood"},
			{`{"from_env":"SOME_OTHER_URL"}`, "fail", "HASURA_GRAPHQL_DATABASE_URL"},
			{`{"connection_parameters":{"host":"other-server","database":"nself"}}`, "fail", "HASURA_GRAPHQL_DATABASE_URL"},
			{`7`, "fail", "unknown shape"},
			{`"postgresql://u:p@postgres:5432/nself"`, "pass", ""},
			{`"postgresql://u:p@fx_postgres/nself"`, "pass", ""},
			{`{"from_env":"HASURA_GRAPHQL_DATABASE_URL"}`, "pass", ""},
		} {
			meta := strings.Replace(string(fx.Metadata), `"name": "default"`, `"name": "default", "configuration": {"connection_info": {"database_url": `+c.url+`}}`, 1)
			dir := isoProject(t, []byte(meta), nil, psqlRows(fx), nil)
			res := IsolationChecks(context.Background(), dir)
			if res[0].Status != c.want || (c.want == "fail" && (len(res) != 1 || !strings.Contains(res[0].Message, c.contains))) {
				t.Errorf("database_url %s: want %s, got %+v", c.url, c.want, res)
			}
		}
	})
	t.Run("export without a default connection fails closed", func(t *testing.T) {
		doc := `{"version":3,"sources":[{"name":"default","tables":[]}]}`
		meta, err := parseIsolationMetadata([]byte(doc))
		if err != nil || bindDefaultSource(meta.DefaultConn, "nself", "fx_postgres") == "" {
			t.Errorf("want a binding failure, got %v / %+v", err, meta)
		}
	})
	t.Run("column rows are parsed strictly", func(t *testing.T) {
		if _, err := parseIsolationColumns("public" + isoFieldSep + "notes\n"); err == nil {
			t.Error("short row accepted")
		}
	})
}

// TestIsolationReadErrorFails: every read or parse failure is one fail
// result in section isolation and never a pass (Constitution §4.1).
func TestIsolationReadErrorFails(t *testing.T) {
	good := loadIsoFixtures(t)["template-ok"]
	rows := psqlRows(good)
	cases := []struct {
		name     string
		meta     []byte
		metaErr  error
		psql     string
		psqlErr  error
		contains string
	}{
		{"export error", nil, errors.New("connection refused"), rows, nil, "connection refused"},
		{"not json", []byte("{nope"), nil, rows, nil, "not understood"},
		{"no sources", []byte(`{"version":3}`), nil, rows, nil, "no sources"},
		{"postgres unreachable", good.Metadata, nil, "", errors.New("no such container"), "no such container"},
		{"garbled rows", good.Metadata, nil, "just-one-field\n", nil, "unexpected column row"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := isoProject(t, c.meta, c.metaErr, c.psql, c.psqlErr)
			res := IsolationChecks(context.Background(), dir)
			if len(res) != 1 || res[0].Status != "fail" || res[0].Section != "isolation" || !strings.Contains(res[0].Message, c.contains) {
				t.Fatalf("want one isolation fail containing %q, got %+v", c.contains, res)
			}
		})
	}
	if res := IsolationChecks(context.Background(), ""); len(res) != 1 || res[0].Status != "fail" {
		t.Errorf("empty project dir must fail, got %+v", res)
	}
}
