// Registry fragment: plugin manifest v2 codes (E111-E114), owned by P7-PLUG-01.
// Registered from init() through Register; see codes.go for the rules and
// codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E111",
			Category:   "plugin",
			Summary:    "Forbidden key in a v2 plugin manifest",
			DefaultWhy: "A manifest_version 2 plugin.json carries a key that belongs to the registry or to a legacy format (bundles, tier_pair, author_public_key, signature, checksum).",
			DefaultFix: "Remove the key from plugin.json. Bundle membership lives in bundles.json and signatures are added by the release pipeline.",
			DocsPath:   "reference/error-codes#e111",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E112",
			Category:   "plugin",
			Summary:    "Plugin manifest compatibility keys drifted",
			DefaultWhy: "The generated compatibility keys (pluginType, binaryName, cliCommands, minNselfVersion, status, isCommercial, licenseType, requires_license, tier) no longer match the v2 fields they are derived from.",
			DefaultFix: "Regenerate them: go run github.com/nself-org/cli/tools/manifestv2migrate -in plugin.json -compat -write",
			DocsPath:   "reference/error-codes#e112",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E113",
			Category:   "plugin",
			Summary:    "Plugin command collides with a core command",
			DefaultWhy: "commands.command is one of the core command verbs, so the plugin cannot be mounted under it.",
			DefaultFix: "Rename commands.command in plugin.json to a name that is not a core verb.",
			DocsPath:   "reference/error-codes#e113",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E114",
			Category:   "plugin",
			Summary:    "Unsupported plugin manifest version",
			DefaultWhy: "plugin.json declares a manifest_version this nself release does not read (supported: 1 or absent, and 2).",
			DefaultFix: "Set manifest_version to 2, or upgrade nself if the manifest was written for a newer release.",
			DocsPath:   "reference/error-codes#e114",
			Exit:       1,
		},
	)
}
