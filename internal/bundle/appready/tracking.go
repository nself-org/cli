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
	"fmt"
	"sort"
	"strings"
	"time"
)

// TrackedTables parses an export_metadata response into the set of tracked
// tables, keyed "schema.name". It reads sources[].tables[].table and, for older
// metadata, a top-level tables array.
func TrackedTables(raw []byte) (map[string]bool, error) {
	var doc struct {
		Sources []struct {
			Tables []tableEntry `json:"tables"`
		} `json:"sources"`
		Tables []tableEntry `json:"tables"`
		// Hasura wraps the metadata in a "metadata" key on some endpoints.
		Metadata *struct {
			Sources []struct {
				Tables []tableEntry `json:"tables"`
			} `json:"sources"`
			Tables []tableEntry `json:"tables"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("export_metadata response is not JSON: %w", err)
	}
	set := map[string]bool{}
	add := func(ts []tableEntry) {
		for _, t := range ts {
			if t.Table.Name == "" {
				continue
			}
			schema := t.Table.Schema
			if schema == "" {
				schema = "public"
			}
			set[schema+"."+t.Table.Name] = true
		}
	}
	for _, s := range doc.Sources {
		add(s.Tables)
	}
	add(doc.Tables)
	if doc.Metadata != nil {
		for _, s := range doc.Metadata.Sources {
			add(s.Tables)
		}
		add(doc.Metadata.Tables)
	}
	return set, nil
}

type tableEntry struct {
	Table struct {
		Schema string `json:"schema"`
		Name   string `json:"name"`
	} `json:"table"`
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
