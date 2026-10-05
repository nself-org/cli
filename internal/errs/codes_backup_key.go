// Registry fragment: zero-config backup key codes (E222-E224, P7-PROD-08).
// Registered from init() through Register; see codes.go for the rules and
// codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E222",
			Category:   "database",
			Summary:    "Backup identity could not be created or read",
			DefaultWhy: "nSelf could not create, secure or read the age identity under ~/.config/nself (missing age-keygen, unwritable directory, a symlink or a file that is not an age identity). An existing identity is never overwritten.",
			DefaultFix: "Install age, fix the directory permissions (0700), or pass --recipient <age1...> and manage the key yourself.",
			DocsPath:   "reference/error-codes#e222",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E223",
			Category:   "database",
			Summary:    "Backup identity missing for decrypt",
			DefaultWhy: "Backups encrypted to an auto-created identity can only be decrypted with that identity file. Without it they are unrecoverable.",
			DefaultFix: "Restore the identity file from your off-host copy to ~/.config/nself/<project>-age.key, or pass --key <file>.",
			DocsPath:   "reference/error-codes#e223",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E224",
			Category:   "database",
			Summary:    "No backup recipient and automatic key creation is disabled",
			DefaultWhy: "NSELF_BACKUP_NO_AUTO_KEY=1 is set, so nSelf will not create an identity, and no recipient is configured. Backups are never written in the clear by default.",
			DefaultFix: "Pass --recipient <age1...|ssh-...|github:user>, set BACKUP_AGE_RECIPIENTS, run nself backup init-key, or pass --no-encrypt to write in the clear.",
			DocsPath:   "reference/error-codes#e224",
			Exit:       2,
		},
	)
}
