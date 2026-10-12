package postgres

// Purpose: portable hasura-auth user snapshots, separate from user schemas.
// Inputs: snapshot transaction. Outputs: auth JSONL and independent counts.
// Constraints: user password hashes belong only to the private bundle, never report/errors.
import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/importer/verify"
	"github.com/nself-org/cli/internal/portable"
)

func exportUsers(ctx context.Context, tx pgx.Tx, w *portable.Writer, m *portable.Manifest) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('auth.users') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT json_build_object('id',u.id,'email',u.email,'email_verified',u.email_verified,'phone',u.phone_number,'display_name',u.display_name,'created_at',u.created_at,'disabled',u.disabled,'password_hash',u.password_hash,'metadata',u.metadata,'roles',coalesce((SELECT json_agg(r.role ORDER BY r.role) FROM auth.user_roles r WHERE r.user_id=u.id),'[]'::json)) FROM auth.users u ORDER BY u.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	m.Auth.HashAlgorithms = map[string]int{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return err
		}
		var u verify.User
		if err = json.Unmarshal(raw, &u); err != nil {
			return err
		}
		u.HashAlgorithm = "none"
		if u.PasswordHash != nil && len(*u.PasswordHash) >= 4 && (*u.PasswordHash)[:2] == "$2" {
			u.HashAlgorithm = "bcrypt"
		} else if u.PasswordHash != nil && *u.PasswordHash != "" {
			u.HashAlgorithm = "unsupported"
			m.Auth.ResetRequired = append(m.Auth.ResetRequired, u.ID)
		}
		if err = enc.Encode(u); err != nil {
			return err
		}
		m.Auth.Users++
		m.Auth.HashAlgorithms[u.HashAlgorithm]++
	}
	if err = rows.Err(); err != nil {
		return err
	}
	var count int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM auth.users`).Scan(&count); err != nil {
		return err
	}
	m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "users", Count: count, Method: "independent count(*)"})
	_, err = w.WriteFile("auth/users.jsonl", &buf)
	return err
}
