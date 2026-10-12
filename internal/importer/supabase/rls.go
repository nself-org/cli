package supabase

// Purpose: translate Supabase row level security policies into Hasura permissions.
// Inputs: pg_policies rows. Outputs: per-table Hasura permission lists or a reason a policy
// could not be translated. Constraints: pure; only `auth.uid() = <column>` is translated, and
// anything else is returned as untranslated, never dropped.
import (
	"regexp"
	"sort"
	"strings"
)

// hasuraRole is the role hasura-auth gives every signed-in user; Supabase `authenticated` maps to it.
const hasuraRole = "user"

// Policy is one pg_policies row. Using and Check are "" when the policy has none.
type Policy struct {
	Schema, Table, Name string
	Permissive          bool
	Roles               []string
	Cmd                 string // SELECT | INSERT | UPDATE | DELETE | ALL
	Using, Check        string
}

var (
	uidSubselect = regexp.MustCompile(`(?i)\(\s*SELECT\s+auth\.uid\(\)\s*(?:AS\s+uid\s*)?\)`)
	ident        = `([a-z_][a-z0-9_]*|"(?:[^"]|"")+")`
	uidLeft      = regexp.MustCompile(`^auth\.uid\(\)\s*=\s*` + ident + `$`)
	uidRight     = regexp.MustCompile(`^` + ident + `\s*=\s*auth\.uid\(\)$`)
)

// ownerColumn returns the column of `auth.uid() = col` (either order, redundant parentheses and
// the `(select auth.uid())` form allowed) and whether the expression has exactly that shape.
func ownerColumn(expr string) (string, bool) {
	e := uidSubselect.ReplaceAllString(strings.TrimSpace(expr), "auth.uid()")
	for strings.HasPrefix(e, "(") && strings.HasSuffix(e, ")") && balanced(e[1:len(e)-1]) {
		e = strings.TrimSpace(e[1 : len(e)-1])
	}
	for _, re := range []*regexp.Regexp{uidLeft, uidRight} {
		if m := re.FindStringSubmatch(e); m != nil {
			return unquote(m[1]), true
		}
	}
	return "", false
}
func unquote(id string) string {
	if strings.HasPrefix(id, `"`) {
		return strings.ReplaceAll(id[1:len(id)-1], `""`, `"`)
	}
	return id
}

// balanced reports whether the parentheses of s close in order, so "(a) = (b)" is not unwrapped.
func balanced(s string) bool {
	depth := 0
	for _, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

// Translation is the Hasura form of one policy: the owner column and the permission kinds it covers.
type Translation struct {
	Column string
	Kinds  []string // select | insert | update | delete
}

// Translate maps one policy. columns is the set of the table's columns. A non-empty reason means
// the policy is untranslated and the Translation is unusable.
func Translate(p Policy, columns map[string]bool) (Translation, string) {
	if !p.Permissive {
		return Translation{}, "restrictive policy has no Hasura permission equivalent"
	}
	for _, r := range p.Roles {
		if r != "public" && r != "authenticated" {
			return Translation{}, "policy applies to role " + r + ", which has no Hasura role here"
		}
	}
	var kinds []string
	switch strings.ToUpper(p.Cmd) {
	case "SELECT":
		kinds = []string{"select"}
	case "INSERT":
		kinds = []string{"insert"}
	case "UPDATE":
		kinds = []string{"update"}
	case "DELETE":
		kinds = []string{"delete"}
	case "ALL":
		kinds = []string{"select", "insert", "update", "delete"}
	default:
		return Translation{}, "unknown policy command " + p.Cmd
	}
	read, write := p.Using, p.Check
	if strings.EqualFold(p.Cmd, "INSERT") {
		read, write = "", p.Check
	}
	need := read
	if need == "" {
		need = write
	}
	col, ok := ownerColumn(need)
	if !ok {
		return Translation{}, "expression is not `auth.uid() = <column>`"
	}
	// The other expression, when present, must name the same column.
	for _, other := range []string{read, write} {
		if other == "" {
			continue
		}
		if c, ok := ownerColumn(other); !ok || c != col {
			return Translation{}, "USING and WITH CHECK differ or are not `auth.uid() = <column>`"
		}
	}
	if !columns[col] {
		return Translation{}, "column " + col + " is not a column of the table"
	}
	return Translation{Column: col, Kinds: kinds}, ""
}

// filterFor is the Hasura boolean expression for an owner column.
func filterFor(col string) map[string]any {
	return map[string]any{col: map[string]any{"_eq": "X-Hasura-User-Id"}}
}

// combine ORs several owner-column filters (permissive policies of one command are ORed by
// Postgres) in a stable order.
func combine(cols []string) map[string]any {
	sort.Strings(cols)
	uniq := cols[:0:0]
	for i, c := range cols {
		if i == 0 || c != cols[i-1] {
			uniq = append(uniq, c)
		}
	}
	if len(uniq) == 1 {
		return filterFor(uniq[0])
	}
	or := make([]any, 0, len(uniq))
	for _, c := range uniq {
		or = append(or, filterFor(c))
	}
	return map[string]any{"_or": or}
}
