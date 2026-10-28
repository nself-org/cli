// Registry fragment: machine invocation gate and machine token codes
// (E424-E426, P7-SURF-25, EPIC SURF D14). Registered from init() through
// Register; see codes.go for the rules and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E424",
			Category:   "cli",
			Summary:    "Confirmation does not match",
			DefaultWhy: "The confirm value is not a live server-issued nonce or plan id for this exact request: it was reused, expired, unknown, computed by the client, or issued for a different request.",
			DefaultFix: "Send the request again without confirm, read the confirmation it returns, and repeat the identical request with that confirm value.",
			DocsPath:   "reference/error-codes#e424",
			Exit:       ExitDestructiveBlocked,
		},
		CodeEntry{
			Code:       "E425",
			Category:   "cli",
			Summary:    "Consent or plan-id flag set by the caller",
			DefaultWhy: "Consent flags (such as --yes or --force) and plan-id flags belong to the invoker; a machine request cannot set them.",
			DefaultFix: "Remove the flag from the request and use the confirmation round trip: the server adds it once you confirm.",
			DocsPath:   "reference/error-codes#e425",
			Exit:       ExitUserError,
		},
		CodeEntry{
			Code:       "E426",
			Category:   "cli",
			Summary:    "Machine token missing, invalid or insecure",
			DefaultWhy: "The machine surface needs a bearer token it can trust: the token file is unreadable, readable by others, owned by another effective user (including sudo), or in a writable parent directory; or a non-loopback bind lacks --allowed-host and a configured token.",
			DefaultFix: "Run as the token file owner; set NSELF_MCP_TOKEN or keep ~/.nself/mcp-token at mode 0600 and run chmod go-w ~/.nself; for a non-loopback --host also pass --allowed-host <name>.",
			DocsPath:   "reference/error-codes#e426",
			Exit:       ExitAuthError,
		},
	)
}
