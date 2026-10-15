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
		s.UserAuth = map[string]User{}
		rows, e := tx.Query(ctx, `SELECT jsonb_build_object(
'id',u.id,'password_hash',u.password_hash,'email',u.email,'phone',u.phone_number,
'display_name',u.display_name,'email_verified',u.email_verified,'disabled',u.disabled,
'metadata',u.metadata,'default_role',u.default_role,'locale',u.locale,
'phone_number_verified',u.phone_number_verified,'is_anonymous',u.is_anonymous,
'roles',coalesce((SELECT jsonb_agg(r.role ORDER BY r.role) FROM auth.user_roles r WHERE r.user_id=u.id),'[]'::jsonb))
FROM auth.users u`)
		if e != nil {
			return s, errors.New("target users unavailable")
		}
		e = observeAuthRows(rows, &s)
		rows.Close()
		if e != nil {
			return s, errors.New("target user read failed")
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
