package hasurameta

// Purpose: open the source database for the nhost and hasura-cloud sources, which both take its
// URL from the --db-url-env flag. Inputs: importer input. Outputs: a connection.
// Constraints: the URL is read only from the named environment variable; errors carry no URL.
import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
)

// Connect opens the database named by the db-url-env flag.
func Connect(ctx context.Context, in importer.Input) (*pgx.Conn, error) {
	value, err := in.Secret("db-url-env")
	if err != nil {
		return nil, err
	}
	cfg, err := pgx.ParseConfig(value)
	if err != nil {
		return nil, errs.New("E517", "source connection configuration invalid")
	}
	cfg.ConnectTimeout = 10e9
	db, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, errs.New("E517", "source database unreachable")
	}
	return db, nil
}
