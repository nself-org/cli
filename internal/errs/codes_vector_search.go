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
			Summary:    "Role has no select permission on the table",
			DefaultWhy: "A role named in --roles has no select permission on the searched table, so a search function granted to it would expose no rows.",
			DefaultFix: "Add a select permission for the role on the table, or pass only roles that already have one (--roles).",
			DocsPath:   "reference/error-codes#e514",
			Exit:       1,
		},
	)
}
