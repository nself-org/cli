package supabase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/portable"
)

func TestUsersPhoneNormalization(t *testing.T) {
	// Both queries must use the same canonical phone key: otherwise +1555 and
	// 1555 pass preflight but collide when inserted into hasura-auth.
	normalized := "CASE WHEN u.phone ~ '^[0-9]+$' THEN '+' || u.phone ELSE nullif(u.phone,'') END"
	if !strings.Contains(fmt.Sprintf(usersSQLFmt, "false"), "'phone',"+normalized) {
		t.Error("export must prefix digit-only phones, preserve other phones and null empty phones")
	}
	if !strings.Contains(duplicateSQL, "GROUP BY "+normalized) {
		t.Error("duplicate detection must group by the normalized phone")
	}
}

type userRow struct{ value bool }

func (r userRow) Scan(dest ...any) error {
	*(dest[0].(*bool)) = r.value
	return nil
}

type userRows struct {
	pgx.Rows
	ids []string
	err error
}

func (r *userRows) Next() bool { return len(r.ids) > 0 }
func (r *userRows) Scan(dest ...any) error {
	*(dest[0].(*string)) = r.ids[0]
	r.ids = r.ids[1:]
	return nil
}
func (r *userRows) Close()     {}
func (r *userRows) Err() error { return r.err }

type userTx struct {
	pgx.Tx
	rows     *userRows
	queryErr error
}

func (tx userTx) QueryRow(context.Context, string, ...any) pgx.Row { return userRow{} }
func (tx userTx) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if strings.Contains(sql, "is_anonymous") {
		return tx.rows, tx.queryErr
	}
	// End after collecting exemptions, before exporting users. No bundle is needed.
	return nil, errors.New("end of fixture")
}

func TestUsersAnonymousExemptions(t *testing.T) {
	m := &portable.Manifest{Exemptions: []portable.Exemption{{Kind: "metadata", ID: "existing"}}}
	tx := userTx{rows: &userRows{ids: []string{"anonymous-user-1", "anonymous-user-2"}}}
	_ = exportUsers(context.Background(), tx, nil, m)
	if len(m.Exemptions) != 3 {
		t.Fatalf("anonymous users must each be reported without losing existing exemptions: %+v", m.Exemptions)
	}
	for i, id := range []string{"anonymous-user-1", "anonymous-user-2"} {
		e := m.Exemptions[i+1]
		if e.Kind != "metadata" || e.ID != "user:"+id || !strings.Contains(e.Reason, "anonymous") {
			t.Errorf("missing anonymous status exemption for %s: %+v", id, e)
		}
	}
}

func TestUsersAnonymousReadFailureStopsExport(t *testing.T) {
	for _, tx := range []userTx{
		{queryErr: errors.New("private database detail")},
		{rows: &userRows{err: errors.New("private database detail")}},
	} {
		err := exportUsers(context.Background(), tx, nil, &portable.Manifest{})
		if err == nil || !strings.Contains(err.Error(), "auth snapshot extraction failed") || strings.Contains(err.Error(), "private") {
			t.Fatalf("anonymous lookup failure must stop extraction without database details: %v", err)
		}
	}
}
