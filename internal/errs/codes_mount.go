// Registry fragment: plugin-command mount codes (E405-E407, P7-CANON-01).
// Registered from init() through Register; see codes.go for the rules and
// codes_blocks.go for the allocation (E408-E409 are spare in this fragment).
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E405",
			Category:   "cli",
			Summary:    "Plugin command not mounted (name collision)",
			DefaultWhy: "The plugin's commands.command collides with a core command verb, a cobra-native command or alias, or another installed plugin's command, so it was not mounted.",
			DefaultFix: "Rename commands.command in the plugin's plugin.json to a free name, or remove the other plugin claiming it, then run nself <command> again.",
			DocsPath:   "reference/error-codes#e405",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E406",
			Category:   "cli",
			Summary:    "Mounted plugin binary missing, not executable or outside the plugins dir",
			DefaultWhy: "The plugin declares a commands block, but its binary was not found in the plugins bin dir, is not executable, or resolves (after symlinks) outside the plugins directory.",
			DefaultFix: "Reinstall the plugin with nself add <slug>; if it was installed by hand, put an executable nself-<command> binary in ~/.nself/plugins/bin/.",
			DocsPath:   "reference/error-codes#e406",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E407",
			Category:   "cli",
			Summary:    "Builtin family not mounted",
			DefaultWhy: "This builtin command family is disabled or was removed from the local installation, so its commands are not mounted.",
			DefaultFix: "Run nself add <slug> to restore the builtin family.",
			DocsPath:   "reference/error-codes#e407",
			Exit:       1,
		},
	)
}
