// Registry fragment: project operation lock codes (E460-E464, P7-LIVE-13).
// Registered from init() through Register; see codes.go for the rules and
// codes_blocks.go for the allocation. E461-E464 are reserved for this Ticket's
// owner Epic and stay unregistered until a producer exists.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E460",
			Category:   "reconcile",
			Summary:    "Project operation lock held",
			DefaultWhy: "Another nself command is changing this project and holds its operation lock.",
			DefaultFix: "Wait for the running nself command to finish, or stop it, then run this command again.",
			DocsPath:   "reference/error-codes#e460",
			Exit:       1,
		},
	)
}
