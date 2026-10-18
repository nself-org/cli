package doctor

import (
	"context"
	"strings"
	"testing"
)

// TestIsolationFollowups covers the final-review fixes: materialized views,
// tracked tables the column query cannot see, default-source surfaces beyond
// table permissions, inherited-role order, and the static interchange check on
// write permissions.
func TestIsolationFollowups(t *testing.T) {
	fixtures := loadIsoFixtures(t)
	fx := fixtures["template-ok"]

	t.Run("column query reads pg_catalog so materialized views are seen", func(t *testing.T) {
		for _, want := range []string{"pg_attribute", "'m'", "attisdropped"} {
			if !strings.Contains(isoColumnsSQL, want) {
				t.Errorf("column query lacks %s", want)
			}
		}
		if strings.Contains(isoColumnsSQL, "information_schema.columns") {
			t.Error("information_schema hides materialized views and unprivileged columns")
		}
	})

	t.Run("a tracked table the database does not have is a failure", func(t *testing.T) {
		meta := strings.Replace(string(fx.Metadata), `"tables": [`, `"tables": [{"table": {"schema": "public", "name": "missing_relation"}},`, 1)
		dir := isoProject(t, []byte(meta), nil, psqlRows(fx), nil)
		res := IsolationChecks(context.Background(), dir)
		if len(res) != 1 || res[0].Status != "fail" || !strings.Contains(res[0].Message, "public.missing_relation") {
			t.Fatalf("want one fail naming public.missing_relation, got %+v", res)
		}
	})

	t.Run("default source surfaces outside table permissions are reported when a wall exists", func(t *testing.T) {
		surfaces := `"name": "default", "native_queries": [{"root_field_name": "all_notes"}], ` +
			`"functions": [{"function": {"schema": "public", "name": "purge_note"}, "configuration": {"exposed_as": "mutation"}, "permissions": [{"role": "user"}]},` +
			`{"function": {"schema": "public", "name": "find_note"}, "configuration": {"exposed_as": "query"}, "permissions": [{"role": "user"}]}]`
		meta := strings.Replace(string(fx.Metadata), `"name": "default"`, surfaces, 1)
		dir := isoProject(t, []byte(meta), nil, psqlRows(fx), nil)
		got := ""
		for _, r := range IsolationChecks(context.Background(), dir) {
			if r.Status == "fail" {
				got += r.Message + "\n"
			}
		}
		if !strings.Contains(got, "native queries") || !strings.Contains(got, "public.purge_note for role user") || strings.Contains(got, "find_note") {
			t.Fatalf("want native queries and purge_note (not find_note) reported, got %q", got)
		}
		single := fixtures["single-app"]
		meta = strings.Replace(string(single.Metadata), `"name": "default"`, surfaces, 1)
		dir = isoProject(t, []byte(meta), nil, psqlRows(single), nil)
		for _, r := range IsolationChecks(context.Background(), dir) {
			if r.Status != "pass" {
				t.Errorf("a single-app project with no isolation column must not fail on surfaces: %+v", r)
			}
		}
	})

	t.Run("inherited roles do not depend on list order", func(t *testing.T) {
		const tbl = `{"table":{"schema":"public","name":"notes"},` +
			`"select_permissions":[{"role":"user","permission":{"columns":["id"],"filter":{"source_account_id":{"_eq":"X-Hasura-Source-Account-Id"}}}},` +
			`{"role":"leaky","permission":{"columns":["id"],"filter":{}}}],` +
			`"delete_permissions":[{"role":"leaky","permission":{"filter":{}}}]}`
		outer := `{"role_name":"outer","role_set":["inner"]}`
		inner := `{"role_name":"inner","role_set":["user","leaky"]}`
		for _, order := range []string{outer + "," + inner, inner + "," + outer} {
			doc := `{"version":3,"inherited_roles":[` + order + `],"sources":[{"name":"default","tables":[` + tbl + `]}]}`
			meta, err := parseIsolationMetadata([]byte(doc))
			if err != nil {
				t.Fatal(err)
			}
			perms := meta.Tables[0].Perms
			if _, ok := perms["outer"]["select"]; !ok {
				t.Errorf("order %q: outer has no inherited select", order)
			}
			if _, ok := perms["outer (via leaky)"]["delete"]; !ok {
				t.Errorf("order %q: outer has no inherited delete", order)
			}
		}
	})

	t.Run("a cyclic role set terminates", func(t *testing.T) {
		doc := `{"version":3,"inherited_roles":[{"role_name":"a","role_set":["b"]},{"role_name":"b","role_set":["a","user"]}],` +
			`"sources":[{"name":"default","tables":[{"table":{"schema":"public","name":"notes"},"select_permissions":[{"role":"user","permission":{"filter":{}}}]}]}]}`
		if _, err := parseIsolationMetadata([]byte(doc)); err != nil {
			t.Fatal(err)
		}
	})
}

// TestHasuraMetadataInterchangeOnWritePermissions: PERM-HASURA-01 also reads the
// check and set of insert/update permissions and the filter of update/delete.
func TestHasuraMetadataInterchangeOnWritePermissions(t *testing.T) {
	for name, perm := range map[string]string{
		"insert set":   "insert_permissions:\n  - role: user\n    permission:\n      check: {}\n      set:\n        source_account_id: X-Hasura-Tenant-Id\n",
		"update check": "update_permissions:\n  - role: user\n    permission:\n      filter: {}\n      check:\n        tenant_id:\n          _eq: X-Hasura-Source-Account-Id\n",
		"delete":       "delete_permissions:\n  - role: user\n    permission:\n      filter:\n        source_account_id:\n          _eq: X-Hasura-Tenant-Id\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeTableYAML(t, dir, "tables.yaml", "- table:\n    schema: public\n    name: np_auditlog_events\n  "+strings.ReplaceAll(strings.TrimSuffix(perm, "\n"), "\n", "\n  ")+"\n")
			res := CheckHasuraMetadataYAML(context.Background(), dir, false)
			if len(res) != 1 || res[0].Status == "pass" || !strings.Contains(res[0].Message, "HASURA-FILTER-INTERCHANGE") {
				t.Fatalf("want an interchange result, got %+v", res)
			}
		})
	}
}
