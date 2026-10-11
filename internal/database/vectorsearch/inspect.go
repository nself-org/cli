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
