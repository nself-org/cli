package errs

func init() {
	Register(
		CodeEntry{Code: "E484", Category: "deploy", Summary: "Invalid deploy host", DefaultWhy: "The host does not match the deploy host grammar.", DefaultFix: "Use [user@]host[:port] and set remote_path separately.", DocsPath: "reference/error-codes#e484", Exit: 1},
		CodeEntry{Code: "E485", Category: "deploy", Summary: "Invalid deploy inventory", DefaultWhy: "The control-plane inventory violates a required field or invariant.", DefaultFix: "Correct the named field in .nself/control-plane.yaml.", DocsPath: "reference/error-codes#e485", Exit: 1},
		CodeEntry{Code: "E486", Category: "deploy", Summary: "No deploy target matched", DefaultWhy: "The selector matched no server.", DefaultFix: "Run nself deploy targets to list available targets.", DocsPath: "reference/error-codes#e486", Exit: 1},
		CodeEntry{Code: "E487", Category: "deploy", Summary: "Deploy host key is unknown", DefaultWhy: "A secret-bearing operation requires a trusted host key.", DefaultFix: "Verify the fingerprint and enrol it with nself env target add --trust-host-key.", DocsPath: "reference/error-codes#e487", Exit: 1},
	)
}
