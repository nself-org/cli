package restoreconsent

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNeedsTypedProject refuses a manual rollback that would restore a
// production database without an operator-typed --confirm-project.
var ErrNeedsTypedProject = errors.New("restoring a production database requires --confirm-project <project> (PROJECT_NAME on the target host); --yes alone is not enough")

// RollbackProject is the name a manual deploy or promote rollback passes to
// `backup restore --confirm-project` (C2-01 review). typed is the operator's
// --confirm-project. A production database restore needs it: --yes skips only
// the yes/no prompt, and the name is never read from the local config there
// (the CLI matching its own config against the host is not consent). Elsewhere
// an empty flag falls back to fromConfig; its error fails a restore with the
// real cause. Promote's automatic rollback does not come through here.
func RollbackProject(typed, target string, restore, prod bool, fromConfig func() (string, error)) (string, error) {
	if typed = strings.TrimSpace(typed); typed != "" {
		return typed, nil
	}
	if restore && prod {
		return "", fmt.Errorf("deploy rollback of %s: %w; nothing was changed (run: nself deploy rollback %s --yes --confirm-project <project>)", target, ErrNeedsTypedProject, target)
	}
	project, err := fromConfig()
	if err != nil && restore {
		return "", fmt.Errorf("deploy rollback: read PROJECT_NAME for the database restore: %w; nothing was changed", err)
	}
	return project, nil
}
