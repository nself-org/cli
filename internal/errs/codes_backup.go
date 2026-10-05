// Registry fragment: off-box backup status and restore drill codes (E217-E221,
// P7-PROD-07). Registered from init() through Register; see codes.go for the
// rules and codes_blocks.go for the allocation. E221 is spare.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E217",
			Category:   "database",
			Summary:    "Off-box backup is stale",
			DefaultWhy: "The newest off-box backup heartbeat is older than --max-age, is missing, or reports a failed backup.",
			DefaultFix: "Check the backup schedule on the production host (nself backup schedule, then its systemd journal) and run nself backup stream once by hand.",
			DocsPath:   "reference/error-codes#e217",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E218",
			Category:   "database",
			Summary:    "Restore drill is stale or failed",
			DefaultWhy: "The newest restore drill is older than --max-drill-age, never ran, or its restore did not match the backup.",
			DefaultFix: "Run nself backup drill --from <remote> --identity <age key> --heartbeat-to <remote> from the owner machine and read the mismatches it lists.",
			DocsPath:   "reference/error-codes#e218",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E219",
			Category:   "database",
			Summary:    "Backup heartbeat unreadable",
			DefaultWhy: "The heartbeat object could not be fetched or parsed, so freshness is unknown. Unknown is treated as not OK.",
			DefaultFix: "Check the heartbeat remote and its credentials (--heartbeat-to or NSELF_BACKUP_HEARTBEAT_REMOTE), then read the object with rclone cat.",
			DocsPath:   "reference/error-codes#e219",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E220",
			Category:   "database",
			Summary:    "Restore drill cannot start",
			DefaultWhy: "The drill needs a running Docker daemon, the age binary for encrypted backups, and free disk of twice the download size.",
			DefaultFix: "Start Docker, install age, or free disk space in the temp directory (TMPDIR), then run the drill again.",
			DocsPath:   "reference/error-codes#e220",
			Exit:       2,
		},
	)
}
