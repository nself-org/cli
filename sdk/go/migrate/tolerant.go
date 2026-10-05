package migrate

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// deferrable lists the SQLSTATEs that mean "a dependency does not exist yet":
// 42P01 undefined_table, 42703 undefined_column, 42704 undefined_object.
var deferrable = map[string]bool{"42P01": true, "42703": true, "42704": true}

func isDeferrable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && deferrable[pgErr.Code]
}

// runRounds applies pending files in order. In tolerant mode a file that
// fails with a deferrable error is left unrecorded and retried in the next
// round; rounds repeat until nothing is pending or a round makes no progress.
// It returns the files still pending. Strict mode (tolerant false) stops at
// the first error, which names the file.
func (p *plan) runRounds(ctx context.Context, conn *pgx.Conn, pending []file, tolerant bool, res *Result) ([]file, error) {
	for len(pending) > 0 {
		progressed := false
		var still []file
		for _, f := range pending {
			err := p.applyFile(ctx, conn, f)
			switch {
			case err == nil:
				res.Applied = append(res.Applied, f.name)
				progressed = true
			case tolerant && isDeferrable(err):
				still = append(still, f)
			default:
				return pending, &FileError{File: f.name, Err: err}
			}
		}
		pending = still
		if !progressed {
			break
		}
	}
	return pending, nil
}
