// Registry fragment: machine invocation codes (E420-E423, P7-SURF-24).
// Registered from init() through Register; see codes.go for the rules and
// codes_blocks.go for the allocation. E424-E426 belong to the gate fragment of
// P7-SURF-25.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E420",
			Category:   "cli",
			Summary:    "Invalid machine request",
			DefaultWhy: "The request sent over MCP or HTTP does not match the command's parameters.",
			DefaultFix: "Send only the args, flags and confirm members the command's params schema lists, with the right types.",
			DocsPath:   "reference/error-codes#e420",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E421",
			Category:   "cli",
			Summary:    "Command not exposed on machine surfaces",
			DefaultWhy: "This command is not served over MCP or HTTP; the reason is in the cause.",
			DefaultFix: "Run the command from a terminal, or call a command that is exposed.",
			DocsPath:   "reference/error-codes#e421",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E422",
			Category:   "cli",
			Summary:    "Command produced no single JSON document",
			DefaultWhy: "The command ran but its standard output was not exactly one JSON document.",
			DefaultFix: "Run the same command in a terminal with --json and read its output; report the command if it prints text in JSON mode.",
			DocsPath:   "reference/error-codes#e422",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E423",
			Category:   "cli",
			Summary:    "Command invocation timed out",
			DefaultWhy: "The command did not finish before the invocation timeout and was stopped.",
			DefaultFix: "Raise the server invocation timeout, or use a command form that returns sooner.",
			DocsPath:   "reference/error-codes#e423",
			Exit:       2,
		},
	)
}
