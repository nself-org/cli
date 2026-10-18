package doctor

// Purpose: ISO-01..ISO-04 — the ADR 0004 convention wall, checked against live
// Hasura permissions. Pure functions over a parsed metadata model; no I/O.
// Inputs: tracked tables with their per-role permissions (isoTable) and, per
// table, which of the two isolation columns exist (isoColumns).
// Outputs: sorted isoFinding values, each naming table, role and operation.
// Constraints: permission values stay decoded JSON (any) because Hasura filters
// are open-ended boolean expression trees. A filter counts as pinning a column only when it GUARANTEES
// `column = session variable` for every row it admits (an _and branch, every
// _or branch, an implicit AND). _not, _exists, an empty filter and a missing
// permission never pin. Hasura's admin role is skipped (and nself_admin on np_*
// tables); every other role is checked (fail-closed). Session variable names compare
// case-insensitively, column names exactly.

import (
	"sort"
	"strings"
)

const (
	isoColSAID   = "source_account_id"
	isoColTID    = "tenant_id"
	isoClaimSAID = "X-Hasura-Source-Account-Id"
	isoClaimTID  = "X-Hasura-Tenant-Id"
)

// isoOps is the check order of the four Hasura permission kinds.
var isoOps = []string{"select", "insert", "update", "delete"}

// skipRole reports a role the rules leave alone on t: Hasura's own admin, and
// nself_admin on np_* tables only (plugin metadata grants it filter {} by design;
// on an app table it is checked like any other role).
func skipRole(t isoTable, role string) bool {
	return role == "admin" || (role == "nself_admin" && t.np())
}

// isoSkipSchemas hold tables nSelf and its auth/storage services own (ISO-01/02).
var isoSkipSchemas = map[string]bool{"auth": true, "storage": true, "hdb_catalog": true}

// isoPerm is one role's permission for one operation. Fields are the decoded
// JSON/YAML values (map[string]any, []any, string); nil when absent.
type isoPerm struct {
	Filter, Check, Set, Columns any
}

// isoTable is a tracked table with Perms[role][op].
type isoTable struct {
	Schema, Name string
	Perms        map[string]map[string]isoPerm
}

func (t isoTable) id() string { return t.Schema + "." + t.Name }

// np reports a plugin table (ISO-01/02 leave those to PERM-HASURA-01).
func (t isoTable) np() bool { return strings.HasPrefix(t.Name, "np_") }

// appTable reports a table ISO-01/ISO-02 apply to.
func (t isoTable) appTable() bool { return !t.np() && !isoSkipSchemas[t.Schema] }

// isoColumns maps "schema.table" to the isolation columns it has.
type isoColumns map[string]map[string]bool

// isoFinding is one rule violation. Fail=false is a warning (ISO-01 only).
type isoFinding struct {
	Rule, Table, Role, Op, Detail string
	Fail                          bool
}

// isolationRules runs ISO-01..ISO-04. strict turns the ISO-01 warning into a fail.
func isolationRules(tables []isoTable, cols isoColumns, strict bool) []isoFinding {
	var out []isoFinding
	multiApp := false
	for _, t := range tables {
		if t.appTable() && cols[t.id()][isoColSAID] {
			multiApp = true
		}
	}
	for _, t := range tables {
		has := cols[t.id()]
		if multiApp && t.appTable() && !has[isoColSAID] {
			out = append(out, isoFinding{Rule: "ISO-01", Table: t.id(), Fail: strict,
				Detail: "tracked app table has no " + isoColSAID + " while other tables do (multi-app mode)"})
		}
		if has[isoColSAID] && t.appTable() {
			out = append(out, pinFindings("ISO-02", t, isoColSAID, isoClaimSAID)...)
		}
		if has[isoColTID] {
			out = append(out, pinFindings("ISO-03", t, isoColTID, isoClaimTID)...)
		}
		out = append(out, interchangeFindings(t)...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return a.Rule+"\x00"+a.Table+"\x00"+a.Role+"\x00"+a.Op < b.Rule+"\x00"+b.Table+"\x00"+b.Role+"\x00"+b.Op
	})
	return out
}

// sortedRoles lists the non-admin roles of t.
func sortedRoles(t isoTable) []string {
	var roles []string
	for r := range t.Perms {
		if !skipRole(t, r) {
			roles = append(roles, r)
		}
	}
	sort.Strings(roles)
	return roles
}

// pinFindings checks that every non-admin role's permissions on t pin col to claim.
func pinFindings(rule string, t isoTable, col, claim string) []isoFinding {
	var out []isoFinding
	want := col + " = " + claim
	for _, role := range sortedRoles(t) {
		for _, op := range isoOps {
			p, ok := t.Perms[role][op]
			if !ok {
				continue
			}
			var detail string
			switch op {
			case "insert":
				if !pins(p.Check, col, claim) && !setPins(p.Set, col, claim) {
					detail = "insert check/set does not pin " + want
				}
			case "update":
				if !pins(p.Filter, col, claim) {
					detail = "update filter does not pin " + want
				} else if !pins(p.Check, col, claim) && !setPins(p.Set, col, claim) && (writable(p.Columns, col) || hasKey(p.Set, col)) {
					detail = "update check/set does not pin " + want + " and the column can be written (columns or preset)"
				}
			default:
				if !pins(p.Filter, col, claim) {
					detail = op + " filter does not pin " + want
				}
			}
			if detail != "" {
				out = append(out, isoFinding{Rule: rule, Table: t.id(), Role: role, Op: op, Detail: detail, Fail: true})
			}
		}
	}
	return out
}

// interchangeFindings is ISO-04: the two columns compared to each other's variable.
func interchangeFindings(t isoTable) []isoFinding {
	var out []isoFinding
	for _, role := range sortedRoles(t) {
		for _, op := range isoOps {
			p, ok := t.Perms[role][op]
			if ok && (interchange(p.Filter) || interchange(p.Check) || interchange(p.Set)) {
				out = append(out, isoFinding{Rule: "ISO-04", Table: t.id(), Role: role, Op: op, Fail: true,
					Detail: isoColSAID + " compared to " + isoClaimTID + " or " + isoColTID + " to " + isoClaimSAID + " (ADR 0004 forbids the interchange)"})
			}
		}
	}
	return out
}

// pins reports whether filter guarantees col = claim for every row it admits.
func pins(filter any, col, claim string) bool {
	m, ok := filter.(map[string]any)
	if !ok {
		return false
	}
	for k, v := range m {
		switch strings.ToLower(k) {
		case "_and":
			for _, item := range asList(v) {
				if pins(item, col, claim) {
					return true
				}
			}
		case "_or":
			items := asList(v)
			all := len(items) > 0
			for _, item := range items {
				all = all && pins(item, col, claim)
			}
			if all {
				return true
			}
		default:
			if k == col && opEq(v, claim) {
				return true
			}
		}
	}
	return false
}

// opEq reports whether an operator object has _eq set to the session variable.
func opEq(v any, claim string) bool {
	ops, ok := v.(map[string]any)
	if !ok {
		return false
	}
	s, ok := ops["_eq"].(string)
	return ok && strings.EqualFold(s, claim)
}

// setPins reports whether an insert preset assigns col the session variable.
func setPins(set any, col, claim string) bool {
	m, ok := set.(map[string]any)
	if !ok {
		return false
	}
	s, ok := m[col].(string)
	return ok && strings.EqualFold(s, claim)
}

// hasKey reports whether a preset assigns col at all (to anything).
func hasKey(set any, col string) bool {
	m, _ := set.(map[string]any)
	_, ok := m[col]
	return ok
}

// writable reports whether an update permission's column list includes col.
// A missing or unreadable list counts as writable (fail-closed).
func writable(columns any, col string) bool {
	if columns == nil {
		return true
	}
	if s, ok := columns.(string); ok {
		return s == "*"
	}
	for _, c := range asList(columns) {
		if s, ok := c.(string); ok && s == col {
			return true
		}
	}
	return false
}

// interchange walks the whole tree (also _or, _not, _exists) for isolation
// column keys whose value mentions the other column's session variable.
func interchange(node any) bool {
	switch n := node.(type) {
	case map[string]any:
		for k, v := range n {
			if (k == isoColSAID && mentions(v, isoClaimTID)) || (k == isoColTID && mentions(v, isoClaimSAID)) {
				return true
			}
			if interchange(v) {
				return true
			}
		}
	case []any:
		for _, v := range n {
			if interchange(v) {
				return true
			}
		}
	}
	return false
}

// mentions reports whether any string inside node equals claim (case-insensitive).
func mentions(node any, claim string) bool {
	switch n := node.(type) {
	case string:
		return strings.EqualFold(n, claim)
	case map[string]any:
		for _, v := range n {
			if mentions(v, claim) {
				return true
			}
		}
	case []any:
		for _, v := range n {
			if mentions(v, claim) {
				return true
			}
		}
	}
	return false
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}
