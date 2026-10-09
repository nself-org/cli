package errs

// Proxy provider errors belong to the P7-DEPL-01 allocation.
func init() {
	Register(
		CodeEntry{Code: "E480", Category: "deploy", Summary: "Proxy provider plugin missing", DefaultWhy: "The selected proxy provider plugin is not installed.", DefaultFix: "Install the selected proxy provider plugin and retry the build.", DocsPath: "reference/error-codes#e480", Exit: 1},
		CodeEntry{Code: "E481", Category: "deploy", Summary: "Route not expressible by provider", DefaultWhy: "The selected proxy cannot represent one of the project's routes.", DefaultFix: "Use nginx or simplify the route before selecting this provider.", DocsPath: "reference/error-codes#e481", Exit: 1},
		CodeEntry{Code: "E482", Category: "deploy", Summary: "Proxy provider refused in fronted mode", DefaultWhy: "A fronted project does not own the serving proxy.", DefaultFix: "Change the proxy provider in the fronting project.", DocsPath: "reference/error-codes#e482", Exit: 1},
	)
}
