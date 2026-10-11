package database

// vector_track.go tracks a vector-search SQL function through Hasura with one
// targeted `bulk` metadata call (P7-ADOPT-07, EPIC ADOPT D7).
//
// Purpose: expose <table>_nearest_<column> as a GraphQL query field for the
// named roles, and answer the two on-disk questions the generator needs
// (is the table tracked, does disk match live).
// Inputs: a context, *config.Config, the schema, function and table names and
// the roles.
// Outputs: nil when the function is tracked with every role permission, E513
// when the table or function is missing, E514 when a role has no select
// permission on the table.
// Constraints: never replace_metadata, never HasuraApplyMetadata; only the
// missing pieces are sent (read from a live export), so a rerun is a no-op
// and a partial state is completed; a Hasura "already-tracked" or
// "already-exists" answer to a race is retried once from a fresh export and
// then counts as success.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// vtSource is the one Hasura source nSelf tracks in.
const vtSource = "default"

type vtRef struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
}

type vtRole struct {
	Role string `json:"role"`
}

type vtMeta struct {
	Sources []struct {
		Name   string `json:"name"`
		Tables []struct {
			Table             vtRef    `json:"table"`
			SelectPermissions []vtRole `json:"select_permissions"`
		} `json:"tables"`
		Functions []struct {
			Function    vtRef    `json:"function"`
			Permissions []vtRole `json:"permissions"`
		} `json:"functions"`
	} `json:"sources"`
}

// TrackFunction tracks schema.name (a function returning SETOF schema.table) in
// Hasura and grants each role. See the file comment for the contract.
func TrackFunction(ctx context.Context, cfg *config.Config, schema, name, table string, roles []string) error {
	for attempt := 0; attempt < 2; attempt++ {
		live, err := HasuraExportMetadata(ctx, cfg)
		if err != nil {
			return err
		}
		ops, err := planTrack(live, schema, name, table, roles)
		if err != nil {
			return err
		}
		if len(ops) == 0 {
			return nil
		}
		_, err = postMetadata(ctx, cfg, metadataRequest{Type: "bulk", Args: ops})
		if err == nil {
			return nil
		}
		msg := err.Error()
		switch {
		case hasuraCode(msg, "not-exists") && strings.Contains(msg, "function"):
			return errs.Newf("E513", "function %s.%s does not exist in Postgres; run nself db migrate --migration-dir migrations first", schema, name)
		case hasuraCode(msg, "already-tracked"), hasuraCode(msg, "already-exists"):
			continue // another writer got there first: plan again from live
		}
		return fmt.Errorf("track function %s.%s: %w", schema, name, err)
	}
	return nil
}

// hasuraCode reports whether a Hasura error message carries the given "code".
func hasuraCode(msg, code string) bool {
	return regexp.MustCompile(`"code"\s*:\s*"` + regexp.QuoteMeta(code) + `"`).MatchString(msg)
}

// planTrack reads the live export and returns the bulk operations still needed.
// The table must be tracked live (E513) and every role must hold a select
// permission on it (E514).
func planTrack(live []byte, schema, name, table string, roles []string) ([]map[string]any, error) {
	var m vtMeta
	if err := json.Unmarshal(live, &m); err != nil {
		return nil, fmt.Errorf("parse live metadata: %w", err)
	}
	selectRoles, tableTracked := map[string]bool{}, false
	funcTracked, funcRoles := false, map[string]bool{}
	for _, src := range m.Sources {
		if src.Name != vtSource {
			continue
		}
		for _, t := range src.Tables {
			if t.Table.Schema == schema && t.Table.Name == table {
				tableTracked = true
				for _, p := range t.SelectPermissions {
					selectRoles[p.Role] = true
				}
			}
		}
		for _, f := range src.Functions {
			if f.Function.Schema == schema && f.Function.Name == name {
				funcTracked = true
				for _, p := range f.Permissions {
					funcRoles[p.Role] = true
				}
			}
		}
	}
	if !tableTracked {
		return nil, errs.Newf("E513", "table %s.%s is not tracked in live Hasura", schema, table)
	}
	var lacking []string
	for _, r := range roles {
		if !selectRoles[r] {
			lacking = append(lacking, r)
		}
	}
	if len(lacking) > 0 {
		return nil, errs.Newf("E514", "role(s) %s have no select permission on %s.%s", strings.Join(lacking, ", "), schema, table)
	}
	fn := map[string]string{"schema": schema, "name": name}
	var ops []map[string]any
	if !funcTracked {
		ops = append(ops, map[string]any{"type": "pg_track_function", "args": map[string]any{"source": vtSource, "function": fn}})
	}
	for _, r := range roles {
		if !funcRoles[r] {
			ops = append(ops, map[string]any{"type": "pg_create_function_permission",
				"args": map[string]any{"source": vtSource, "function": fn, "role": r}})
		}
	}
	return ops, nil
}

// MetadataTracksTable reports whether the project's on-disk metadata lists
// schema.table. present is false when the project has no on-disk metadata. It
// reads the sources.yaml of `nself db hasura metadata export`, the Hasura CLI
// layouts and the legacy hasura/metadata.json.
func MetadataTracksTable(projectDir, schema, table string) (present, tracked bool, err error) {
	hasuraDir := filepath.Join(projectDir, "hasura")
	var tables map[string]hasuraTable
	srcYAML := filepath.Join(hasuraDir, "metadata", "sources.yaml")
	if data, readErr := os.ReadFile(srcYAML); readErr == nil {
		var sources []hasuraSource
		if err := yaml.Unmarshal(data, &sources); err != nil {
			return true, false, fmt.Errorf("parse %s: %w", srcYAML, err)
		}
		var all []hasuraTable
		for _, s := range sources {
			all = append(all, s.Tables...)
		}
		tables = tablesByKey(all)
	} else {
		_, dirErr := os.Stat(filepath.Join(hasuraDir, "metadata"))
		_, jsonErr := os.Stat(filepath.Join(hasuraDir, "metadata.json"))
		if dirErr != nil && jsonErr != nil {
			return false, false, nil
		}
		if tables, err = loadRepoTables(projectDir); err != nil {
			return true, false, err
		}
	}
	_, tracked = tables[schema+"."+table]
	return true, tracked, nil
}

// VectorMetadataDrift lists what differs between live Hasura and the project's
// on-disk metadata: the YAML directory goes through HasuraDiffMetadata, the
// legacy hasura/metadata.json is compared as a whole document. Empty means no
// drift.
func VectorMetadataDrift(ctx context.Context, cfg *config.Config, projectDir string) ([]string, error) {
	if st, err := os.Stat(filepath.Join(projectDir, "hasura", "metadata")); err == nil && st.IsDir() {
		return HasuraDiffMetadata(ctx, cfg, projectDir)
	}
	file := filepath.Join(projectDir, "hasura", "metadata.json")
	disk, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("no on-disk metadata: %w", err)
	}
	live, err := HasuraExportMetadata(ctx, cfg)
	if err != nil {
		return nil, err
	}
	a, b := decodeNumbers(live), decodeNumbers(disk)
	if a == nil || b == nil {
		return []string{"metadata.json (unreadable)"}, nil
	}
	if reflect.DeepEqual(a, b) {
		return nil, nil
	}
	return []string{"metadata.json"}, nil
}

// decodeNumbers decodes JSON keeping numbers exact; nil means invalid JSON.
func decodeNumbers(b []byte) any {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil
	}
	return v
}
