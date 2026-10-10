// Registry fragment: ACME certificate codes (E470-E474, P7-LIVE-22). Registered
// from init() through Register; see codes.go for the rules and codes_blocks.go
// for the allocation. E473 and E474 stay unregistered until a producer exists.
// The commands raise these only in v1.5 mode (ADR 0021); v1.4 keeps E151.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E470",
			Category:   "reconcile",
			Summary:    "ACME issuance failed",
			DefaultWhy: "The ACME client could not obtain a certificate from the certificate authority.",
			DefaultFix: "Check that the names resolve to this server and that port 80 or the DNS credential is reachable, then run nself trust ssl renew --dry-run.",
			DocsPath:   "reference/error-codes#e470",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E471",
			Category:   "reconcile",
			Summary:    "Certificate install or verification failed",
			DefaultWhy: "The new certificate was issued but could not be installed, or nginx did not serve it after the reload. The previous certificate was restored.",
			DefaultFix: "Run nginx -t in the nginx container, fix the reported error, then run nself trust ssl renew --force.",
			DocsPath:   "reference/error-codes#e471",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E472",
			Category:   "reconcile",
			Summary:    "DNS credential missing",
			DefaultWhy: "A DNS-01 certificate needs the DNS provider credential, and it is not in the secret store.",
			DefaultFix: "Store the credential with nself secrets set <NAME> <value>, then run the command again.",
			DocsPath:   "reference/error-codes#e472",
			Exit:       1,
		},
	)
}
