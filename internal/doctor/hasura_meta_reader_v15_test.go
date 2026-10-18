package doctor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"gopkg.in/yaml.v3"
)

// pluginAuditTables is a real plugin's Hasura metadata (plugins origin/main,
// free/audit-log/hasura/tables.yaml): one np_* table with source_account_id.
func pluginAuditTables(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "isolation", "plugin-audit-log-tables.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestHasuraMetadataInterchange: the real plugin file passes PERM-HASURA-01;
// with its user filter switched to X-Hasura-Tenant-Id it warns in v1.4 and fails
// in v1.5, and the same document fails ISO-04 in both modes.
func TestHasuraMetadataInterchange(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		clean := pluginAuditTables(t)
		swapped := strings.Replace(clean, "_eq: X-Hasura-Source-Account-Id", "_eq: X-Hasura-Tenant-Id", 1)
		if swapped == clean {
			t.Fatal("fixture has no source-account filter to switch")
		}

		dir := t.TempDir()
		writeTableYAML(t, dir, "tables.yaml", clean)
		for _, r := range CheckHasuraMetadataYAML(context.Background(), dir, false) {
			if r.Status != "pass" {
				t.Fatalf("real plugin metadata must pass PERM-HASURA-01, got %+v", r)
			}
		}

		dir = t.TempDir()
		writeTableYAML(t, dir, "tables.yaml", swapped)
		want := "warn"
		if compat.V15() {
			want = "fail"
		}
		res := CheckHasuraMetadataYAML(context.Background(), dir, false)
		if len(res) != 1 || res[0].Status != want || !strings.Contains(res[0].Message, "HASURA-FILTER-INTERCHANGE") ||
			!strings.Contains(res[0].Message, "np_auditlog_events") {
			t.Fatalf("mode %s: want one %s interchange result, got %+v", compat.Mode(), want, res)
		}

		// The same swapped document, as live metadata, fails ISO-04 (both modes).
		var list []map[string]any
		if err := yaml.Unmarshal([]byte(swapped), &list); err != nil {
			t.Fatal(err)
		}
		doc, err := json.Marshal(map[string]any{"metadata": map[string]any{"sources": []any{map[string]any{"name": "default", "tables": list}}}})
		if err != nil {
			t.Fatal(err)
		}
		tables, _, err := parseIsolationMetadata(doc)
		if err != nil {
			t.Fatal(err)
		}
		cols := isoColumns{"public.np_auditlog_events": {isoColSAID: true}}
		found := false
		for _, f := range isolationRules(tables, cols, false) {
			if f.Rule == "ISO-04" && f.Table == "public.np_auditlog_events" && f.Role == "user" && f.Op == "select" && f.Fail {
				found = true
			}
			if f.Rule == "ISO-02" {
				t.Errorf("np_* table must stay outside ISO-02: %+v", f)
			}
		}
		if !found {
			t.Error("ISO-04 did not fire on the switched plugin filter")
		}
		// The unmodified plugin table, as live metadata, has no finding at all.
		var cleanList []map[string]any
		if err := yaml.Unmarshal([]byte(clean), &cleanList); err != nil {
			t.Fatal(err)
		}
		doc, _ = json.Marshal(map[string]any{"sources": []any{map[string]any{"name": "default", "tables": cleanList}}})
		tables, _, err = parseIsolationMetadata(doc)
		if err != nil {
			t.Fatal(err)
		}
		if got := isolationRules(tables, cols, true); len(got) != 0 {
			t.Errorf("real plugin metadata has isolation findings: %v", isoKeys(got))
		}
	})
}
