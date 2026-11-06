package backup

import "github.com/nself-org/cli/internal/config"

// RestoreProject is PROJECT_NAME of the project at workdir, the name deploy
// and promote rollback pass to `backup restore --confirm-project` (C2-01).
// "" when the config cannot be read: the restore then refuses before any
// host change.
func RestoreProject(workdir string) string {
	cfg, err := config.LoadWithOptions(workdir, config.LoadOptions{})
	if err != nil {
		return ""
	}
	return cfg.ProjectName
}
