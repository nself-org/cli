package exporter

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/portable"
)

func writeJSON(t *testing.T, w io.Writer, v any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataDropsEventTriggers(t *testing.T) {
	raw := []byte(`{"version":3,"sources":[{"name":"default","kind":"postgres","tables":[{"table":{"schema":"public","name":"notes"},"event_triggers":[{"name":"hook","webhook":"https://FAKE-HOOK.invalid/x","headers":[{"name":"X-Fake","value":"FAKE-LITERAL-HEADER"}]}],"select_permissions":[{"role":"user"}],"object_relationships":[{"name":"owner"}]}],"functions":[]}]}`)
	b, c, err := filterMetadata(raw, portable.DB{Schemas: []string{"public"}, Tables: []portable.Table{{Schema: "public", Name: "notes"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"FAKE-LITERAL-HEADER", "FAKE-HOOK", "event_triggers", "webhook"} {
		if strings.Contains(string(b), bad) {
			t.Fatal("event trigger data exported", bad)
		}
	}
	if !strings.Contains(string(b), "select_permissions") || !strings.Contains(string(b), "owner") || c.Permissions != 1 || c.Relationships != 1 {
		t.Fatal("portable table metadata lost", c)
	}
}

func TestMetadataSubset(t *testing.T) {
	raw := []byte(`{"version":3,"sources":[{"name":"default","kind":"postgres","configuration":{"connection_info":{"database_url":"FAKE-METADATA-SECRET"}},"tables":[{"table":{"schema":"public","name":"notes"},"select_permissions":[{"role":"user"}]},{"table":{"schema":"public","name":"np_plugins"}},{"table":{"schema":"other","name":"hidden"}}],"functions":[{"function":{"schema":"public","name":"search"}},{"function":{"schema":"public","name":"np_search"}}]},{"name":"remote","tables":[]}],"remote_schemas":[{}],"actions":[{}]}`)
	b, c, err := filterMetadata(raw, portable.DB{Schemas: []string{"public"}, Tables: []portable.Table{{Schema: "public", Name: "notes"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"FAKE-METADATA-SECRET", "np_plugins", "hidden", "np_search", "configuration"} {
		if strings.Contains(string(b), bad) {
			t.Fatal("exported excluded metadata", bad)
		}
	}
	if c.Tables != 1 || c.Permissions != 1 || c.Functions != 1 || c.Skipped.Actions != 1 || c.Skipped.RemoteSchemas != 1 {
		t.Fatalf("counts %+v", c)
	}
	if _, _, err = filterMetadata([]byte(`{`), portable.DB{}); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	for _, s := range []string{"auth", "np_private", "pg_catalog", "information_schema", "nself_ops", "hdb_catalog"} {
		if !excludedSchema(s) {
			t.Fatal("schema admitted", s)
		}
	}
	if excludedSchema("app") {
		t.Fatal("user schema excluded")
	}
}
