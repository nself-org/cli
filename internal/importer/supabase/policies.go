package supabase

// Purpose: read policies, columns and row security switches from the source snapshot.
// Inputs: snapshot transaction and the dumped tables. Outputs: Table records for Plan.
// Constraints: read-only catalog queries; only dumped tables are returned.
import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
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
	rows, err := tx.Query(ctx, `SELECT n.nspname::text, c.relname::text, a.attname::text FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname = ANY($1) AND c.relkind IN ('r','p') AND a.attnum > 0 AND NOT a.attisdropped ORDER BY n.nspname, c.relname, a.attnum`, m.DB.Schemas)
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
	if err = readGrants(ctx, tx, byID); err != nil {
		return nil, false, err
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

// readGrants records which kinds the role authenticated can use on each dumped table. Without
// the role (not a Supabase database) nothing is granted, so nothing is translated.
func readGrants(ctx context.Context, tx pgx.Tx, byID map[string]*Table) error {
	var has bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='authenticated')`).Scan(&has) != nil {
		return policyErr()
	}
	for id, t := range byID {
		t.Granted = map[string]bool{}
		if !has {
			continue
		}
		var s, i, u, d bool
		if tx.QueryRow(ctx, `SELECT has_table_privilege('authenticated', to_regclass(quote_ident($1)||'.'||quote_ident($2)), 'SELECT'), has_table_privilege('authenticated', to_regclass(quote_ident($1)||'.'||quote_ident($2)), 'INSERT'), has_table_privilege('authenticated', to_regclass(quote_ident($1)||'.'||quote_ident($2)), 'UPDATE'), has_table_privilege('authenticated', to_regclass(quote_ident($1)||'.'||quote_ident($2)), 'DELETE')`, t.Schema, t.Name).Scan(&s, &i, &u, &d) != nil {
			return policyErr()
		}
		t.Granted["select"], t.Granted["insert"], t.Granted["update"], t.Granted["delete"] = s, i, u, d
		_ = id
	}
	return nil
}

// readForeign lists what the import does not move and must not drop silently: policies on
// tables outside the dumped schemas (storage.objects rules, realtime) and user triggers on
// auth.* or storage.* tables whose function lives in a dumped schema (for example the usual
// on_auth_user_created profile trigger).
func readForeign(ctx context.Context, tx pgx.Tx, m *portable.Manifest) ([]importer.Policy, []portable.Exemption, error) {
	var pols []importer.Policy
	var ex []portable.Exemption
	rows, err := tx.Query(ctx, `SELECT schemaname::text, tablename::text, policyname::text, permissive = 'PERMISSIVE', cmd::text, coalesce(qual,''), coalesce(with_check,'') FROM pg_policies WHERE NOT schemaname = ANY($1) AND schemaname NOT IN ('information_schema') AND left(schemaname,3) <> 'pg_' ORDER BY 1,2,3`, m.DB.Schemas)
	if err != nil {
		return nil, nil, policyErr()
	}
	for rows.Next() {
		var p Policy
		if rows.Scan(&p.Schema, &p.Table, &p.Name, &p.Permissive, &p.Cmd, &p.Using, &p.Check) != nil {
			rows.Close()
			return nil, nil, policyErr()
		}
		id := p.Schema + "." + p.Table
		pols = append(pols, importer.Policy{Table: id, Policy: describe(p), Reason: "policy on a Supabase-owned table is not moved; set equivalent access in the target"})
		ex = append(ex, portable.Exemption{Kind: "metadata", ID: "policy:" + id + "#" + p.Name, Reason: "RLS policy of a Supabase-owned schema; listed in untranslated_policies"})
	}
	rows.Close()
	if rows.Err() != nil {
		return nil, nil, policyErr()
	}
	trig, err := tx.Query(ctx, `SELECT n.nspname::text||'.'||c.relname::text, t.tgname::text FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_proc f ON f.oid=t.tgfoid JOIN pg_namespace fn ON fn.oid=f.pronamespace WHERE NOT t.tgisinternal AND n.nspname IN ('auth','storage') AND fn.nspname = ANY($1) ORDER BY 1,2`, m.DB.Schemas)
	if err != nil {
		return nil, nil, policyErr()
	}
	defer trig.Close()
	for trig.Next() {
		var table, name string
		if trig.Scan(&table, &name) != nil {
			return nil, nil, policyErr()
		}
		ex = append(ex, portable.Exemption{Kind: "metadata", ID: "trigger:" + table + "#" + name, Reason: "trigger on a Supabase-owned table is not moved; recreate it on the target if needed"})
	}
	return pols, ex, trig.Err()
}
