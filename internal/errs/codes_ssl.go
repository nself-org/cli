// Registry fragment: ssl codes (E150-E199). Registered from init() through
// Register; see codes.go for the rules and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E150",
			Category:   "ssl",
			Summary:    "mkcert not installed",
			DefaultWhy: "mkcert is not installed; falling back to OpenSSL self-signed certs.",
			DefaultFix: "Install mkcert: brew install mkcert (macOS) or see https://github.com/FiloSottile/mkcert",
			DocsPath:   "reference/error-codes#e150",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E151",
			Category:   "ssl",
			Summary:    "SSL certificate generation failed",
			DefaultWhy: "Could not generate SSL certificates for the configured domain.",
			DefaultFix: "Check domain configuration and ensure openssl is available.",
			DocsPath:   "reference/error-codes#e151",
			Exit:       2,
		},
	)
}
