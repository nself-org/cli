// Registry fragment: portable bundle codes (E515-E516, P7-ADOPT-23).
// Registered from init() through Register; see codes.go for the rules and
// codes_blocks.go for the allocation. The reader in internal/portable is the
// only producer.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E515",
			Category:   "adopt",
			Summary:    "Unknown portable bundle format or major version",
			DefaultWhy: "The bundle manifest is not a version 1 nself portable export, so this nself cannot read it safely.",
			DefaultFix: "Re-export the bundle with a matching nself release, or upgrade nself if the bundle comes from a newer one.",
			DocsPath:   "reference/error-codes#e515",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E516",
			Category:   "adopt",
			Summary:    "Portable bundle failed its integrity check",
			DefaultWhy: "A bundle file is missing, changed, truncated, unlisted or has an unsafe name, so the bundle cannot be trusted.",
			DefaultFix: "Export the bundle again from the source and keep it unmodified, then re-run the command.",
			DocsPath:   "reference/error-codes#e516",
			Exit:       1,
		},
	)
}
