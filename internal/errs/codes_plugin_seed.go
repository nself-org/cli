// Registry fragment: plugin seed hook code (E127), owned by P7-PLUG-32.
// Registered from init() through Register; see codes.go for the rules and
// codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E127",
			Category:   "plugin",
			Summary:    "Plugin declares no seed command",
			DefaultWhy: "The plugin's manifest has no seed.command, so `nself db seed --plugin` has nothing to run inside its container.",
			DefaultFix: "Ask the plugin author to declare an idempotent seed argv under \"seed\": {\"command\": [...]} in plugin.json, or seed the data another way.",
			DocsPath:   "reference/error-codes#e127",
			Exit:       1,
		},
	)
}
