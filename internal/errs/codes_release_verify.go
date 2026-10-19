package errs

// Purpose: error codes for release-manifest verification before a promotion
// changes any target (contract:ci.release-manifest, P7-DEPL-19).
// Constraints: E493 and E494 are auth-class (exit 3); the rest exit 1.

func init() {
	Register(
		CodeEntry{Code: "E492", Category: "deploy", Summary: "Release manifest invalid", DefaultWhy: "The release manifest is missing, malformed, or from a schema major this nself does not read.", DefaultFix: "Rebuild the release with a matching nself ci, then run the promotion with --release <dir>.", DocsPath: "reference/error-codes#e492", Exit: 1},
		CodeEntry{Code: "E493", Category: "deploy", Summary: "Release signature or key rejected", DefaultWhy: "The release manifest is not a ci-release DSSE envelope signed by a key this project trusts (wrong payload type, unknown key id, key of another purpose, bad signature, or unreadable trust files).", DefaultFix: "Add the coordinator's public key to .nself/trust/ci-release.keys on the default branch (or ~/.nself/ci/trust/ci-release.keys), then retry.", DocsPath: "reference/error-codes#e493", Exit: 3},
		CodeEntry{Code: "E494", Category: "deploy", Summary: "Release signing key revoked", DefaultWhy: "The key that signed the release manifest is listed in ci-release.revoked.", DefaultFix: "Re-sign the release with a current coordinator key; never remove a revocation to proceed.", DocsPath: "reference/error-codes#e494", Exit: 3},
		CodeEntry{Code: "E495", Category: "deploy", Summary: "Image digest not in release manifest", DefaultWhy: "A per-platform image digest the promotion would deploy is not listed in the signed release manifest.", DefaultFix: "Promote only images the release produced, or build a new release that lists them.", DocsPath: "reference/error-codes#e495", Exit: 1},
		CodeEntry{Code: "E496", Category: "deploy", Summary: "Release revision mismatch", DefaultWhy: "The revision in the release manifest differs from the revision in the evidence beside it.", DefaultFix: "Use the manifest and evidence of one release run; do not mix directories.", DocsPath: "reference/error-codes#e496", Exit: 1},
		CodeEntry{Code: "E497", Category: "deploy", Summary: "Release evidence not acceptable", DefaultWhy: "The ci plugin reports the evidence is not current, bound, passing and release-class at the manifest revision, or could not judge it.", DefaultFix: "Run nself ci evidence verify --file <dir>/evidence.json --at-revision <revision> and fix what it reports.", DocsPath: "reference/error-codes#e497", Exit: 1},
		CodeEntry{Code: "E498", Category: "deploy", Summary: "Release quarantined", DefaultWhy: "The ci plugin reports the release evidence as quarantined.", DefaultFix: "Wait for the release to be cleared by the coordinator, or promote a different release.", DocsPath: "reference/error-codes#e498", Exit: 1},
		CodeEntry{Code: "E499", Category: "deploy", Summary: "Verified release required", DefaultWhy: "This target requires a verified release, but --release was not given or the ci plugin is not installed.", DefaultFix: "Install the ci plugin (nself add ci) and pass --release <dir> to deploy promote.", DocsPath: "reference/error-codes#e499", Exit: 1},
	)
}
