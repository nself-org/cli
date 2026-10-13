// Registry fragment: plugin release-signature codes (E121-E124), owned by
// P7-PLUG-23. Registered from init() through Register; see codes.go for the
// rules and codes_blocks.go for the allocation. All four exit 1 (user class):
// the normative exit table (TestRegistryExitTable, REG Epic) gives the E100
// range class 1 except E101-E104/E110 (auth) and the infra codes it lists, and
// a refused install is something the user retries or fixes by updating.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E121",
			Category:   "plugin",
			Summary:    "Plugin release signature is invalid",
			DefaultWhy: "The release signature in the registry does not verify for this tarball, or it is malformed. The download may have been tampered with.",
			DefaultFix: "Retry the install. If it keeps failing, report the plugin and version to nself-org/plugins; do not bypass the check.",
			DocsPath:   "reference/error-codes#e121",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E122",
			Category:   "plugin",
			Summary:    "Plugin release signed by an unknown key",
			DefaultWhy: "The release signature names a key id that this CLI does not trust for this plugin tier. Trusted keys are compiled into the CLI.",
			DefaultFix: "Update the CLI (nself update) so it carries the current release keys, then retry the install.",
			DocsPath:   "reference/error-codes#e122",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E123",
			Category:   "plugin",
			Summary:    "Plugin release signed by a revoked key",
			DefaultWhy: "The key that signed this release has been revoked, so the release is no longer trusted.",
			DefaultFix: "Install a newer release of the plugin, or update the CLI (nself update) and retry.",
			DocsPath:   "reference/error-codes#e123",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E124",
			Category:   "plugin",
			Summary:    "Plugin download checksum mismatch",
			DefaultWhy: "The downloaded tarball does not match the checksum published in the registry. Nothing was extracted.",
			DefaultFix: "Retry the install. If the mismatch repeats, the download source may be corrupted or tampered with; report it to nself-org/plugins.",
			DocsPath:   "reference/error-codes#e124",
			Exit:       1,
		},
	)
}
