package errs

// Purpose: register the D5 secret resolution and recipient errors.
// Inputs: none. Outputs: E540-E544 registry entries.
// Constraints: fixed descriptions never include secret values.
// SPORT: P7-TRUST-03, block 540-549 secret.
func init() {
	for _, e := range []CodeEntry{
		{Code: "E540", Summary: "Invalid secret reference", DefaultWhy: "The reference, environment name or store name is invalid, or references collide on a NAME.", DefaultFix: "Use project/NAME, team/NAME, release/NAME, environment/<env>/NAME or deploy/<env>/NAME with distinct uppercase names and no reserved environments.", DocsPath: "reference/error-codes#e540", Exit: 1},
		{Code: "E541", Summary: "Secret reference not found", DefaultWhy: "The encrypted file or requested key does not exist.", DefaultFix: "Create the secret with config secrets set in the mapped store using the operator identity.", DocsPath: "reference/error-codes#e541", Exit: 1},
		{Code: "E542", Summary: "Secret resolution requires FIFO stdout", DefaultWhy: "Stdout is not a named or anonymous pipe.", DefaultFix: "Pipe config secrets resolve --json to the secret consumer; do not redirect to a file.", DocsPath: "reference/error-codes#e542", Exit: 1},
		{Code: "E543", Summary: "Age identity unreadable or decryption failed", DefaultWhy: "The identity cannot be read, cannot decrypt this store, or the decrypted store is invalid.", DefaultFix: "Check SECRETS_AGE_KEY_PATH and have the operator grant this identity access to the required file.", DocsPath: "reference/error-codes#e543", Exit: 1},
		{Code: "E544", Summary: "Secret class or recipient change not permitted", DefaultWhy: "Only the store owner may change recipients, the owner cannot be removed, and release or deploy changes require explicit release-class consent.", DefaultFix: "Run recipient changes with the store owner identity, preserve its recipient, and use --allow-release-class only for an eligible dedicated trusted node. Use secrets rekey for owner changes.", DocsPath: "reference/error-codes#e544", Exit: 1},
	} {
		Register(e)
	}
}
