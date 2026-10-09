package errs

func init() {
	Register(CodeEntry{
		Code:       "E491",
		Category:   "deploy",
		Summary:    "Deploy image architecture unavailable",
		DefaultWhy: "At least one image has no proven manifest for a target host architecture.",
		DefaultFix: "Publish the missing platform or use a compatible image, then rebuild and retry.",
		DocsPath:   "reference/error-codes#e491",
		Exit:       1,
	})
}
