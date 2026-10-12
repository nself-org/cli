package supabase

// Purpose: read policies, columns and row security switches from the source snapshot.
// Inputs: snapshot transaction and the dumped tables. Outputs: Table records for Plan.
// Constraints: read-only catalog queries; only dumped tables are returned.
import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/portable"
)

func policyErr() error { return errs.New("E520", "source policy lookup failed") }

// readTables returns each dumped table with its columns and policies, and whether any dumped
// table has row level security switched on or any policy exists.
func readTables(ctx context.Context, tx pgx.Tx, m *portable.Manifest) ([]Table, bool, error) {
	byID := map[string]*Table{}
	order := []string{}
	for _, t := range m.DB.Tables {
		id := t.Schema + "." + t.Name
		byID[id] = &Table{Schema: t.Schema, Name: t.Name}
		order = append(order, id)
	}
	rows, err := tx.Query(ctx, `SELECT table_schema::text, table_name::text, column_name::text FROM information_schema.columns WHERE table_schema = ANY($1) ORDER BY table_schema, table_name, ordinal_position`, m.DB.Schemas)
	if err != nil {
		return nil, false, policyErr()
	}
	for rows.Next() {
		var s, t, c string
		if rows.Scan(&s, &t, &c) != nil {
			rows.Close()
			return nil, false, policyErr()
		}
		if tbl := byID[s+"."+t]; tbl != nil {
			tbl.Columns = append(tbl.Columns, c)
		}
	}
	rows.Close()
	if rows.Err() != nil {
		return nil, false, policyErr()
	}
	rows, err = tx.Query(ctx, `SELECT schemaname::text, tablename::text, policyname::text, permissive = 'PERMISSIVE', roles::text[], cmd::text, coalesce(qual,''), coalesce(with_check,'') FROM pg_policies WHERE schemaname = ANY($1) ORDER BY schemaname, tablename, policyname`, m.DB.Schemas)
	if err != nil {
		return nil, false, policyErr()
	}
	found := false
	for rows.Next() {
		var p Policy
		if rows.Scan(&p.Schema, &p.Table, &p.Name, &p.Permissive, &p.Roles, &p.Cmd, &p.Using, &p.Check) != nil {
			rows.Close()
			return nil, false, policyErr()
		}
		if tbl := byID[p.Schema+"."+p.Table]; tbl != nil {
			tbl.Policies = append(tbl.Policies, p)
			found = true
		}
	}
	rows.Close()
	if rows.Err() != nil {
		return nil, false, policyErr()
	}
	var rls bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relrowsecurity AND n.nspname = ANY($1))`, m.DB.Schemas).Scan(&rls) != nil {
		return nil, false, policyErr()
	}
	out := make([]Table, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, found || rls, nil
}
