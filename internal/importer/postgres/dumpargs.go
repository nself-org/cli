package postgres

// Purpose: pg_dump arguments and source checks that must hold before the dump runs.
// Inputs: snapshot id, schema names, snapshot transaction. Outputs: argv or a refusal.
// Constraints: patterns are quoted so mixed-case names match exactly; no secrets here.
import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
)

// quotePattern makes a pg_dump pattern that matches exactly one object name: double quotes
// keep case and make '*' and '?' literal, and an inner double quote is doubled.
func quotePattern(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func dumpArgs(snapshot string, schemas []string) ([]string, error) {
	if len(schemas) == 0 {
		return nil, errs.New("E520", "source has no importable schemas")
	}
	args := []string{"pg_dump", "--snapshot=" + snapshot, "--no-owner", "--no-privileges"}
	for _, schema := range schemas {
		if _, err := database.SanitizeIdentifier(schema); err != nil {
			return nil, errs.New("E520", "unsupported source schema")
		}
		args = append(args, "--schema="+quotePattern(schema))
	}
	return append(args, "--exclude-table=*.np_*"), nil
}

var leadingDigits = regexp.MustCompile(`^[0-9]+`)

// majorOf reads the major from a server_version text such as "16.4" or "17beta1".
func majorOf(version string) int {
	n, _ := strconv.Atoi(leadingDigits.FindString(version))
	return n
}

// refusePartitioned stops before any dump when a declared schema holds a partitioned table or
// a partition: restore cannot yet bind partition entries to the manifest, so the import would
// fail at load. Failing here names the table and writes nothing.
func refusePartitioned(ctx context.Context, tx pgx.Tx, schemas []string) error {
	var name string
	err := tx.QueryRow(ctx, `SELECT n.nspname||'.'||c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname = ANY($1) AND (c.relkind='p' OR c.relispartition) ORDER BY 1 LIMIT 1`, schemas).Scan(&name)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return errs.New("E520", "source partition lookup failed")
	}
	return errs.New("E520", importer.PartitionedPrefix+name)
}
