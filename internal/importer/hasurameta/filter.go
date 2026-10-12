package hasurameta

// Purpose: reduce a source's export_metadata document to what the bundle can carry.
// Inputs: raw export and the bundle's dumped tables. Outputs: canonical JSON and counts.
// Constraints: allowlist per table (event triggers and unknown keys are dropped, so a webhook
// URL or header value cannot leak); connection configuration is never copied; output is sorted.
import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/portable"
)

// portableTableKeys is the table metadata kept: the definition, relationships, computed
// fields and permissions.
var portableTableKeys = []string{"table", "is_enum", "configuration", "object_relationships", "array_relationships", "computed_fields", "select_permissions", "insert_permissions", "update_permissions", "delete_permissions"}

// Filter keeps the tables of source "default" that the bundle dumped, and the functions in
// the dumped schemas. The counts feed manifest.hasura.
func Filter(raw []byte, db portable.DB) ([]byte, portable.Hasura, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, portable.Hasura{}, errs.New("E520", "source Hasura metadata unreadable")
	}
	if inner, ok := root["metadata"].(map[string]any); ok {
		root = inner
	}
	counts := portable.Hasura{Skipped: portable.HasuraSkipped{RemoteSchemas: len(list(root["remote_schemas"])), Actions: len(list(root["actions"]))}}
	allowed := map[string]bool{}
	for _, t := range db.Tables {
		allowed[t.Schema+"."+t.Name] = true
	}
	schemas := map[string]bool{}
	for _, s := range db.Schemas {
		schemas[s] = true
	}
	sources := []any{}
	for _, item := range list(root["sources"]) {
		src, ok := item.(map[string]any)
		if !ok {
			return nil, counts, errs.New("E520", "source Hasura metadata has an invalid source")
		}
		if src["name"] != "default" {
			continue
		}
		tables, err := keepTables(list(src["tables"]), allowed, &counts)
		if err != nil {
			return nil, counts, err
		}
		functions := keepFunctions(list(src["functions"]), schemas)
		counts.Functions += len(functions)
		sources = append(sources, map[string]any{"name": src["name"], "kind": src["kind"], "tables": sorted(tables), "functions": sorted(functions)})
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{"version": 3, "sources": sources}); err != nil {
		return nil, counts, errs.New("E520", "source Hasura metadata not encodable")
	}
	return buf.Bytes(), counts, nil
}

func list(v any) []any { a, _ := v.([]any); return a }

// sorted orders records by their encoded form: the API array order is not canonical.
func sorted(items []any) []any {
	keys := make([]string, len(items))
	idx := make([]int, len(items))
	for i, it := range items {
		b, _ := json.Marshal(it)
		keys[i], idx[i] = string(b), i
	}
	sort.Slice(idx, func(a, b int) bool { return keys[idx[a]] < keys[idx[b]] })
	out := make([]any, len(items))
	for i, j := range idx {
		out[i] = items[j]
	}
	return out
}

func keepTables(records []any, allowed map[string]bool, counts *portable.Hasura) ([]any, error) {
	out := []any{}
	for _, item := range records {
		t, ok := item.(map[string]any)
		if !ok {
			return nil, errs.New("E520", "source Hasura metadata has an invalid table")
		}
		ref, ok := t["table"].(map[string]any)
		if !ok {
			continue
		}
		schema, _ := ref["schema"].(string)
		name, _ := ref["name"].(string)
		if !allowed[schema+"."+name] {
			continue
		}
		kept := make(map[string]any, len(portableTableKeys))
		for _, k := range portableTableKeys {
			if v, present := t[k]; present {
				kept[k] = v
			}
		}
		out = append(out, kept)
		counts.Tables++
		for _, k := range []string{"object_relationships", "array_relationships"} {
			counts.Relationships += len(list(t[k]))
		}
		for _, k := range []string{"select_permissions", "insert_permissions", "update_permissions", "delete_permissions"} {
			counts.Permissions += len(list(t[k]))
		}
	}
	return out, nil
}

func keepFunctions(records []any, schemas map[string]bool) []any {
	out := []any{}
	for _, item := range records {
		f, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ref, ok := f["function"].(map[string]any)
		if !ok {
			continue
		}
		schema, _ := ref["schema"].(string)
		name, _ := ref["name"].(string)
		if schemas[schema] && !strings.HasPrefix(name, "np_") {
			out = append(out, f)
		}
	}
	return out
}
