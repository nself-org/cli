package nhost

// Purpose: find the hasura-auth data the importer does not carry, so a pass never hides a security
// downgrade. Users come across with their bcrypt hash only; a second factor, an OAuth identity or
// a security key stays behind, and the user would sign in with the password alone or not at all.
// Inputs: a source connection. Outputs: counts. Constraints: read-only; tables and columns that an
// older hasura-auth lacks count as zero.
import (
	"context"

	"github.com/nself-org/cli/internal/errs"
)

type authExtras struct{ mfa, providers, keys int64 }

func (a authExtras) nonzero() bool { return a.mfa > 0 || a.providers > 0 || a.keys > 0 }

func authExtrasIn(ctx context.Context, q rower) (authExtras, error) {
	var out authExtras
	var has bool
	if q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='auth' AND table_name='users' AND column_name='active_mfa_type')`).Scan(&has) != nil {
		return out, errs.New("E517", msgStorageLookup)
	}
	if has && q.QueryRow(ctx, `SELECT count(*) FROM auth.users WHERE active_mfa_type IS NOT NULL AND active_mfa_type <> ''`).Scan(&out.mfa) != nil {
		return out, errs.New("E517", msgStorageLookup)
	}
	for table, dest := range map[string]*int64{"auth.user_providers": &out.providers, "auth.user_security_keys": &out.keys} {
		var present bool
		if q.QueryRow(ctx, `SELECT to_regclass('`+table+`') IS NOT NULL`).Scan(&present) != nil {
			return out, errs.New("E517", msgStorageLookup)
		}
		if present && q.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(dest) != nil {
			return out, errs.New("E517", msgStorageLookup)
		}
	}
	return out, nil
}
