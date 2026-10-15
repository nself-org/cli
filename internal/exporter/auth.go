package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/portable"
)

// bcryptHash is the full modular-crypt bcrypt shape: version, two-digit cost
// and 53 salt-plus-hash characters. A looser prefix match would let any text
// that starts with $2b$ skip the secret scan.
var bcryptHash = regexp.MustCompile(`^\$2[aby]\$\d{2}\$[./A-Za-z0-9]{53}$`)

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
'password_hash',u.password_hash,'metadata',u.metadata,'default_role',u.default_role,
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
		_ = pw.CloseWithError(streamAuth(rows, pw, &m.Auth))
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

type userRows interface {
	Next() bool
	Scan(...any) error
	Err() error
}

// Purpose: stream portable user records without holding the user set in memory.
// Inputs: snapshot rows and a private writer. Outputs: JSONL and algorithm counts.
// Constraints: only portable fields arrive from the explicit auth query.
func streamAuth(rows userRows, w io.Writer, auth *portable.Auth) error {
	enc := json.NewEncoder(w)
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return err
		}
		// UseNumber keeps integers beyond float64 precision intact.
		var user map[string]any
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&user); err != nil {
			return err
		}
		algo := "none" // no stored hash (null or absent)
		if v := user["password_hash"]; v != nil {
			algo = "unknown"
			if hash, ok := v.(string); ok && bcryptHash.MatchString(hash) {
				algo = "bcrypt"
			}
		}
		user["hash_algorithm"] = algo
		if err := enc.Encode(user); err != nil {
			return err
		}
		auth.Users++
		auth.HashAlgorithms[algo]++
	}
	return rows.Err()
}
