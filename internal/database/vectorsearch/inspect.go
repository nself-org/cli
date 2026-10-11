package vectorsearch

// inspect.go reads the column type from the project's Postgres container and
// turns it into a dimension. Only `vector(N)` is searchable; anything else
// (text, halfvec, sparsevec, a missing column or table) is E513 and nothing is
// written.

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/errs"
)

// Exec runs a command in a running container and returns stdout and stderr; it
// has the signature of docker.ExecCapture so tests inject a fake.
type Exec func(ctx context.Context, container string, cmd []string) (stdout, stderr string, err error)

// PG names the Postgres container and the role and database psql uses inside
// it. The role is the container's own superuser: this is the CLI's admin path
// (like db migrate), not a credential handed to apps.
type PG struct {
	Container string
	User      string
	DB        string
}

var vectorTypeRe = regexp.MustCompile(`^vector\(([0-9]+)\)$`)

// typeQuery builds the format_type query from validated identifiers only. The
// quoted pair is a regclass text literal; to_regclass returns NULL for a
// missing table, so a missing table or column gives empty output (E513).
func typeQuery(s Spec) (string, error) {
	schema, err := database.SanitizeIdentifier(s.Schema)
	if err != nil {
		return "", err
	}
	table, err := database.SanitizeIdentifier(s.Table)
	if err != nil {
		return "", err
	}
	if _, err := database.SanitizeIdentifier(s.Column); err != nil {
		return "", err
	}
	return "SELECT format_type(a.atttypid, a.atttypmod) FROM pg_attribute a" +
		" WHERE a.attrelid = to_regclass('" + schema + "." + table + "')" +
		" AND a.attname = '" + s.Column + "' AND a.attnum > 0 AND NOT a.attisdropped", nil
}

// psqlArgv is the psql command run in the container: tuples only, unaligned,
// no psqlrc, stop on error.
func psqlArgv(pg PG, query string) []string {
	return []string{"psql", "-X", "-U", pg.User, "-d", pg.DB, "-v", "ON_ERROR_STOP=1", "-At", "-c", query}
}

// ParseDim extracts N from a format_type result of the form vector(N).
func ParseDim(formatType string) (int, error) {
	m := vectorTypeRe.FindStringSubmatch(strings.TrimSpace(formatType))
	if m == nil {
		return 0, fmt.Errorf("column type is %q, not vector(n)", strings.TrimSpace(formatType))
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 || n > maxDim {
		return 0, fmt.Errorf("vector dimension %q is out of range 1..%d", m[1], maxDim)
	}
	return n, nil
}

// InspectColumn returns the dimension of the vector(n) column named by s.
// A non-vector column, a missing column or table, or a column without a fixed
// dimension is E513.
func InspectColumn(ctx context.Context, run Exec, pg PG, s Spec) (int, error) {
	q, err := typeQuery(s)
	if err != nil {
		return 0, err
	}
	stdout, stderr, err := run(ctx, pg.Container, psqlArgv(pg, q))
	if err != nil {
		return 0, fmt.Errorf("read column type of %s.%s.%s (is the stack running? try nself start): %w: %s",
			s.Schema, s.Table, s.Column, err, strings.TrimSpace(stderr))
	}
	out := strings.TrimSpace(stdout)
	if out == "" {
		return 0, errs.Newf("E513", "column %s.%s.%s does not exist (or the table is missing)", s.Schema, s.Table, s.Column)
	}
	dim, err := ParseDim(out)
	if err != nil {
		return 0, errs.Newf("E513", "%s.%s.%s: %v", s.Schema, s.Table, s.Column, err)
	}
	return dim, nil
}

// shapeQuery asks Postgres whether the function is the generated, inlinable
// shape: LANGUAGE sql, STABLE, not SECURITY DEFINER, not STRICT, no SET
// clause, set returning, returning the table's own row type, and with the
// generated body (whitespace-normalised, so a hand edit that adds a LIMIT or
// a WHERE inside the function is caught: D-0369). A STRICT function is not
// inlined, so Hasura's limit and the row filter would apply after the
// function ran. It is built only from validated identifiers; empty output
// means the function (or table) is missing.
func shapeQuery(s Spec) (string, error) {
	schema, err := database.SanitizeIdentifier(s.Schema)
	if err != nil {
		return "", err
	}
	table, err := database.SanitizeIdentifier(s.Table)
	if err != nil {
		return "", err
	}
	fn, err := database.SanitizeIdentifier(s.FunctionName())
	if err != nil {
		return "", err
	}
	column, err := database.SanitizeIdentifier(s.Column)
	if err != nil {
		return "", err
	}
	// The body holds quoted identifiers (no single quote passes
	// SanitizeIdentifier) and a fixed operator, so it is a safe literal.
	body := functionBody(schema, table, column, s.Operator())
	return "SELECT p.prolang = (SELECT oid FROM pg_language WHERE lanname = 'sql')" +
		" AND p.provolatile = 's' AND NOT p.prosecdef AND NOT p.proisstrict AND p.proconfig IS NULL" +
		" AND p.proretset AND p.prorettype = c.reltype" +
		" AND regexp_replace(btrim(p.prosrc, E' \\t\\r\\n;'), '\\s+', ' ', 'g') = '" + body + "'" +
		" FROM pg_proc p JOIN pg_class c ON c.oid = to_regclass('" + schema + "." + table + "')" +
		" WHERE p.oid = to_regprocedure('" + schema + "." + fn + "(vector)')", nil
}

// CheckFunctionShape returns E513 when the function is missing from Postgres or
// exists in a shape other than the one this command generates (for example one
// hand-written with SECURITY DEFINER, which would bypass the row filter).
func CheckFunctionShape(ctx context.Context, run Exec, pg PG, s Spec) error {
	q, err := shapeQuery(s)
	if err != nil {
		return err
	}
	stdout, stderr, err := run(ctx, pg.Container, psqlArgv(pg, q))
	if err != nil {
		return fmt.Errorf("check function %s.%s: %w: %s", s.Schema, s.FunctionName(), err, strings.TrimSpace(stderr))
	}
	switch strings.TrimSpace(stdout) {
	case "t":
		return nil
	case "":
		return errs.Newf("E513", "function %s.%s does not exist in Postgres; run nself db migrate up --migration-dir migrations first", s.Schema, s.FunctionName())
	}
	return errs.Newf("E513", "function %s.%s exists but is not the generated inlinable shape (LANGUAGE sql, STABLE, SECURITY INVOKER, not STRICT, no SET, SETOF %s.%s, the generated body with no LIMIT or filter); regenerate it with this command", s.Schema, s.FunctionName(), s.Schema, s.Table)
}
