package backup

import "github.com/nself-org/cli/internal/config"

// RestoreProject is PROJECT_NAME of the project at workdir, the name promote
// and a non-production deploy rollback pass to `backup restore
// --confirm-project` (C2-01). An unreadable config is returned as the error,
// so the operator sees the real cause, not a missing project name.
func RestoreProject(workdir string) (string, error) {
	cfg, err := config.LoadWithOptions(workdir, config.LoadOptions{})
	if err != nil {
		return "", err
	}
	return cfg.ProjectName, nil
}
