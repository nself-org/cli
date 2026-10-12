package hasurameta

// Purpose: reduce a source's export_metadata document to what the bundle can carry.
// Inputs: raw export and the bundle's dumped tables. Outputs: canonical JSON and counts.
// Constraints: allowlist per table and per function (event triggers and unknown keys are dropped,
// so a webhook URL or header value cannot leak); connection configuration is never copied; output
// is sorted. Whatever is tracked in the source but not carried is returned as a metadata exemption,
// so a pass never hides a dropped view or database source.
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

// portableFunctionKeys is the function metadata kept.
var portableFunctionKeys = []string{"function", "configuration", "permissions"}

// Filter keeps the tables of source "default" that the bundle dumped, and the functions in
// the dumped schemas. The counts feed manifest.hasura; the exemptions list each tracked table or
// function and each extra database source that was not carried.
func Filter(raw []byte, db portable.DB) ([]byte, portable.Hasura, []portable.Exemption, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, portable.Hasura{}, nil, errs.New("E520", msgMetadata)
	}
	if inner, ok := root["metadata"].(map[string]any); ok {
		root = inner
	}
	if _, ok := root["sources"].([]any); !ok {
		return nil, portable.Hasura{}, nil, errs.New("E520", msgMetadata)
	}
	counts := portable.Hasura{Skipped: portable.HasuraSkipped{RemoteSchemas: len(list(root["remote_schemas"])), Actions: len(list(root["actions"]))}}
	allowed := map[string]bool{}
	for _, t := range db.Tables {
		allowed[t.Schema+"\x00"+t.Name] = true
	}
	schemas := map[string]bool{}
	for _, s := range db.Schemas {
		schemas[s] = true
	}
	sources := []any{}
	exempt := []portable.Exemption{}
	for _, item := range list(root["sources"]) {
		src, ok := item.(map[string]any)
		if !ok {
			return nil, counts, nil, errs.New("E520", msgMetadataSource)
		}
		if src["name"] != "default" {
			name, _ := src["name"].(string)
			exempt = append(exempt, portable.Exemption{Kind: "metadata", ID: "source " + name, Reason: reasonSource})
			continue
		}
		tables, skipped, err := keepTables(list(src["tables"]), allowed, &counts)
		if err != nil {
			return nil, counts, nil, err
		}
		functions, skippedFns, err := keepFunctions(list(src["functions"]), schemas)
		if err != nil {
			return nil, counts, nil, err
		}
		counts.Functions += len(functions)
		exempt = append(exempt, skipped...)
		exempt = append(exempt, skippedFns...)
		sources = append(sources, map[string]any{"name": src["name"], "kind": src["kind"], "tables": sorted(tables), "functions": sorted(functions)})
	}
	if len(sources) == 0 {
		return nil, counts, nil, errs.New("E520", msgNoDefault)
	}
	sort.Slice(exempt, func(i, j int) bool { return exempt[i].ID < exempt[j].ID })
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{"version": 3, "sources": sources}); err != nil {
		return nil, counts, nil, errs.New("E520", msgMetadataEncode)
	}
	return buf.Bytes(), counts, exempt, nil
}

const (
	reasonSource   = "database source other than default is not carried"
	reasonTable    = "tracked object not carried: not a dumped base table (view, materialized view, foreign table or excluded schema)"
	reasonFunction = "tracked function not carried: its schema was not dumped"
)

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

func keepTables(records []any, allowed map[string]bool, counts *portable.Hasura) ([]any, []portable.Exemption, error) {
	out := []any{}
	skipped := []portable.Exemption{}
	for _, item := range records {
		t, ok := item.(map[string]any)
		if !ok {
			return nil, nil, errs.New("E520", msgMetadataTable)
		}
		ref, ok := t["table"].(map[string]any)
		if !ok {
			return nil, nil, errs.New("E520", msgMetadataTable)
		}
		schema, _ := ref["schema"].(string)
		name, _ := ref["name"].(string)
		if !allowed[schema+"\x00"+name] {
			skipped = append(skipped, portable.Exemption{Kind: "metadata", ID: schema + "." + name, Reason: reasonTable})
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
	return out, skipped, nil
}

func keepFunctions(records []any, schemas map[string]bool) ([]any, []portable.Exemption, error) {
	out := []any{}
	skipped := []portable.Exemption{}
	for _, item := range records {
		f, ok := item.(map[string]any)
		if !ok {
			return nil, nil, errs.New("E520", msgMetadataFunction)
		}
		ref, ok := f["function"].(map[string]any)
		if !ok {
			return nil, nil, errs.New("E520", msgMetadataFunction)
		}
		schema, _ := ref["schema"].(string)
		name, _ := ref["name"].(string)
		if strings.HasPrefix(name, "np_") {
			continue
		}
		if !schemas[schema] {
			skipped = append(skipped, portable.Exemption{Kind: "metadata", ID: "function " + schema + "." + name, Reason: reasonFunction})
			continue
		}
		kept := make(map[string]any, len(portableFunctionKeys))
		for _, k := range portableFunctionKeys {
			if v, present := f[k]; present {
				kept[k] = v
			}
		}
		out = append(out, kept)
	}
	return out, skipped, nil
}

// Check runs the structural checks of Filter on a raw export (a document with no usable sources
// or no default source) without a table list, so --plan fails where the real run would.
func Check(raw []byte) error {
	_, _, _, err := Filter(raw, portable.DB{})
	return err
}
