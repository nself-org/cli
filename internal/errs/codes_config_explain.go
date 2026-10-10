// Registry fragment: config explain code (E434), owned by P7-SURF-06.
// Registered from init() through Register; see codes.go for the rules and
// codes_blocks.go for the allocation.
package errs

func init() {
	Register(CodeEntry{
		Code:       "E434",
		Category:   "cli",
		Summary:    "Unknown configuration key or unbound flag",
		DefaultWhy: "nself config explain was given a key that is not a known configuration key, or a command flag that supplies no configuration key.",
		DefaultFix: "Run 'nself config list' for the known keys, or 'nself help --json' and read each flag's env field for the flags that supply one.",
		DocsPath:   "reference/error-codes#e434",
		Exit:       1,
	})
}
