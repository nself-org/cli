package exporter

import (
	"context"
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/portable"
)

var bcryptHash = regexp.MustCompile(`^\$2[aby]\$`)

// Purpose: export only portable auth user fields, never sessions or tokens.
// Inputs: the database snapshot. Outputs: sorted JSONL and algorithm counts.
// Constraints: bcrypt hashes are allowed; metadata is still scrubbed.
func writeAuth(ctx context.Context, tx pgx.Tx, w *portable.Writer, m *portable.Manifest) error {
	m.Auth = portable.Auth{HashAlgorithms: map[string]int{}, ResetRequired: []string{}}
	var exists bool
	if err := tx.QueryRow(ctx, "SELECT to_regclass('auth.users') IS NOT NULL").Scan(&exists); err != nil {
		return err
	}
	if !exists {
		_, err := w.WriteFile("auth/users.jsonl", strings.NewReader(""))
		return err
	}
	rows, err := tx.Query(ctx, `SELECT jsonb_build_object(
'id',u.id,'email',u.email,'email_verified',u.email_verified,'phone',u.phone_number,
'display_name',u.display_name,'created_at',u.created_at,'disabled',u.disabled,
'password_hash',u.password_hash,'metadata',u.metadata,
'roles',coalesce((SELECT jsonb_agg(r.role ORDER BY r.role COLLATE "C") FROM auth.user_roles r WHERE r.user_id=u.id),'[]'::jsonb))
FROM auth.users u ORDER BY u.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	pr, pw := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		var failure error
		defer func() { _ = pw.CloseWithError(failure) }()
		enc := json.NewEncoder(pw)
		for rows.Next() {
			var data []byte
			if failure = rows.Scan(&data); failure != nil {
				return
			}
			var user map[string]any
			if failure = json.Unmarshal(data, &user); failure != nil {
				return
			}
			algo := "none"
			if hash, ok := user["password_hash"].(string); ok && bcryptHash.MatchString(hash) {
				algo = "bcrypt"
			}
			user["hash_algorithm"] = algo
			if failure = enc.Encode(user); failure != nil {
				return
			}
			m.Auth.Users++
			m.Auth.HashAlgorithms[algo]++
		}
		failure = rows.Err()
	}()
	_, err = w.WriteFile("auth/users.jsonl", pr)
	_ = pr.CloseWithError(err)
	<-done
	if err != nil {
		return err
	}
	m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "users", ID: "auth.users", Count: int64(m.Auth.Users), Method: "snapshot auth.users"})
	return nil
}
