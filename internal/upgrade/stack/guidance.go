package stack

// Purpose: build the operator fix text for failed, blocked and untrusted stack runs.
// Inputs: options, journal and the failure cause. Outputs: exact shell guidance.
// Constraints: a restore command is shown only for a backup this run proved and only
// when the database may have changed; nothing here runs a command.

import (
	"errors"
	"fmt"
	"strings"
)

// shellQuote single-quotes one shell word so metacharacters cannot execute.
func quoteShellWord(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

// RestoreCommand quotes an exact retained backup id for an operator shell command.
// Inputs: backup id. Outputs: executable restore guidance.
// Constraints: shell metacharacters in backup identifiers cannot execute commands.
func RestoreCommand(backupID string) string {
	return "nself backup restore " + quoteShellWord(backupID)
}

// VerifyCommand quotes the command that re-checks a retained backup.
// Inputs: backup id. Outputs: executable verify guidance.
func VerifyCommand(backupID string) string { return "nself backup verify " + quoteShellWord(backupID) }

// untrustedBackupFix names verification and journal archival without offering an unverified restore.
// Inputs: retained journal evidence. Outputs: operator reconciliation guidance.
// Constraints: used when resume cannot prove the original backup is still usable.
func untrustedBackupFix(j *Journal) string {
	return fmt.Sprintf("The step-one backup can no longer be trusted. Check it with: %s. Restore only a backup that verifies. To leave this state, either verify and restore by hand, or archive the journal by moving %s to %s.", VerifyCommand(j.BackupID), ".nself/state/update-stack.json", ".nself/state/update-stack."+j.RunID+".json")
}

// RestartCommand restarts the pinned previous images and removes orphans, never volumes.
// Inputs: snapshot path. Outputs: executable compose command.
func RestartCommand(snapshot string) string {
	return "docker compose -f " + quoteShellWord(snapshot) + " up -d --remove-orphans"
}

// evidenceReset reports that the step-one backup evidence was cleared after a failure.
// Inputs: the unusable backup id and the cause. Outputs: an error carrying both.
// Constraints: the backup file itself is never deleted.
type evidenceReset struct {
	id    string
	cause error
}

// Error returns the underlying failure.
func (e *evidenceReset) Error() string { return e.cause.Error() }

// Unwrap exposes the cause.
func (e *evidenceReset) Unwrap() error { return e.cause }

// stateMayHaveChanged reports whether start or migrate began, so the database may differ.
// Inputs: journal. Outputs: true once either step left the pending state.
// Constraints: pull and build never touch the database.
func stateMayHaveChanged(j *Journal) bool {
	for _, s := range j.Steps {
		if (s.Name == "start" || s.Name == "migrate") && s.Result != "pending" {
			return true
		}
	}
	return false
}

// recoveryFix gives the restore command, then the snapshot path and restart command.
// Inputs: journal with backup and snapshot evidence. Outputs: ordered operator steps.
// Constraints: the snapshot is the only immutable record of the old images.
func recoveryFix(j *Journal) string {
	parts := []string{"1. Restore the step-one backup: " + RestoreCommand(j.BackupID)}
	if j.PreviousCompose != "" {
		parts = append(parts, fmt.Sprintf("2. Restart the previous images from the pinned snapshot %s: %s", j.PreviousCompose, RestartCommand(j.PreviousCompose)))
	}
	return strings.Join(parts, "; ")
}

// blockedFix explains the reconciliation a blocked recovery journal needs.
// Inputs: journal. Outputs: guidance naming the backup, snapshot and archive step.
func blockedFix(j *Journal) string {
	msg := "Check that restoring the step-one backup, restarting the previous images, and per-table row counts all succeeded; then archive .nself/state/update-stack.json before starting a new update."
	if j.BackupID != "" {
		msg += " " + recoveryFix(j)
	}
	return msg
}

// failureFix picks the guidance for a failed step.
// Inputs: options, journal and cause. Outputs: fix text, or empty for none.
// Constraints: production restore guidance needs a verified step one and a changed database.
func failureFix(o Options, j *Journal, cause error) string {
	var reset *evidenceReset
	if errors.As(cause, &reset) {
		return fmt.Sprintf("Backup %s failed verification and can no longer be trusted. It stays on disk, but the journal evidence was cleared so the next `nself update stack --yes` takes a fresh dump. Inspect it with: %s", reset.id, VerifyCommand(reset.id))
	}
	if j.BackupID == "" {
		return ""
	}
	if !o.Prod {
		if j.Status != "rollback-failed" {
			return ""
		}
		var missing *imagesMissingError
		if errors.As(cause, &missing) {
			return "The previous images are missing, so nothing was stopped or restored and the database is untouched. Restore the images named in the snapshot, then: " + recoveryFix(j)
		}
		return recoveryFix(j)
	}
	if len(j.Steps) == 0 || j.Steps[0].Result != "ok" {
		return ""
	}
	if !stateMayHaveChanged(j) {
		return "Nothing was changed: the old stack is still running. Retry with `nself update stack --yes`. Do not restore the backup."
	}
	return recoveryFix(j) + "; " + archiveAfterRestoreFix(j)
}

// archiveAfterRestoreFix tells a production operator to archive the journal after a manual restore.
// Inputs: journal. Outputs: the archive step and the retry alternative.
// Constraints: a plain retry resumes the failed step against whatever stack is running.
func archiveAfterRestoreFix(j *Journal) string {
	return fmt.Sprintf("3. After restoring, archive the journal before any new update: move .nself/state/update-stack.json to .nself/state/update-stack.%s.json. A retry without archiving resumes this run at the failed step, so retry `nself update stack --yes` only if you fixed the cause and did not restore.", j.RunID)
}
