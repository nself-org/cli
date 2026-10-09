package errs

// Purpose: register project/CLI compatibility diagnostics.
// Inputs: E060 and E061 allocations. Outputs: registry entries.
// Constraints: E060 warns in doctor; E061 follows config validation mode.
// SPORT: contract:cli.error-codes.

func init() {
	Register(
		CodeEntry{Code: "E060", Category: "config", Summary: "CLI older than project minimum", DefaultWhy: "The project's cli_min_version is newer than this nself CLI.", DefaultFix: "nself update", DocsPath: "reference/error-codes#e060", Exit: 1},
		CodeEntry{Code: "E061", Category: "config", Summary: "Invalid CLI minimum version", DefaultWhy: "cli_min_version in nself.yaml is not semantic version X.Y.Z.", DefaultFix: "Set cli_min_version to a complete semantic version, such as 1.4.0.", DocsPath: "reference/error-codes#e061", Exit: 1},
	)
}
