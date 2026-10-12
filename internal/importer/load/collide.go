package load

// Purpose: refuse before any write when the dump would create a function or type that the
// target already owns (for example the target's own public.uuidv7()).
// Inputs: pg_restore list, declared schemas. Outputs: E519 naming the object.
// Constraints: read-only; names come from the bundle, never from user data rows.
import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
)

func TypeFunctionCollisions(ctx context.Context, db queryRower, toc string, schemas []string) error {
	declared := map[string]bool{}
	for _, s := range schemas {
		declared[s] = true
	}
	for _, line := range strings.Split(toc, "\n") {
		f := strings.Fields(line)
		if len(f) < 7 || strings.HasPrefix(f[0], ";") || !declared[f[4]] {
			continue
		}
		var exists bool
		var err error
		name := f[5]
		switch f[3] {
		case "FUNCTION", "PROCEDURE", "AGGREGATE":
			sig := strings.Join(f[5:len(f)-1], " ")
			open := strings.Index(sig, "(")
			if open < 0 {
				continue
			}
			name = sig[:open]
			err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=$1 AND p.proname=$2 AND regexp_replace(pg_get_function_identity_arguments(p.oid),'\s+',' ','g') = $3)`,
				f[4], sig[:open], strings.Join(strings.Fields(strings.TrimSuffix(sig[open+1:], ")")), " ")).Scan(&exists)
		case "TYPE", "DOMAIN":
			err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_type t JOIN pg_namespace n ON n.oid=t.typnamespace WHERE n.nspname=$1 AND t.typname=$2)`, f[4], name).Scan(&exists)
		default:
			continue
		}
		if err != nil && err != pgx.ErrNoRows {
			return errs.New("E521", "target object lookup failed")
		}
		if exists {
			return errs.New("E519", "target already has "+strings.ToLower(f[3])+" "+f[4]+"."+name+" declared by the bundle")
		}
	}
	return nil
}
