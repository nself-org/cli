// Registry fragment: removed legacy MCP tool code (E433), owned by P7-SURF-03.
// Registered from init() through Register; see codes.go for the rules and
// codes_blocks.go for the allocation.
package errs

func init() {
	Register(CodeEntry{
		Code:       "E433",
		Category:   "cli",
		Summary:    "Legacy MCP tool removed in v1.5",
		DefaultWhy: "This MCP tool ran raw SQL outside the migration ledger, and v1.5 no longer serves it.",
		DefaultFix: "Write the change as a migration with 'nself db migrate create', then apply it with 'nself db migrate up'.",
		DocsPath:   "reference/error-codes#e433",
		Exit:       1,
	})
}
