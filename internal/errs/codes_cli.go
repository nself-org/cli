// Registry fragment: cli codes (E400-E449). Registered from init() through
// Register; see codes.go for the rules and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E400",
			Category:   "cli",
			Summary:    "Unclassified error",
			DefaultWhy: "The command failed with an error that has no specific code.",
			DefaultFix: "Read the message. If it looks like a bug, report it at https://github.com/nself-org/cli/issues",
			DocsPath:   "reference/error-codes#e400",
			Exit:       0,
		},
		CodeEntry{
			Code:       "E401",
			Category:   "cli",
			Summary:    "Invalid usage",
			DefaultWhy: "A flag or argument was missing, unknown or had an invalid value.",
			DefaultFix: "Run the command with --help to see the accepted flags and arguments.",
			DocsPath:   "reference/error-codes#e401",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E402",
			Category:   "cli",
			Summary:    "JSON output not supported",
			DefaultWhy: "This command or flag combination cannot produce JSON output.",
			DefaultFix: "Run the command without --json, or use nself help --json to see which commands support it.",
			DocsPath:   "reference/error-codes#e402",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E403",
			Category:   "cli",
			Summary:    "Destructive action blocked",
			DefaultWhy: "A safety gate refused a destructive action.",
			DefaultFix: "Read the message for the missing confirmation or flag, check the target environment, then confirm explicitly.",
			DocsPath:   "reference/error-codes#e403",
			Exit:       4,
		},
		CodeEntry{
			Code:       "E404",
			Category:   "cli",
			Summary:    "Command moved to a plugin",
			DefaultWhy: "This command now lives in a plugin that is not installed.",
			DefaultFix: "Run the nself add command printed in the message, then run the command again.",
			DocsPath:   "reference/error-codes#e404",
			Exit:       1,
		},
	)
}
