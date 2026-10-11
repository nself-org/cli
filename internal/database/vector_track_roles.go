package database

// vector_track_roles.go: role resolution for vector-search tracking. An
// inherited role holds select through its role set (D-0369), and the doctor's
// live re-check of tracked search functions (D-0339) reuses the select
// safety rule through VectorSelectProblem.

import "encoding/json"

// vtInherited is one Hasura inherited role: it holds the union of the
// permissions of the roles in its role set (which may be inherited roles too).
type vtInherited struct {
	RoleName string   `json:"role_name"`
	RoleSet  []string `json:"role_set"`
}

// hasSelect reports whether role holds a select permission on the table: its
// own, or (for an inherited role, D-0369) one of a role in its role set,
// resolved through nested inherited roles. seen stops a cycle.
func hasSelect(role string, selectRoles map[string]bool, inherited map[string][]string, seen map[string]bool) bool {
	if selectRoles[role] {
		return true
	}
	if seen[role] {
		return false
	}
	seen[role] = true
	for _, member := range inherited[role] {
		if hasSelect(member, selectRoles, inherited, seen) {
			return true
		}
	}
	return false
}

// VectorSelectProblem is selectProblem for callers outside this package (the
// doctor's live re-check of tracked search functions, D-0339): why a select
// permission (its columns, query_root_fields and subscription_root_fields
// values) makes a search function ordered by column unsafe, or "".
func VectorSelectProblem(columns, queryRoot, subRoot json.RawMessage, column string) string {
	return selectProblem(columns, queryRoot, subRoot, column)
}
