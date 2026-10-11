// Registry fragment: plugin command binary collision code (E141), owned by
// D-0259. Registered from init() through Register; see codes.go for the rules
// and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E141",
			Category:   "plugin",
			Summary:    "Plugin command binary already provided by another plugin",
			DefaultWhy: "The plugin declares a command whose binary (nself-<command>) another installed plugin already publishes. The command proxy runs one binary per command, so installing this plugin would replace the other plugin's command. The message names both plugins and the command.",
			DefaultFix: "Remove the other plugin first (`nself remove <plugin>`) if you mean to replace its command, or install a version of this plugin whose cliCommands do not overlap. Plugin authors: give each command a name unique to the plugin.",
			DocsPath:   "reference/error-codes#e141",
			Exit:       1,
		},
	)
}
