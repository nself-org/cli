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
	return WriteUsers(ctx, tx, w, m, `SELECT json_build_object('id',u.id,'email',u.email,'email_verified',u.email_verified,'phone',u.phone_number,'display_name',u.display_name,'created_at',u.created_at,'disabled',u.disabled,'password_hash',u.password_hash,'metadata',u.metadata,'roles',coalesce((SELECT json_agg(r.role ORDER BY r.role) FROM auth.user_roles r WHERE r.user_id=u.id),'[]'::json)) FROM auth.users u ORDER BY u.id`)
}

// WriteUsers runs selectSQL (one JSON user object per row, ordered by id) inside the snapshot
// and writes auth/users.jsonl with each user's hash algorithm, the manifest user counts and the
// independent count(*) of auth.users. A hash that is not bcrypt marks the user reset-required.
func WriteUsers(ctx context.Context, tx pgx.Tx, w *portable.Writer, m *portable.Manifest, selectSQL string) error {
	rows, err := tx.Query(ctx, selectSQL)
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
