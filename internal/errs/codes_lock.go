// Registry fragment: project lock codes (E455-E459, P7-LIVE-09). E457-E459 are
// reserved for this Ticket's owner Epic and stay unregistered until a producer
// exists. Registered from init() through Register; see codes.go for the rules
// and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E455",
			Category:   "reconcile",
			Summary:    "Stack does not match nself.lock.yaml",
			DefaultWhy: "nself start --frozen found a service image, an installed plugin version or checksum, or the CLI minor that differs from the project lock, or the project has no lock.",
			DefaultFix: "Run nself build to regenerate the stack from the lock, or accept the change by running nself build --write-lock and committing the new nself.lock.yaml.",
			DocsPath:   "reference/error-codes#e455",
			Exit:       1,
		},
		CodeEntry{
			Code: "E456", Category: "reconcile", Summary: "Update plan source is unavailable",
			DefaultWhy: "The CLI release lookup or plugin registry could not answer the update plan request.",
			DefaultFix: "Retry in a few minutes. Nothing was changed.", DocsPath: "reference/error-codes#e456", Exit: 1,
		},
	)
}
