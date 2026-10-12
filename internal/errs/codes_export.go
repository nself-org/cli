package errs

// Purpose: register portable-export refusals. Inputs: none. Outputs: E525-E527.
// Constraints: registration errors are recorded by Register, never panics.
func init() {
	Register(
		CodeEntry{Code: "E525", Category: "adopt", Summary: "Not enough free disk for export", DefaultWhy: "Available disk is below the export estimate.", DefaultFix: "Free disk space or choose another --out directory.", DocsPath: "reference/error-codes#e525", Exit: 1},
		CodeEntry{Code: "E526", Category: "adopt", Summary: "Secret found in export", DefaultWhy: "A resolved secret occurs in exported text or SQL. The bundle was removed.", DefaultFix: "Remove the secret from the source data before exporting again.", DocsPath: "reference/error-codes#e526", Exit: 1},
		CodeEntry{Code: "E527", Category: "adopt", Summary: "Export output is not empty", DefaultWhy: "The output exists and is not an empty directory.", DefaultFix: "Choose a new or empty --out directory.", DocsPath: "reference/error-codes#e527", Exit: 1},
	)
}
