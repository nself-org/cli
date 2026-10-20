package verify

// Purpose: observe loaded data through independent target reads.
// Inputs: pgx connection, active store, verified bundle. Outputs: comparison state.
// Constraints: hashes stream; errors never carry database or object values.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/objstore"
	"github.com/nself-org/cli/internal/portable"
	"github.com/nself-org/cli/internal/postgres/tablehash"
	"io"
)

type Live struct {
	HasAuth  bool
	DB       *pgx.Conn
	Config   *config.Config
	Store    *objstore.Client
	Users    []User
	Metadata Metadata
	Observed State
}

func (l *Live) Expected() ([]User, Metadata) { return l.Users, l.Metadata }
func (l *Live) Snapshot(ctx context.Context, m portable.Manifest) (State, error) {
	s := State{Tables: map[string]portable.Table{}, Users: map[string]string{}, Objects: map[string]string{}, Counts: map[string]int64{}}
	tx, err := l.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return s, errors.New("target snapshot failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tables := []tablehash.Table{}
	for _, t := range m.DB.Tables {
		var exists bool
		if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=$1 AND table_name=$2)`, t.Schema, t.Name).Scan(&exists) != nil {
			return s, errors.New("target table lookup failed")
		}
		if exists {
			tables = append(tables, tablehash.Table{Schema: t.Schema, Name: t.Name})
		}
	}
	hashed, _, err := tablehash.Snapshot(ctx, tx, tables)
	if err != nil {
		return s, errors.New("target table hash failed")
	}
	for _, t := range hashed {
		id := t.Table.String()
		s.Tables[id] = portable.Table{Schema: t.Table.Schema, Name: t.Table.Name, Rows: t.Rows, Hash: t.Hash}
		s.Counts["table\x00"+id] = t.Rows
	}
	if l.HasAuth || len(l.Users) > 0 {
		s.UserIdent = map[string]string{}
		s.UserAuth = map[string]UserAuth{}
		rows, e := tx.Query(ctx, `SELECT id::text, password_hash, email::text, phone_number, display_name, default_role, email_verified, disabled FROM auth.users`)
		if e != nil {
			return s, errors.New("target users unavailable")
		}
		for rows.Next() {
			var id string
			var hash, email, phone, display *string
			var ua UserAuth
			if rows.Scan(&id, &hash, &email, &phone, &display, &ua.DefaultRole, &ua.EmailVerified, &ua.Disabled) != nil {
				rows.Close()
				return s, errors.New("target user read failed")
			}
			algo := "none"
			if hash != nil && len(*hash) >= 4 && (*hash)[:2] == "$2" {
				algo = "bcrypt"
			}
			s.Users[id] = algo
			s.UserIdent[id] = Identity(email, phone, display)
			s.UserAuth[id] = ua
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return s, errors.New("target user read failed")
		}
		if e = readUserRoles(ctx, tx, s.UserAuth); e != nil {
			return s, e
		}
	}
	s.Counts["users\x00"] = int64(len(s.Users))
	for _, o := range m.Storage.Objects {
		if l.Store == nil {
			continue
		}
		body, e := l.Store.Get(ctx, o.Bucket, o.Key)
		if e != nil {
			var api *objstore.Error
			if errors.As(e, &api) && api.IsNotFound() {
				continue
			}
			return s, errors.New("target object read failed")
		}
		h := sha256.New()
		_, e = io.Copy(h, body)
		_ = body.Close()
		if e != nil {
			return s, errors.New("target object hash failed")
		}
		s.Objects[o.Bucket+"/"+o.Key] = hex.EncodeToString(h.Sum(nil))
	}
	s.Counts["objects\x00"] = int64(len(s.Objects))
	for _, c := range m.SourceCounts {
		if c.Kind == "users" {
			s.Counts["users\x00"+c.ID] = int64(len(s.Users))
		}
		if c.Kind == "objects" {
			var count int64
			for _, o := range m.Storage.Objects {
				if objectInCount(c.ID, m, o) {
					if _, ok := s.Objects[o.Bucket+"/"+o.Key]; ok {
						count++
					}
				}
			}
			s.Counts["objects\x00"+c.ID] = count
		}
	}
	b, err := database.HasuraExportMetadata(ctx, l.Config)
	if err != nil {
		return s, errors.New("target metadata unavailable")
	}
	s.Metadata, err = ParseMetadata(b)
	if err != nil {
		return s, err
	}
	l.Observed = s
	return s, nil
}

// readUserRoles fills the Roles of each user in into from auth.user_roles.
func readUserRoles(ctx context.Context, tx pgx.Tx, into map[string]UserAuth) error {
	rows, err := tx.Query(ctx, `SELECT user_id::text, role FROM auth.user_roles ORDER BY user_id, role`)
	if err != nil {
		return errors.New("target user roles unavailable")
	}
	defer rows.Close()
	for rows.Next() {
		var id, role string
		if rows.Scan(&id, &role) != nil {
			return errors.New("target user role read failed")
		}
		if ua, ok := into[id]; ok {
			ua.Roles = append(ua.Roles, role)
			into[id] = ua
		}
	}
	if rows.Err() != nil {
		return errors.New("target user role read failed")
	}
	return nil
}
