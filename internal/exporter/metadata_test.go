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
