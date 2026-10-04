// Registry fragment: domain codes (E350-E399). Registered from init() through
// Register; see codes.go for the rules and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E350",
			Category:   "domain",
			Summary:    "Invalid domain name",
			DefaultWhy: "The configured domain name is not valid.",
			DefaultFix: "Use a valid domain like 'example.com' or 'localhost'.",
			DocsPath:   "reference/error-codes#e350",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E351",
			Category:   "domain",
			Summary:    "Invalid port number",
			DefaultWhy: "Port number is outside the valid range (1-65535).",
			DefaultFix: "Use a port number between 1024 and 65535 for non-privileged ports.",
			DocsPath:   "reference/error-codes#e351",
			Exit:       1,
		},
	)
}
