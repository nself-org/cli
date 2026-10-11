// Registry fragment: bundle remove codes (E133-E134), owned by P7-PLUG-20.
// Registered from init() through Register; see codes.go for the rules and
// codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E133",
			Category:   "plugin",
			Summary:    "Plugin still referenced by a bundle",
			DefaultWhy: "The plugin is a member of an installed bundle (the bundle install ledger lists it under installed_by), so removing it on its own would leave that bundle half installed.",
			DefaultFix: "Remove the bundle that references it (nself bundle remove <bundle>; plugins no other bundle needs and you did not install yourself go with it), or pass --force to remove only this plugin.",
			DocsPath:   "reference/error-codes#e133",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E134",
			Category:   "plugin",
			Summary:    "Bundle not installed",
			DefaultWhy: "The bundle install ledger has no record of this bundle in the plugin directory, so there is nothing to remove: a bundle is recorded only when all of its plugins are installed.",
			DefaultFix: "Run nself bundle list --installed to see what is recorded, or remove single plugins with nself remove <plugin>.",
			DocsPath:   "reference/error-codes#e134",
			Exit:       1,
		},
	)
}
