// Registry fragment: canon engine codes (E410, P7-CANON-21). Registered from
// init() through Register; see codes.go for the rules and codes_blocks.go for the
// allocation (E411-E412 are spare in this fragment).
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E410",
			Category:   "cli",
			Summary:    "Command removed in v1.5",
			DefaultWhy: "This command was removed in nSelf v1.5 and has no replacement.",
			DefaultFix: "Read the message for what to use instead, or run nself --help to see the current commands.",
			DocsPath:   "reference/error-codes#e410",
			Exit:       1,
		},
	)
}
