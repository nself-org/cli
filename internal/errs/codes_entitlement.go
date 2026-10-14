package errs

// Purpose: register runtime entitlement failures owned by P7-PLUG-24.
// Inputs: the E125-E126 allocation in codes_blocks.go.
// Outputs: unavailable licence JWT exits 3; stale entitlement warning exits 1.
// Constraints: registration stays in this fragment; no duplicate codes.
// SPORT: contract:plugin.entitlement-token v1.
func init() {
	Register(
		CodeEntry{
			Code: "E125", Category: "plugin", Summary: "Licensed plugin entitlement JWT unavailable",
			DefaultWhy: "The CLI could not obtain a verified ping-issued licence JWT covering every licensed plugin within grace. A still-valid runtime token is retained unless rejected or its key was removed; otherwise the runtime guard stays closed.",
			DefaultFix: "Set a valid licence with nself license set, check connectivity to ping.nself.org, and run nself license refresh followed by nself build.",
			DocsPath:   "reference/error-codes#e125", Exit: 3,
		},
		CodeEntry{
			Code: "E126", Category: "plugin", Summary: "Plugin entitlement file is stale",
			DefaultWhy: "The verified entitlement JWT expired more than seven days ago. Licensed plugins stop serving after the thirty-day grace ends.",
			DefaultFix: "Run nself license refresh followed by nself build before the entitlement grace expires.",
			DocsPath:   "reference/error-codes#e126", Exit: 1,
		},
	)
}
