package errs

// Purpose: register the stack executor's resumable failure code.
// Inputs: the P7-PLUG-31 allocation E139. Outputs: registry entry, exit 2.
// Constraints: Register records validation failures and never panics.
func init() {
	Register(CodeEntry{Code: "E139", Category: "plugin", Summary: "Update stack step failed, resumable", DefaultWhy: "A stack upgrade step failed. The journal records the failed step and the verified recovery backup; production recovery awaits the operator.", DefaultFix: "Read .nself/state/update-stack.json and retry nself update stack --yes. If awaiting-operator, run the exact backup restore command shown and recover the previous images before starting a new upgrade.", DocsPath: "reference/error-codes#e139", Exit: 2})
}
