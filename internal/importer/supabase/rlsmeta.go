package supabase

// Purpose: turn a table's policies into a Hasura metadata table record plus the report lines.
// Inputs: tables with columns and policies. Outputs: metadata document, untranslated policies
// and one exemption per stripped policy. Constraints: pure and deterministic.
import (
	"encoding/json"
	"sort"

	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/portable"
)

// Table is one dumped table with its columns and policies.
type Table struct {
	Schema, Name string
	Columns      []string
	Policies     []Policy
	// Granted lists the permission kinds (select, insert, update, delete) the Supabase role
	// `authenticated` holds on the table. A policy for a kind without the grant gives no access in
	// Postgres, so it is not translated. A nil map means every kind (unit tests).
	Granted map[string]bool
}

// Result is the outcome of translating every table.
type Result struct {
	// Doc is the export_metadata subset (nil when no policy translated).
	Doc          []byte
	Tables       int
	Permissions  int
	Untranslated []importer.Policy
	Exemptions   []portable.Exemption
}

// Plan translates every table's policies. Every policy is either part of a permission or in
// Untranslated, and each one is stripped from the restore, so it also gets an exemption.
func Plan(tables []Table) (Result, error) {
	res := Result{Untranslated: []importer.Policy{}, Exemptions: []portable.Exemption{}}
	records := []map[string]any{}
	tables = append([]Table(nil), tables...)
	sort.Slice(tables, func(i, j int) bool {
		return tables[i].Schema+"."+tables[i].Name < tables[j].Schema+"."+tables[j].Name
	})
	for _, t := range tables {
		cols := map[string]bool{}
		for _, c := range t.Columns {
			cols[c] = true
		}
		owners := map[string][]string{}
		restrictive := ""
		for _, p := range t.Policies {
			if !p.Permissive && restrictive == "" {
				restrictive = p.Name
			}
		}
		for _, p := range t.Policies {
			id := t.Schema + "." + t.Name
			exemptionID := "policy:" + id + "#" + p.Name
			tr, reason := Translate(p, cols)
			if reason == "" && restrictive != "" {
				// A restrictive policy ANDs with the permissive ones; Hasura has no such rule, so
				// translating the permissive ones alone would grant more than Postgres does.
				reason = "table has restrictive policy " + restrictive + "; permissive policies are not translated because Hasura would grant more"
			}
			if reason == "" {
				if kinds, ok := grantedKinds(t, tr.Kinds); !ok {
					reason = "role authenticated has no matching privilege on the table"
				} else {
					tr.Kinds = kinds
				}
			}
			if reason != "" {
				res.Untranslated = append(res.Untranslated, importer.Policy{Table: id, Policy: describe(p), Reason: reason})
				res.Exemptions = append(res.Exemptions, portable.Exemption{Kind: "metadata", ID: exemptionID, Reason: "RLS policy not restored; listed in untranslated_policies"})
				continue
			}
			for _, k := range tr.Kinds {
				owners[k] = append(owners[k], tr.Column)
			}
			res.Exemptions = append(res.Exemptions, portable.Exemption{Kind: "metadata", ID: exemptionID, Reason: "RLS policy not restored; translated to Hasura permission"})
		}
		if len(owners) == 0 {
			continue
		}
		rec := map[string]any{"table": map[string]any{"schema": t.Schema, "name": t.Name}}
		columns := append([]string(nil), t.Columns...)
		sort.Strings(columns)
		for _, kind := range []string{"select", "insert", "update", "delete"} {
			cs, ok := owners[kind]
			if !ok {
				continue
			}
			f := combine(cs)
			perm := map[string]any{}
			switch kind {
			case "select":
				perm = map[string]any{"columns": columns, "filter": f}
			case "insert":
				perm = map[string]any{"columns": columns, "check": f}
			case "update":
				perm = map[string]any{"columns": columns, "filter": f, "check": f}
			case "delete":
				perm = map[string]any{"filter": f}
			}
			rec[kind+"_permissions"] = []any{map[string]any{"role": hasuraRole, "permission": perm}}
			res.Permissions++
		}
		records = append(records, rec)
		res.Tables++
	}
	if len(records) == 0 {
		return res, nil
	}
	doc, err := json.MarshalIndent(map[string]any{"version": 3, "sources": []any{map[string]any{"name": "default", "tables": records}}}, "", "  ")
	if err != nil {
		return res, err
	}
	res.Doc = append(doc, '\n')
	return res, nil
}

// describe renders the policy text for the report.
func describe(p Policy) string {
	s := p.Name + " (" + p.Cmd
	if !p.Permissive {
		s += ", restrictive"
	}
	s += ")"
	if p.Using != "" {
		s += " USING " + p.Using
	}
	if p.Check != "" {
		s += " WITH CHECK " + p.Check
	}
	return s
}

// grantedKinds keeps the kinds the table grants to authenticated; ok is false when none remain.
func grantedKinds(t Table, kinds []string) ([]string, bool) {
	if t.Granted == nil {
		return kinds, true
	}
	var out []string
	for _, k := range kinds {
		if t.Granted[k] {
			out = append(out, k)
		}
	}
	return out, len(out) > 0
}
