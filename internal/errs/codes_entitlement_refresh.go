package errs

// Purpose: register the unattended entitlement refresh diagnostic owned by P7-PLUG-64.
// Inputs: the E132 allocation in codes_blocks.go.
// Outputs: a doctor warning (exit 1) when a licensed v1.5 stack has no active refresh timer.
// Constraints: registration stays in this fragment; no duplicate codes.
// SPORT: cap:cli.entitlement-autorefresh.
func init() {
	Register(
		CodeEntry{
			Code: "E132", Category: "plugin", Summary: "No active entitlement refresh timer",
			DefaultWhy: "A licensed plugin is installed but no host timer refreshes the entitlement file, so the plugin stops serving 30 days after the last build, start or manual refresh. The timer is missing, was removed with its project or binary, or the project or nself binary path failed the safety rule (unsafe-path).",
			DefaultFix: "Run nself build to install the timer again, or nself license refresh from your own scheduler. NSELF_ENTITLEMENT_AUTOREFRESH=false turns the timer off on purpose.",
			DocsPath:   "reference/error-codes#e132", Exit: 1,
		},
	)
}
