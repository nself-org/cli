package errs

// Purpose: register the stack executor's resumable failure code.
// Inputs: the P7-PLUG-31 allocation E139. Outputs: registry entry, exit 2.
// Constraints: Register records validation failures and never panics.
func init() {
	Register(CodeEntry{Code: "E139", Category: "plugin", Summary: "Update stack step failed, resumable", DefaultWhy: "A stack upgrade step failed. The journal records the failed step and the verified recovery backup; production recovery awaits the operator.", DefaultFix: "Read .nself/state/update-stack.json and retry nself update stack --yes. If awaiting-operator or rollback-failed, run the exact backup restore command shown, then restart the previous images from the pinned snapshot (.nself/state/update-stack-previous-*.json) with the docker compose command shown, then archive the journal by moving .nself/state/update-stack.json to .nself/state/update-stack.<run_id>.json before starting a new upgrade.", DocsPath: "reference/error-codes#e139", Exit: 2})
}
