package errs

func init() {
	Register(
		CodeEntry{Code: "E488", Category: "deploy", Summary: "Promote rolled back", DefaultWhy: "A promotion step failed after the verified snapshot, so the target was restored.", DefaultFix: "Fix the named failing step and retry the promotion.", DocsPath: "reference/error-codes#e488", Exit: 2},
		CodeEntry{Code: "E489", Category: "deploy", Summary: "Promote rollback failed", DefaultWhy: "A compensating action or rollback health check failed, so the target needs operator recovery.", DefaultFix: "Use the snapshot tag <tag> and backup ID <id> named in the error; on the target run nself backup restore <id> --yes, then nself doctor.", DocsPath: "reference/error-codes#e489", Exit: 2},
		CodeEntry{Code: "E490", Category: "deploy", Summary: "Release drift", DefaultWhy: "Source and target per-platform image identities differ or cannot be verified.", DefaultFix: "Rebuild the release so every service resolves to the source manifest digest, then retry.", DocsPath: "reference/error-codes#e490", Exit: 1},
	)
}
