// Registry fragment: plugin catalog codes (E115-E117), owned by P7-PLUG-09.
// Registered from init() through Register; see codes.go for the rules and
// codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E115",
			Category:   "plugin",
			Summary:    "Plugin catalog unavailable",
			DefaultWhy: "The CLI could not fetch https://plugins.nself.org/catalog.json and has no cached copy to fall back on.",
			DefaultFix: "Check the network connection and run the command again; once one fetch succeeds the catalog is cached under ~/.nself/cache/plugins and later runs work offline.",
			DocsPath:   "reference/error-codes#e115",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E116",
			Category:   "plugin",
			Summary:    "Plugin catalog invalid",
			DefaultWhy: "The catalog document does not parse as catalog schema 1: it is not JSON, has another schema_version, lists no plugins, or a plugin row is malformed. An invalid document is never cached.",
			DefaultFix: "Run the command again in a few minutes; if it keeps failing, update the CLI and report the problem at https://github.com/nself-org/plugins/issues.",
			DocsPath:   "reference/error-codes#e116",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E117",
			Category:   "plugin",
			Summary:    "Command provided by an uninstalled plugin",
			DefaultWhy: "The command is not built in and no installed plugin provides it, but the cached plugin catalog lists a plugin that does.",
			DefaultFix: "Install the plugin named in the message with nself add <plugin>, then run the command again.",
			DocsPath:   "reference/error-codes#e117",
			Exit:       1,
		},
	)
}
