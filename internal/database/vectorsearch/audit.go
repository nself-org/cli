package vectorsearch

// audit.go re-runs the --track permission checks against live metadata for
// every vector-search function already tracked (D-0339). Hasura re-infers
// function access on every metadata change, so a select permission added or
// narrowed after --track can expose the function to a role that hides the
// vector column or the table's root field. The doctor calls AuditLive.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/database"
)

// Finding is one tracked search function that live metadata makes unsafe.
// Problems are "role: reason", sorted.
type Finding struct {
	Function string
	Table    string
	Column   string
	Problems []string
}

type auditMeta struct {
	InheritedRoles []struct {
		RoleName string   `json:"role_name"`
		RoleSet  []string `json:"role_set"`
	} `json:"inherited_roles"`
	Sources []struct {
		Name   string `json:"name"`
		Tables []struct {
			Table struct {
				Schema string `json:"schema"`
				Name   string `json:"name"`
			} `json:"table"`
			SelectPermissions []struct {
				Role       string `json:"role"`
				Permission struct {
					Columns   json.RawMessage `json:"columns"`
					QueryRoot json.RawMessage `json:"query_root_fields"`
					SubRoot   json.RawMessage `json:"subscription_root_fields"`
				} `json:"permission"`
			} `json:"select_permissions"`
		} `json:"tables"`
		Functions []struct {
			Function struct {
				Schema string `json:"schema"`
				Name   string `json:"name"`
			} `json:"function"`
			Permissions []struct {
				Role string `json:"role"`
			} `json:"permissions"`
		} `json:"functions"`
	} `json:"sources"`
}

// AuditLive parses an export_metadata document and returns the tracked
// functions of the default source named <table>_nearest_<column> (the name
// FunctionName gives) over a tracked table of the same schema whose select
// permissions fail the --track checks. The second value is the number of
// search functions examined. The match is by name only: the metadata does
// not carry the function's return type, and the _nearest_ name is the one
// the generator owns, so a hand-written function with such a name is held to
// the same rule. A name that matches more than one table (a table itself
// named x_nearest_y) is checked against each: failing closed is the safe side
// for an ordering oracle.
//
// inferred is Hasura's HASURA_GRAPHQL_INFER_FUNCTION_PERMISSIONS (true by
// default): every role with select on the table can call the function. When
// it is false only the roles with an explicit function permission can, so
// only their select permissions are checked; an inherited role among them
// stands for every role of its role set (its select comes from them).
func AuditLive(live []byte, inferred bool) ([]Finding, int, error) {
	var m auditMeta
	if err := json.Unmarshal(live, &m); err != nil {
		return nil, 0, fmt.Errorf("parse live metadata: %w", err)
	}
	roleSets := map[string][]string{}
	for _, ir := range m.InheritedRoles {
		roleSets[ir.RoleName] = ir.RoleSet
	}
	var out []Finding
	seen := 0
	for _, src := range m.Sources {
		if src.Name != defaultSource {
			continue
		}
		for _, f := range src.Functions {
			matched := false
			for _, t := range src.Tables {
				column, ok := strings.CutPrefix(f.Function.Name, t.Table.Name+"_nearest_")
				if !ok || column == "" || t.Table.Schema != f.Function.Schema {
					continue
				}
				matched = true
				callers := map[string]bool{}
				for _, p := range f.Permissions {
					addCaller(callers, roleSets, p.Role)
				}
				var problems []string
				for _, p := range t.SelectPermissions {
					if !inferred && !callers[p.Role] {
						continue
					}
					if why := database.VectorSelectProblem(p.Permission.Columns, p.Permission.QueryRoot, p.Permission.SubRoot, column); why != "" {
						problems = append(problems, p.Role+": "+why)
					}
				}
				if len(problems) > 0 {
					sort.Strings(problems)
					out = append(out, Finding{Function: f.Function.Schema + "." + f.Function.Name,
						Table: t.Table.Schema + "." + t.Table.Name, Column: column, Problems: problems})
				}
			}
			if matched {
				seen++
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Function != out[j].Function {
			return out[i].Function < out[j].Function
		}
		return out[i].Table < out[j].Table
	})
	return out, seen, nil
}

// addCaller marks role and, for an inherited role, every role of its role set
// (nested; the map doubles as the cycle guard).
func addCaller(callers map[string]bool, roleSets map[string][]string, role string) {
	if callers[role] {
		return
	}
	callers[role] = true
	for _, member := range roleSets[role] {
		addCaller(callers, roleSets, member)
	}
}
