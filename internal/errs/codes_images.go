package errs

// Image availability codes. E466-E469 are reserved for P7-LIVE-18.
func init() {
	Register(CodeEntry{
		Code:       "E465",
		Category:   "reconcile",
		Summary:    "Image unavailable",
		DefaultWhy: "The locked image could not be pulled from its upstream or digest-identical mirror.",
		DefaultFix: "Run nself doctor images, then check registry access and the locked digest.",
		DocsPath:   "reference/error-codes#e465",
		Exit:       1,
	})
}
