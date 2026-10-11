package database

// vector_track.go tracks a vector-search SQL function through Hasura with one
// targeted `bulk` metadata call (P7-ADOPT-07, EPIC ADOPT D7), and answers the
// on-disk questions the generator needs (table tracked, column visible, drift).
// Outputs: nil when the function is tracked with every role permission; E513
// when the table or function is missing; E514 when a --roles role has no select
// permission or any select role hides the searched column.
// Constraints: never replace_metadata or HasuraApplyMetadata. Only missing
// pieces are sent (read from a live export), so a rerun is a no-op; an
// "already-tracked"/"already-exists" race is re-planned from a fresh export.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// vtSource is the one Hasura source nSelf tracks in.
const vtSource = "default"

type vtRef struct{ Schema, Name string }

type vtRole struct{ Role string }

// vtSelect is one select permission of a tracked table; Columns is "*" or a list.
type vtSelect struct {
	Role       string `json:"role"`
	Permission struct {
		Columns   json.RawMessage `json:"columns"`
		QueryRoot json.RawMessage `json:"query_root_fields"`
		SubRoot   json.RawMessage `json:"subscription_root_fields"`
	} `json:"permission"`
}

type vtMeta struct {
	InheritedRoles []vtInherited `json:"inherited_roles"`
	Sources        []struct {
		Name   string `json:"name"`
		Tables []struct {
			Table             vtRef      `json:"table"`
			SelectPermissions []vtSelect `json:"select_permissions"`
		} `json:"tables"`
		Functions []struct {
			Function    vtRef    `json:"function"`
			Permissions []vtRole `json:"permissions"`
		} `json:"functions"`
	} `json:"sources"`
}

// TrackFunction tracks schema.name (a function returning SETOF schema.table,
// ordered by table.column) in Hasura and grants each role. See the file comment
// for the contract.
func TrackFunction(ctx context.Context, cfg *config.Config, schema, name, table, column string, roles []string) error {
	for attempt := 0; attempt < 2; attempt++ {
		live, err := HasuraExportMetadata(ctx, cfg)
		if err != nil {
			return err
		}
		ops, err := planTrack(live, schema, name, table, column, roles)
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
			return errs.Newf("E513", "function %s.%s does not exist in Postgres; run nself db migrate up --migration-dir migrations first", schema, name)
		case hasuraCode(msg, "already-tracked"), hasuraCode(msg, "already-exists"):
			continue // another writer got there first: plan again from live
		}
		return fmt.Errorf("track function %s.%s: %w", schema, name, err)
	}
	// Two races in a row: only success if live now holds everything.
	live, err := HasuraExportMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	ops, err := planTrack(live, schema, name, table, column, roles)
	if err != nil {
		return err
	}
	if len(ops) > 0 {
		return fmt.Errorf("track function %s.%s: Hasura kept answering already-tracked but live metadata still lacks %d change(s); run it again", schema, name, len(ops))
	}
	return nil
}

// hasuraCode reports whether a Hasura error message carries the given "code".
func hasuraCode(msg, code string) bool {
	return regexp.MustCompile(`"code"\s*:\s*"` + regexp.QuoteMeta(code) + `"`).MatchString(msg)
}

// columnVisible reports whether a select permission's columns value ("*" or a
// list of names) includes column. A missing value lists nothing.
func columnVisible(raw json.RawMessage, column string) bool {
	var all string
	if json.Unmarshal(raw, &all) == nil {
		return all == "*"
	}
	var list []string
	if json.Unmarshal(raw, &list) != nil {
		return false
	}
	for _, c := range list {
		if c == column {
			return true
		}
	}
	return false
}

// rootExposed reports whether a query_root_fields / subscription_root_fields
// value leaves "select" exposed. An absent value (or null) exposes everything.
func rootExposed(raw json.RawMessage) bool {
	var list []string
	if len(raw) == 0 || json.Unmarshal(raw, &list) != nil || list == nil {
		return true
	}
	for _, f := range list {
		if f == "select" {
			return true
		}
	}
	return false
}

// selectProblem says why a select permission makes the search function unsafe,
// or "" when it does not: the permission hides the ordered column, or it is the
// relationship-only pattern (query or subscription root fields without
// "select"), for which Hasura still infers the function and returns every row.
func selectProblem(columns, queryRoot, subRoot json.RawMessage, column string) string {
	switch {
	case !columnVisible(columns, column):
		return fmt.Sprintf("hides column %q", column)
	case !rootExposed(queryRoot):
		return "table select is not exposed at the root (query_root_fields)"
	case !rootExposed(subRoot):
		return "table select is not exposed at the root (subscription_root_fields)"
	}
	return ""
}

// unsafeSelectErr is the E514 for roles whose select permission makes the
// function unsafe (problems are "role: reason"). Hasura infers function
// permissions from select permissions, so every such role can call it.
func unsafeSelectErr(schema, table string, problems []string) error {
	sort.Strings(problems)
	return errs.Newf("E514", "select permission(s) on %s.%s make the search function unsafe: %s. The function orders by the vector column inside Postgres and Hasura exposes it to every role with select on the table, so these roles could learn hidden data. Fix or remove those permissions",
		schema, table, strings.Join(problems, "; "))
}

// planTrack reads the live export and returns the bulk operations still needed.
// The table must be tracked live (E513). Every --roles role must hold a select
// permission on it, and every role with a select permission must see column
// (E514). Nothing is planned when a check fails.
func planTrack(live []byte, schema, name, table, column string, roles []string) ([]map[string]any, error) {
	var m vtMeta
	if err := json.Unmarshal(live, &m); err != nil {
		return nil, fmt.Errorf("parse live metadata: %w", err)
	}
	selectRoles, tableTracked := map[string]bool{}, false
	var unsafe []string
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
					if why := selectProblem(p.Permission.Columns, p.Permission.QueryRoot, p.Permission.SubRoot, column); why != "" {
						unsafe = append(unsafe, p.Role+": "+why)
					}
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
	inherited := map[string][]string{}
	for _, ir := range m.InheritedRoles {
		inherited[ir.RoleName] = ir.RoleSet
	}
	var lacking []string
	for _, r := range roles {
		if !hasSelect(r, selectRoles, inherited, map[string]bool{}) {
			lacking = append(lacking, r)
		}
	}
	if len(lacking) > 0 {
		return nil, errs.Newf("E514", "role(s) %s have no select permission on %s.%s", strings.Join(lacking, ", "), schema, table)
	}
	if len(unsafe) > 0 {
		return nil, unsafeSelectErr(schema, table, unsafe)
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

// loadDiskTables reads the tables of the project's on-disk metadata. present
// is false when the project has no on-disk metadata. It reads the sources.yaml
// of `nself db hasura metadata export`, the Hasura CLI layouts and the legacy
// hasura/metadata.json.
func loadDiskTables(projectDir string) (present bool, tables map[string]hasuraTable, err error) {
	hasuraDir := filepath.Join(projectDir, "hasura")
	srcYAML := filepath.Join(hasuraDir, "metadata", "sources.yaml")
	if data, readErr := os.ReadFile(srcYAML); readErr == nil {
		var sources []hasuraSource
		if err := yaml.Unmarshal(data, &sources); err != nil {
			return true, nil, fmt.Errorf("parse %s: %w", srcYAML, err)
		}
		var all []hasuraTable
		for _, s := range sources {
			all = append(all, s.Tables...)
		}
		return true, tablesByKey(all), nil
	}
	_, dirErr := os.Stat(filepath.Join(hasuraDir, "metadata"))
	_, jsonErr := os.Stat(filepath.Join(hasuraDir, "metadata.json"))
	if dirErr != nil && jsonErr != nil {
		return false, nil, nil
	}
	tables, err = loadRepoTables(projectDir)
	return true, tables, err
}

// MetadataTracksTable reports whether the project's on-disk metadata lists
// schema.table. present is false when the project has no on-disk metadata.
func MetadataTracksTable(projectDir, schema, table string) (present, tracked bool, err error) {
	present, tables, err := loadDiskTables(projectDir)
	if err != nil || !present {
		return present, false, err
	}
	_, tracked = tables[schema+"."+table]
	return true, tracked, nil
}

// CheckDiskColumnVisible returns E514 when a role's on-disk select permission on
// schema.table hides column or is root-less (see selectProblem): the generator
// refuses to write a search function that would reveal rows or the column. Untracked tables or missing metadata pass
// (MetadataTracksTable reports those).
func CheckDiskColumnVisible(projectDir, schema, table, column string) error {
	present, tables, err := loadDiskTables(projectDir)
	if err != nil || !present {
		return err
	}
	var unsafe []string
	for _, p := range tables[schema+"."+table].SelectPermissions {
		raw := func(k string) json.RawMessage { b, _ := json.Marshal(p.Permission[k]); return b }
		if why := selectProblem(raw("columns"), raw("query_root_fields"), raw("subscription_root_fields"), column); why != "" {
			unsafe = append(unsafe, p.Role+": "+why)
		}
	}
	if len(unsafe) == 0 {
		return nil
	}
	return unsafeSelectErr(schema, table, unsafe)
}
