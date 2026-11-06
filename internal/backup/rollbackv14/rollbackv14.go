// Package rollbackv14 restores a v1.4 deploy or promote rollback by backup id
// with an operator-typed project name (P7-WIRE-26).
package rollbackv14

import (
	"context"
	"errors"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/promote"
	"github.com/nself-org/cli/internal/restoreconsent"
)

// ListBackups lists the project's local backups as promote refs; tests replace it.
var ListBackups = func(workdir string) (refs []promote.BackupRef, err error) {
	cfg, err := config.LoadWithOptions(workdir, config.LoadOptions{SkipMinioCredentialGuard: true})
	if err != nil {
		return nil, err
	}
	entries, err := backup.List(cfg, backup.ListOptions{})
	for _, e := range entries {
		refs = append(refs, promote.BackupRef{ID: e.ID, Tag: e.Tag, Date: e.Date})
	}
	return refs, err
}

// Run restores the newest backup tagged tag (the latest promote record's tag
// when empty). On production a typed project is required (E403 before any
// restore) and the name is never read from the local config; elsewhere the
// config name is the fallback.
func Run(ctx context.Context, workdir, typed, target string, prod bool, tag string) error {
	project, err := restoreconsent.RollbackProject(typed, target, true, prod, func() (string, error) { return backup.RestoreProject(workdir) })
	if errors.Is(err, restoreconsent.ErrNeedsTypedProject) {
		err = errs.New("E403", err.Error())
	}
	if err != nil {
		return err
	}
	return promote.RollbackProject(ctx, workdir, project, tag, func() ([]promote.BackupRef, error) { return ListBackups(workdir) })
}
