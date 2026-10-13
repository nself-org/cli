// Registry fragment: postgres image switch codes (E509-E512, P7-ADOPT-22).
// Registered from init() through Register; see codes.go for the rules. The
// producer is internal/postgres/imageswitch (E512 also from the invocation
// guard in cmd/commands).
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E509",
			Category:   "adopt",
			Summary:    "Image switch preflight failed",
			DefaultWhy: "The switch cannot start safely: postgres is not running, the target is not a known pgvector image of the same major version, the disk is too small, or the cluster has a feature the switch cannot move.",
			DefaultFix: "Fix the item named above and run `nself db image switch --to pgvector --plan` again. Nothing was changed.",
			DocsPath:   "reference/error-codes#e509",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E510",
			Category:   "adopt",
			Summary:    "Image switch verification mismatch",
			DefaultWhy: "The restored copy differs from the original (rows, schema, sequences, roles, grants), or the original changed after it was dumped. Nothing was switched.",
			DefaultFix: "Check that nothing else writes to the database while the switch runs, then run the switch again. The old volume was not touched.",
			DocsPath:   "reference/error-codes#e510",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E511",
			Category:   "adopt",
			Summary:    "Image switch restore failed",
			DefaultWhy: "Restoring the dump into the new volume failed (for example an extension the old database uses is not in the pgvector image). Nothing was switched and the new volume was removed.",
			DefaultFix: "Read the error above, fix the cause in the old database, and run the switch again. The old volume was not touched.",
			DocsPath:   "reference/error-codes#e511",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E512",
			Category:   "adopt",
			Summary:    "Image switch incomplete",
			DefaultWhy: "A postgres image switch started in this project and has not finished, so commands that change the project are blocked.",
			DefaultFix: "Run `nself db image switch --to pgvector --resume` to finish it, or `nself db image switch --rollback` to go back to the old volume.",
			DocsPath:   "reference/error-codes#e512",
			Exit:       1,
		},
	)
}
