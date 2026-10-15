package supabase

// Purpose: export Supabase auth users into the portable users member.
// Inputs: the snapshot transaction. Outputs: auth/users.jsonl and independent user counts.
// Constraints: the bcrypt hash (gotrue encrypted_password) belongs only to the private bundle.
import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer/postgres"
	"github.com/nself-org/cli/internal/portable"
)

// usersSQL maps gotrue's auth.users to the portable fields. Empty strings become NULL so a
// phone-only or OAuth user keeps a NULL email and no hash; banned_until in the future disables.
// Every user gets the allowed role `user`, which the translated permissions use (an empty role
// list would leave the user with no role Hasura accepts).
const phoneSQL = `CASE WHEN u.phone ~ '^[0-9]+$' THEN '+' || u.phone ELSE nullif(u.phone,'') END`

const usersSQLFmt = `SELECT json_build_object('id',u.id,'email',nullif(u.email,''),'email_verified',(u.email_confirmed_at IS NOT NULL),'phone',` + phoneSQL + `,'display_name',coalesce(u.raw_user_meta_data->>'full_name',u.raw_user_meta_data->>'name'),'created_at',u.created_at,'disabled',((u.banned_until IS NOT NULL AND u.banned_until > now()) OR %s),'password_hash',nullif(u.encrypted_password,''),'metadata',coalesce(u.raw_user_meta_data,'{}'::jsonb),'roles','["user"]'::json) FROM auth.users u ORDER BY u.id`

// duplicateSQL finds users that hasura-auth (unique citext email, unique phone) would reject.
const duplicateSQL = `SELECT EXISTS(SELECT 1 FROM auth.users WHERE nullif(email,'') IS NOT NULL GROUP BY lower(email) HAVING count(*) > 1) OR EXISTS(SELECT 1 FROM auth.users u WHERE nullif(u.phone,'') IS NOT NULL GROUP BY ` + phoneSQL + ` HAVING count(*) > 1)`

func exportUsers(ctx context.Context, tx pgx.Tx, w *portable.Writer, m *portable.Manifest) error {
	// gotrue's own unique index is case sensitive and ignores SSO users, so the source can hold
	// users the target refuses. Stop now, before anything reaches the target.
	var dup bool
	if tx.QueryRow(ctx, duplicateSQL).Scan(&dup) != nil {
		return errs.New("E520", "auth snapshot extraction failed")
	}
	if dup {
		return errs.New("E520", "source has users whose email or phone repeats (case-insensitive); fix them in the source first")
	}
	// deleted_at (soft delete) exists in current gotrue; older schemas lack it.
	var hasDeleted bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='auth' AND table_name='users' AND column_name='deleted_at')`).Scan(&hasDeleted) != nil {
		return errs.New("E520", "auth snapshot extraction failed")
	}
	deleted := "false"
	if hasDeleted {
		deleted = "u.deleted_at IS NOT NULL"
	}
	if err := reportAnonymousUsers(ctx, tx, m); err != nil {
		return err
	}
	return postgres.WriteUsers(ctx, tx, w, m, fmt.Sprintf(usersSQLFmt, deleted))
}

// reportAnonymousUsers records the anonymous status that hasura-auth cannot preserve.
// Inputs: snapshot transaction. Outputs: per-user metadata exemptions in the report.
// Constraints: older gotrue schemas may lack is_anonymous; user verification stays enabled.
func reportAnonymousUsers(ctx context.Context, tx pgx.Tx, m *portable.Manifest) error {
	rows, err := tx.Query(ctx, `SELECT u.id::text FROM auth.users u WHERE to_jsonb(u)->>'is_anonymous' = 'true' ORDER BY u.id`)
	if err != nil {
		return errs.New("E520", "auth snapshot extraction failed")
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return errs.New("E520", "auth snapshot extraction failed")
		}
		m.Exemptions = append(m.Exemptions, portable.Exemption{
			Kind: "metadata", ID: "user:" + id,
			Reason: "Supabase anonymous status is not preserved; imported as an ordinary user",
		})
	}
	if rows.Err() != nil {
		return errs.New("E520", "auth snapshot extraction failed")
	}
	return nil
}
