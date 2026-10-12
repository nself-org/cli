package load

// Purpose: every read-only check that must pass before the first write to the target.
// Inputs: target connection, verified bundle, object store. Outputs: users and metadata tables.
// Constraints: nothing here changes the target; refusals name no user data.
import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer/verify"
	"github.com/nself-org/cli/internal/objstore"
	"github.com/nself-org/cli/internal/portable"
)

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func preflight(ctx context.Context, cfg *config.Config, db *pgx.Conn, r *portable.Reader, store *objstore.Client, opts Options) ([]verify.User, []map[string]any, error) {
	m := r.Manifest()
	if err := Empty(ctx, db, m.DB.Schemas, store != nil); err != nil {
		return nil, nil, err
	}
	if err := StoragePreflight(m, store, opts.SkipStorage); err != nil {
		return nil, nil, err
	}
	if err := KeysFree(ctx, m, store, opts.SkipStorage); err != nil {
		return nil, nil, err
	}
	users, err := verify.ReadUsers(r)
	if err != nil {
		return nil, nil, errs.New("E521", "bundle users invalid")
	}
	if err = AuthPreflight(ctx, db, len(users)); err != nil {
		return nil, nil, err
	}
	metadata, err := verify.ReadMetadata(r)
	if err != nil {
		return nil, nil, errs.New("E521", "bundle metadata invalid")
	}
	tables := make([]map[string]any, 0, len(m.DB.Tables))
	for _, t := range m.DB.Tables {
		tables = append(tables, metadata[t.Schema+"."+t.Name])
	}
	// A conflict between the bundle's metadata and the metadata already live for the same tables
	// is decidable now, so refuse before any write. The plan is rebuilt after the restore because
	// only then do the imported tables exist; that late check stays as the guard for anything that
	// can only be found once the tables are there.
	if _, err = database.BuildAdditivePlan(ctx, cfg, tables); err != nil {
		return nil, nil, errs.New("E521", err.Error()+"; nothing was written")
	}
	return users, tables, nil
}

// AuthPreflight refuses a bundle with users when the target already holds auth users: the load
// would add to them and verification (which counts every target user) could never reach zero.
func AuthPreflight(ctx context.Context, db queryRower, bundleUsers int) error {
	if bundleUsers == 0 {
		return nil
	}
	var present bool
	if db.QueryRow(ctx, `SELECT to_regclass('auth.users') IS NOT NULL`).Scan(&present) != nil {
		return errs.New("E521", "target auth lookup failed")
	}
	if !present {
		return errs.New("E521", "target has no auth.users table; enable AUTH_ENABLED before importing users")
	}
	var used bool
	if db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth.users)`).Scan(&used) != nil {
		return errs.New("E521", "target auth lookup failed")
	}
	if used {
		return errs.New("E519", "target auth.users is not empty")
	}
	return nil
}
