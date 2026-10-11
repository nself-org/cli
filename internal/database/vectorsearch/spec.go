// Package vectorsearch generates the pgvector k-NN SQL function for one table
// column and mirrors its Hasura tracking entry (P7-ADOPT-07, EPIC ADOPT D7).
//
// Purpose: apps reach similarity search through Hasura GraphQL only (ADR 0003).
// A STABLE SQL function that returns SETOF the source table keeps the table's
// select filter on every row, so no new access path, role or grant is needed.
// Inputs: a Spec (schema.table.column, distance, roles) and the project dir.
// Outputs: a migration file, an optional live tracking call and an optional
// on-disk metadata mirror.
// Constraints: identifiers pass database.SanitizeIdentifier; the generated SQL
// is built with text/template over validated values only; no Postgres role,
// grant or connection string is ever created; metadata is never replaced.
package vectorsearch

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/database"
)

// Distance names the pgvector distance used to order results.
type Distance string

// The supported distances and their pgvector operators (see Operator).
const (
	Cosine Distance = "cosine"
	L2     Distance = "l2"
	IP     Distance = "ip"
)

// DefaultRole is the Hasura role granted when --roles is not given.
const DefaultRole = "user"

// maxFuncNameLen is the Postgres identifier limit; a longer function name would
// be truncated silently and then not match what Hasura is asked to track.
const maxFuncNameLen = 63

// maxDim is the largest dimension pgvector stores in a vector column.
const maxDim = 16000

var roleRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// Spec describes one generated function.
type Spec struct {
	Schema   string
	Table    string
	Column   string
	Dim      int      // read from the column type by Inspect
	Distance Distance // "" means cosine
	Roles    []string // empty means [DefaultRole]
}

// ParseTarget splits "<schema>.<table>.<column>" into a Spec. Each part must be
// a plain identifier; the full check is Validate.
func ParseTarget(target string) (Spec, error) {
	parts := strings.Split(target, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return Spec{}, fmt.Errorf("target %q must be <schema>.<table>.<column>, for example public.docs.embedding", target)
	}
	return Spec{Schema: parts[0], Table: parts[1], Column: parts[2]}, nil
}

// Validate checks every field, fills the defaults (cosine, role user) and
// removes duplicate roles. Dim is checked only when set (Inspect sets it).
func (s *Spec) Validate() error {
	for _, id := range []struct{ what, v string }{{"schema", s.Schema}, {"table", s.Table}, {"column", s.Column}} {
		if _, err := database.SanitizeIdentifier(id.v); err != nil {
			return fmt.Errorf("invalid %s: %w", id.what, err)
		}
	}
	// A column named like the function parameter would shadow it inside the
	// SQL body (SQL functions prefer column names) and silently order by itself.
	if strings.EqualFold(s.Column, "query") {
		return fmt.Errorf("column %q cannot be searched: it collides with the function parameter name query; rename the column", s.Column)
	}
	if n := len(s.FunctionName()); n > maxFuncNameLen {
		return fmt.Errorf("function name %q is %d bytes, over the Postgres limit of %d; use a shorter table or column name", s.FunctionName(), n, maxFuncNameLen)
	}
	switch s.Distance {
	case "":
		s.Distance = Cosine
	case Cosine, L2, IP:
	default:
		return fmt.Errorf("invalid --distance %q: use cosine, l2 or ip", s.Distance)
	}
	roles, err := normalizeRoles(s.Roles)
	if err != nil {
		return err
	}
	s.Roles = roles
	if s.Dim != 0 && (s.Dim < 1 || s.Dim > maxDim) {
		return fmt.Errorf("invalid vector dimension %d: must be 1..%d", s.Dim, maxDim)
	}
	return nil
}

// normalizeRoles validates roles, drops duplicates and defaults to [user].
func normalizeRoles(in []string) ([]string, error) {
	if len(in) == 0 {
		return []string{DefaultRole}, nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, r := range in {
		r = strings.TrimSpace(r)
		if !roleRe.MatchString(r) {
			return nil, fmt.Errorf("invalid role %q: must match ^[a-z][a-z0-9_]{0,62}$", r)
		}
		if r == "admin" {
			return nil, fmt.Errorf("role admin already sees every function; name only the roles that need a permission")
		}
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out, nil
}

// FunctionName is the unqualified function name: <table>_nearest_<column>.
func (s Spec) FunctionName() string { return s.Table + "_nearest_" + s.Column }

// Operator returns the pgvector operator for the distance: cosine <=>, l2 <->,
// ip <#>.
func (s Spec) Operator() string {
	switch s.Distance {
	case L2:
		return "<->"
	case IP:
		return "<#>"
	default:
		return "<=>"
	}
}
