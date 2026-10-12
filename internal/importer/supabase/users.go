package supabase

// Purpose: export Supabase auth users into the portable users member.
// Inputs: the snapshot transaction. Outputs: auth/users.jsonl and independent user counts.
// Constraints: the bcrypt hash (gotrue encrypted_password) belongs only to the private bundle.
import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/importer/postgres"
	"github.com/nself-org/cli/internal/portable"
)

// usersSQL maps gotrue's auth.users to the portable fields. Empty strings become NULL so a
// phone-only or OAuth user keeps a NULL email and no hash; banned_until in the future disables.
// Every user gets the allowed role `user`, which the translated permissions use (an empty role
// list would leave the user with no role Hasura accepts).
const usersSQL = `SELECT json_build_object('id',u.id,'email',nullif(u.email,''),'email_verified',(u.email_confirmed_at IS NOT NULL),'phone',nullif(u.phone,''),'display_name',coalesce(u.raw_user_meta_data->>'full_name',u.raw_user_meta_data->>'name'),'created_at',u.created_at,'disabled',(u.banned_until IS NOT NULL AND u.banned_until > now()),'password_hash',nullif(u.encrypted_password,''),'metadata',coalesce(u.raw_user_meta_data,'{}'::jsonb),'roles','["user"]'::json) FROM auth.users u ORDER BY u.id`

func exportUsers(ctx context.Context, tx pgx.Tx, w *portable.Writer, m *portable.Manifest) error {
	return postgres.WriteUsers(ctx, tx, w, m, usersSQL)
}
