package appready

// tracking.go — the tracking step: every manifest table is tracked in Hasura.
//
// Purpose: a plugin applies its own SQL at boot; the CLI proves Hasura tracks the
// resulting tables by reading export_metadata through the existing admin client.
// Inputs: the raw export_metadata JSON and each member's manifest tables.
// Outputs: TrackedTables (schema.name set) and the first member with a missing table.
// Constraints: a manifest table "x" means "public.x"; "s.x" means schema s.
// Read only: nothing is tracked or changed here.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// TrackedTables parses an export_metadata response into the set of tracked
// postgres tables, keyed "schema.name". It reads sources[].tables[].table and,
// for older metadata, a top-level tables array. Only postgres-kind sources count
// (kind "postgres", "citus", "cockroach", or no kind); a data-connector or
// BigQuery source is ignored. A table entry counts only in the {schema,name}
// object form (schema defaults to public); any other shape is skipped.
func TrackedTables(raw []byte) (map[string]bool, error) {
	var doc metadataDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			return nil, fmt.Errorf("export_metadata response has an unexpected shape (%s is %s, want %s): %w", te.Field, te.Value, te.Type, err)
		}
		return nil, fmt.Errorf("export_metadata response is not JSON: %w", err)
	}
	set := map[string]bool{}
	doc.collect(set)
	if doc.Metadata != nil {
		doc.Metadata.collect(set)
	}
	return set, nil
}

// metadataDoc is export_metadata; Hasura wraps it in a "metadata" key on some endpoints.
type metadataDoc struct {
	Sources  []sourceDoc  `json:"sources"`
	Tables   []tableEntry `json:"tables"`
	Metadata *metadataDoc `json:"metadata"`
}

type sourceDoc struct {
	Kind   string       `json:"kind"`
	Tables []tableEntry `json:"tables"`
}

// tableEntry keeps "table" raw: it is an object for postgres but an array for
// data connectors, and one wrong shape must not fail the whole document.
type tableEntry struct {
	Table json.RawMessage `json:"table"`
}

func (d metadataDoc) collect(set map[string]bool) {
	for _, s := range d.Sources {
		switch strings.ToLower(s.Kind) {
		case "", "postgres", "citus", "cockroach":
			addTables(set, s.Tables)
		}
	}
	addTables(set, d.Tables) // older metadata: a top-level list is postgres
}

func addTables(set map[string]bool, ts []tableEntry) {
	for _, t := range ts {
		var obj struct {
			Schema  string `json:"schema"`
			Name    string `json:"name"`
			Dataset string `json:"dataset"`
		}
		if json.Unmarshal(t.Table, &obj) != nil || obj.Name == "" || obj.Dataset != "" {
			continue // not the {schema,name} object form
		}
		if obj.Schema == "" {
			obj.Schema = "public"
		}
		set[obj.Schema+"."+obj.Name] = true
	}
}

// tableKey normalises a manifest table name to "schema.name".
func tableKey(t string) string {
	if strings.Contains(t, ".") {
		return t
	}
	return "public." + t
}

// Missing returns, for the first member (in order) that has untracked tables,
// that member's name and the sorted missing tables.
func Missing(members []Member, tracked map[string]bool) (string, []string) {
	for _, m := range members {
		var miss []string
		for _, t := range m.Tables {
			if !tracked[tableKey(t)] {
				miss = append(miss, tableKey(t))
			}
		}
		if len(miss) > 0 {
			sort.Strings(miss)
			return m.Name, miss
		}
	}
	return "", nil
}

// waitTracked polls ExportMetadata until every member table is tracked or the
// timeout ends.
func waitTracked(ctx context.Context, h Hooks, members []Member) (string, error) {
	total := 0
	for _, m := range members {
		total += len(m.Tables)
	}
	if total == 0 {
		return "no member declares tables", nil
	}
	deadline := time.Now().Add(h.timeout())
	var last error
	for {
		raw, err := h.ExportMetadata(ctx)
		if err == nil {
			var tracked map[string]bool
			if tracked, err = TrackedTables(raw); err == nil {
				name, miss := Missing(members, tracked)
				if name == "" {
					return fmt.Sprintf("%d tables tracked", total), nil
				}
				err = &StepError{Plugin: name, Err: fmt.Errorf("plugin %s: tables not tracked in Hasura: %s", name, strings.Join(miss, ", "))}
			}
		}
		last = err
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if time.Now().Add(h.interval()).After(deadline) {
			return "", last
		}
		pause(ctx, h.interval())
	}
}
