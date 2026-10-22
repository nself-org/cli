package supabase

// Purpose: export Supabase auth users into the portable users member.
// Inputs: the snapshot transaction. Outputs: auth/users.jsonl and independent user counts.
// Constraints: the bcrypt hash (gotrue encrypted_password) belongs only to the private bundle.
import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nself-org/cli/internal/importer/verify"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer/postgres"
	"github.com/nself-org/cli/internal/portable"
)

// usersSQL maps gotrue's auth.users to the portable fields. Empty strings become NULL so a
// phone-only or OAuth user keeps a NULL email and no hash; banned_until in the future disables.
// Every user gets the allowed role `user`, which the translated permissions use (an empty role
// list would leave the user with no role Hasura accepts).
const usersSQLFmt = `SELECT json_build_object('id',u.id,'is_anonymous',coalesce((to_jsonb(u)->>'is_anonymous')::boolean,false),'email',nullif(u.email,''),'email_verified',(u.email_confirmed_at IS NOT NULL),'phone',nullif(u.phone,''),'display_name',coalesce(u.raw_user_meta_data->>'full_name',u.raw_user_meta_data->>'name'),'created_at',u.created_at,'disabled',((u.banned_until IS NOT NULL AND u.banned_until > now()) OR %s),'password_hash',nullif(u.encrypted_password,''),'metadata',coalesce(u.raw_user_meta_data,'{}'::jsonb),'roles','["user"]'::json) FROM auth.users u ORDER BY u.id`

// duplicateSQL finds users that hasura-auth (unique citext email, unique phone) would reject.
const duplicateSQL = `SELECT EXISTS(SELECT 1 FROM auth.users WHERE nullif(email,'') IS NOT NULL GROUP BY lower(email) HAVING count(*) > 1) OR EXISTS(SELECT 1 FROM auth.users WHERE nullif(phone,'') IS NOT NULL GROUP BY CASE WHEN phone ~ '^[0-9]+$' THEN '+' || phone ELSE phone END HAVING count(*) > 1)`

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
	return postgres.WriteUsers(ctx, tx, w, m, fmt.Sprintf(usersSQLFmt, deleted), normalizeUser(m))
}

// normalizeUser keeps GoTrue's digit-only phones in international form and reports
// anonymous accounts whose temporary-session semantics the target cannot preserve.
func normalizeUser(m *portable.Manifest) func(*verify.User, []byte) {
	return func(u *verify.User, raw []byte) {
		if u.Phone != nil && *u.Phone != "" {
			digits := true
			for _, ch := range *u.Phone {
				if ch < '0' || ch > '9' {
					digits = false
					break
				}
			}
			if digits {
				phone := "+" + *u.Phone
				u.Phone = &phone
			}
		}
		var source struct {
			Anonymous bool `json:"is_anonymous"`
		}
		if json.Unmarshal(raw, &source) == nil && source.Anonymous {
			m.Exemptions = append(m.Exemptions, portable.Exemption{Kind: "metadata", ID: "user:" + u.ID + "#anonymous", Reason: "Supabase anonymous account imported as a regular user; anonymous session semantics are not carried"})
		}
	}
}
