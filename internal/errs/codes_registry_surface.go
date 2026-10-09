package errs

// Purpose: register the warning code for a malformed installed-plugin surface.
// Inputs: registry validation of mounted plugin annotations.
// Outputs: E437 metadata for stderr and error-code documentation.
// Constraints: invalid plugin declarations never block the core registry.
func init() {
	Register(CodeEntry{
		Code: "E437", Category: "cli", Summary: "Plugin registry surface declaration invalid",
		DefaultWhy: "A mounted plugin declares invalid confirmation or surface fields.",
		DefaultFix: "Fix the plugin manifest confirm flags and surface declaration, then reinstall it.",
		DocsPath:   "reference/error-codes#e437", Exit: 1,
	})
}
