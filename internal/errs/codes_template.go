// Registry fragment: template entitlement code (E137), owned by P7-PLUG-25.
// Registered from init() through Register; see codes.go for the rules and
// codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E137",
			Category:   "plugin",
			Summary:    "Template requires a plugin that is in no registry",
			DefaultWhy: "The clone template lists a required plugin that neither the free nor the licensed plugin catalog publishes, so the project it writes could not run.",
			DefaultFix: "Pick another template, or scaffold without this one and add the plugin yourself once it is published. List what exists with: nself plugin search.",
			DocsPath:   "reference/error-codes#e137",
			Exit:       1,
		},
	)
}
