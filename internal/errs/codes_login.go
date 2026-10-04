// Registry fragment: login codes. E225 sits in the database block (E200-E249)
// because the P7-PROD Epic allocated it there (D20). Registered from init()
// through Register; see codes.go for the rules and codes_blocks.go for the
// allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E225",
			Category:   "database",
			Summary:    "Server does not provide this path",
			DefaultWhy: "The server answered 404 for an account path this CLI version calls but the nSelf auth server does not serve.",
			DefaultFix: "Use the nself.org account pages for this action, or update the CLI to a version that matches the server.",
			DocsPath:   "reference/error-codes#e225",
			Exit:       2,
		},
	)
}
