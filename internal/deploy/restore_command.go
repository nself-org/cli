package deploy

// Purpose: the one `nself backup restore` command line deploy rollback and
// promote rollback run over ssh (C2-01).
// Inputs: a backup id and the project name the operator already confirmed.
// Outputs: a shell-quoted command, or an error when the project is unknown.
// Constraints: remote runs have no stdin, and `backup restore` refuses bare
// --yes on production, so the command always carries --confirm-project, which
// the remote checks against its own PROJECT_NAME. --yes is kept for a remote
// on an older CLI, which then fails loudly on the unknown flag.

import (
	"fmt"
	"strings"
)

// BackupRestoreCommand returns `nself backup restore <id> --yes
// --confirm-project <project>` with both values shell-quoted.
func BackupRestoreCommand(backupID, project string) (string, error) {
	if strings.TrimSpace(project) == "" {
		return "", fmt.Errorf("restoring backup %s needs the project name for --confirm-project; nothing was restored", backupID)
	}
	return fmt.Sprintf("nself backup restore %s --yes --confirm-project %s", q(backupID), q(project)), nil
}

// BackupRestoreHint is the command an operator runs by hand on the backup
// host. An unknown project prints the <project> placeholder.
func BackupRestoreHint(backupID, project string) string {
	if strings.TrimSpace(project) == "" {
		return fmt.Sprintf("nself backup restore %s --confirm-project <project>", q(backupID))
	}
	return fmt.Sprintf("nself backup restore %s --confirm-project %s", q(backupID), q(project))
}
