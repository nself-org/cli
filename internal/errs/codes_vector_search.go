// Registry fragment: vector-search codes (E513-E514, P7-ADOPT-07). Registered
// from init() through Register; see codes.go for the rules. The only producers
// are internal/database/vectorsearch and internal/database (TrackFunction).
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E513",
			Category:   "adopt",
			Summary:    "Vector search cannot be generated or tracked",
			DefaultWhy: "The column is not a vector(n) column, the table is not tracked in the project's on-disk Hasura metadata, no on-disk metadata exists, or the search function is missing from Postgres when tracking it.",
			DefaultFix: "Point at a vector(n) column of a tracked table (nself db hasura metadata export writes the on-disk metadata), and run nself db migrate up --migration-dir migrations before --track.",
			DocsPath:   "reference/error-codes#e513",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E514",
			Category:   "adopt",
			Summary:    "Role cannot use the vector search function",
			DefaultWhy: "A role named in --roles has no select permission on the searched table, or a role with a select permission hides the vector column or does not expose the table's select at the root (query_root_fields or subscription_root_fields without select, the relationship-only pattern). The function orders by that column and Hasura exposes it to every select role, so such a role could learn hidden data or read every row.",
			DefaultFix: "Give the role a select permission on the table, pass only roles that have one (--roles), and give every select role the vector column and a root select (or remove that role's permission).",
			DocsPath:   "reference/error-codes#e514",
			Exit:       1,
		},
	)
}
