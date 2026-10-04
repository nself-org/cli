// Registry fragment: init codes (E300-E349). Registered from init() through
// Register; see codes.go for the rules and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E300",
			Category:   "init",
			Summary:    "Project already initialized",
			DefaultWhy: "A .env file already exists in this directory.",
			DefaultFix: "Use 'nself config set' to modify existing config, or delete .env to reinitialize.",
			DocsPath:   "reference/error-codes#e300",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E301",
			Category:   "init",
			Summary:    "Source directory detected",
			DefaultWhy: "You are running nself inside the CLI source repository.",
			DefaultFix: "Change to your project directory first: cd /path/to/your/project",
			DocsPath:   "reference/error-codes#e301",
			Exit:       1,
		},
	)
}
