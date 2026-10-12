package load

// Purpose: translate portable auth users into the target hasura-auth schema.
// Inputs: decoded portable users. Outputs: atomic COPY of users and roles.
// Constraints: only bcrypt hashes carried; no user content in errors.
import (
	"context"
	"crypto/rand"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nself-org/cli/internal/importer/verify"
	"time"
)

func ApplyUsers(ctx context.Context, db *pgx.Conn, users []verify.User) error {
	if len(users) == 0 {
		return nil
	}
	var citextOID uint32
	if err := db.QueryRow(ctx, `SELECT oid FROM pg_type WHERE typname=$1`, "citext").Scan(&citextOID); err == nil {
		db.TypeMap().RegisterType(&pgtype.Type{Name: "citext", OID: citextOID, Codec: pgtype.TextCodec{}})
	}
	nullable, err := nullableAuthColumns(ctx, db)
	if err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return errors.New("auth transaction failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	values := make([][]any, 0, len(users))
	roles := [][]any{}
	for _, u := range users {
		var id pgtype.UUID
		if err = id.Scan(u.ID); err != nil {
			return errors.New("invalid auth user identifier")
		}
		created, err := time.Parse(time.RFC3339Nano, u.CreatedAt)
		if err != nil {
			return errors.New("invalid user creation time")
		}
		var hash *string
		if u.HashAlgorithm == "bcrypt" {
			hash = u.PasswordHash
		}
		values = append(values, userRow(id, created, hash, u, nullable))
		for _, role := range u.Roles {
			if _, err = tx.Exec(ctx, `INSERT INTO auth.roles(role) VALUES($1) ON CONFLICT DO NOTHING`, role); err != nil {
				return errors.New("auth role registration failed")
			}
			var roleID [16]byte
			if _, err = rand.Read(roleID[:]); err != nil {
				return errors.New("auth role identifier generation failed")
			}
			roleID[6] = (roleID[6] & 0x0f) | 0x40
			roleID[8] = (roleID[8] & 0x3f) | 0x80
			roles = append(roles, []any{pgtype.UUID{Bytes: roleID, Valid: true}, created, id, role})
		}
	}
	columns := []string{"id", "email", "email_verified", "phone_number", "display_name", "created_at", "disabled", "password_hash", "metadata", "updated_at", "avatar_url", "locale", "phone_number_verified", "default_role", "is_anonymous", "otp_hash_expires_at", "ticket_expires_at"}
	if _, err = tx.CopyFrom(ctx, pgx.Identifier{"auth", "users"}, columns, pgx.CopyFromRows(values)); err != nil {
		return authCopyError(err, "users")
	}
	if len(roles) > 0 {
		if _, err = tx.CopyFrom(ctx, pgx.Identifier{"auth", "user_roles"}, []string{"id", "created_at", "user_id", "role"}, pgx.CopyFromRows(roles)); err != nil {
			return authCopyError(err, "roles")
		}
	}
	if tx.Commit(ctx) != nil {
		return errors.New("auth commit failed")
	}
	return nil
}
func authCopyError(err error, phase string) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return errors.New("auth " + phase + " COPY failed (SQLSTATE " + pg.Code + ")")
	}
	return errors.New("auth " + phase + " COPY failed (encoding or connection)")
}

// nullOrString keeps a NULL as NULL when the column is nullable, else uses the empty string.
func nullOrString(nullable bool, v *string) any {
	if v != nil {
		return *v
	}
	if nullable {
		return nil
	}
	return ""
}

// nullOr returns nil for a nullable column, else the supplied default.
func nullOr(nullable bool, def any) any {
	if nullable {
		return nil
	}
	return def
}

// nullableAuthColumns lists the auth.users columns that accept NULL in this target.
func nullableAuthColumns(ctx context.Context, db *pgx.Conn) (map[string]bool, error) {
	rows, err := db.Query(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema='auth' AND table_name='users' AND is_nullable='YES'`)
	if err != nil {
		return nil, errors.New("auth column lookup failed")
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if rows.Scan(&name) != nil {
			return nil, errors.New("auth column lookup failed")
		}
		out[name] = true
	}
	if rows.Err() != nil {
		return nil, errors.New("auth column lookup failed")
	}
	return out, nil
}

// userRow builds one auth.users COPY row. NULL stays NULL wherever the target column allows
// it; hasura-auth keeps email unique, so mapping a NULL email to "" would make the second
// phone-only user collide.
func userRow(id pgtype.UUID, created time.Time, hash *string, u verify.User, nullable map[string]bool) []any {
	var meta any = u.Metadata
	if u.Metadata == nil {
		meta = nullOr(nullable["metadata"], map[string]any{})
	}
	return []any{id, nullOrString(nullable["email"], u.Email), u.EmailVerified, nullOrString(nullable["phone_number"], u.Phone), nullOrString(nullable["display_name"], u.DisplayName), created, u.Disabled, hash, meta, created, "", "en", false, "user", false, time.Unix(0, 0).UTC(), time.Unix(0, 0).UTC()}
}
