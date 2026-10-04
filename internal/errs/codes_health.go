// Registry fragment: health codes (E250-E299). Registered from init() through
// Register; see codes.go for the rules and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E250",
			Category:   "health",
			Summary:    "Service unhealthy",
			DefaultWhy: "A service health check returned an unhealthy status.",
			DefaultFix: "Run 'nself doctor --verbose' for detailed diagnostics.",
			DocsPath:   "reference/error-codes#e250",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E251",
			Category:   "health",
			Summary:    "Health check timeout",
			DefaultWhy: "The health check did not complete within the timeout period.",
			DefaultFix: "The service may be starting slowly. Wait and retry, or check logs with 'nself logs'.",
			DocsPath:   "reference/error-codes#e251",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E252",
			Category:   "health",
			Summary:    "Service not found",
			DefaultWhy: "The named service is not part of this project stack.",
			DefaultFix: "Run nself status to list the services in this project, and check the spelling.",
			DocsPath:   "reference/error-codes#e252",
			Exit:       2,
		},
	)
}
