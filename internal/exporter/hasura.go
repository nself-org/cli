package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/portable"
)

// Purpose: export metadata for this database's selected tables and key names.
// Inputs: live Hasura metadata. Outputs: credential-free source subset.
// Constraints: connection configurations, remote schemas and actions omitted.
func writeMetadata(ctx context.Context, cfg *config.Config, w *portable.Writer, m *portable.Manifest, keys []string) error {
	raw, err := database.HasuraExportMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	filtered, summary, err := filterMetadata(raw, m.DB)
	if err != nil {
		return err
	}
	m.Hasura = summary
	if _, err = w.WriteFile("hasura/metadata.json", bytes.NewReader(filtered)); err != nil {
		return err
	}
	keys = append([]string{}, keys...)
	sort.Strings(keys)
	_, err = w.WriteFile("config/keys.txt", strings.NewReader(strings.Join(keys, "\n")+"\n"))
	return err
}

func filterMetadata(raw []byte, db portable.DB) ([]byte, portable.Hasura, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, portable.Hasura{}, err
	}
	if metadata, ok := root["metadata"].(map[string]any); ok {
		root = metadata
	}
	result := map[string]any{"version": 3, "sources": []any{}}
	counts := portable.Hasura{Skipped: portable.HasuraSkipped{RemoteSchemas: len(array(root["remote_schemas"])), Actions: len(array(root["actions"]))}}
	allowed := map[string]bool{}
	for _, t := range db.Tables {
		allowed[t.Schema+"."+t.Name] = true
	}
	schemas := map[string]bool{}
	for _, s := range db.Schemas {
		schemas[s] = true
	}
	for _, item := range array(root["sources"]) {
		source, ok := item.(map[string]any)
		if !ok {
			return nil, counts, safeFailure("metadata source")
		}
		if source["name"] != "default" {
			continue
		}
		out := map[string]any{"name": source["name"], "kind": source["kind"], "tables": []any{}, "functions": []any{}}
		tables, err := metadataTables(array(source["tables"]), allowed, &counts)
		if err != nil {
			return nil, counts, err
		}
		functions := metadataFunctions(array(source["functions"]), schemas)
		out["tables"], out["functions"] = tables, functions
		counts.Functions += len(functions)
		// API metadata arrays are not a canonical order; sort by encoded identity.
		for _, key := range []string{"tables", "functions"} {
			items := array(out[key])
			sort.Slice(items, func(i, j int) bool {
				a, _ := json.Marshal(items[i])
				b, _ := json.Marshal(items[j])
				return bytes.Compare(a, b) < 0
			})
			out[key] = items
		}
		result["sources"] = append(array(result["sources"]), out)
	}
	b, err := json.MarshalIndent(result, "", "  ")
	return append(b, '\n'), counts, err
}

func array(v any) []any { a, _ := v.([]any); return a }

// Purpose: retain dumped tables and count their portable metadata.
// Inputs: records and exact table set. Outputs: selected records.
// Constraints: connections are excluded at the source level.
func metadataTables(records []any, allowed map[string]bool, counts *portable.Hasura) ([]any, error) {
	out := []any{}
	for _, item := range records {
		t, ok := item.(map[string]any)
		if !ok {
			return nil, safeFailure("metadata table")
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
		out = append(out, t)
		counts.Tables++
		for _, k := range []string{"object_relationships", "array_relationships"} {
			counts.Relationships += len(array(t[k]))
		}
		for _, k := range []string{"select_permissions", "insert_permissions", "update_permissions", "delete_permissions"} {
			counts.Permissions += len(array(t[k]))
		}
	}
	return out, nil
}

// Purpose: retain functions in exported schemas. Inputs: source functions.
// Outputs: selected records. Constraints: plugin functions excluded.
func metadataFunctions(records []any, schemas map[string]bool) []any {
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
