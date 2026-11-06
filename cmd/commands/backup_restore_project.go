package commands

// Purpose: the project name deploy rollback and promote rollback pass to the
// remote `nself backup restore --confirm-project` (C2-01).
// Inputs: the local project root. Outputs: PROJECT_NAME, or "" when the
// config cannot be read.
// Constraints: "" makes the restore refuse before any host change; a name the
// remote does not share makes the remote restore refuse. Neither restores.

import "github.com/nself-org/cli/internal/config"

// restoreProjectName reads PROJECT_NAME from the project at workdir.
func restoreProjectName(workdir string) string {
	cfg, err := config.LoadWithOptions(workdir, config.LoadOptions{})
	if err != nil {
		return ""
	}
	return cfg.ProjectName
}
