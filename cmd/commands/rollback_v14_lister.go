package commands

// Purpose: wire the v1.4 deploy and promote rollbacks to promote.RollbackProject
// with a typed --confirm-project and a lister over the local backups.
// Inputs: the rollback command (--confirm-project), the project dir, the target
// ("" when unknown) and the backup tag ("" for the latest promote record's tag).
// Outputs: nil after a restore by id; E403 on a production target without a typed name.
// Constraints: the name is never read from the local config for a production
// restore (restoreconsent rule); a target that cannot be told is treated as production.

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/promote"
	"github.com/spf13/cobra"
)

// v14ListBackups lists local backups; a variable so tests supply entries.
var v14ListBackups = func(workdir string) ([]backup.BackupEntry, error) {
	cfg, err := config.LoadWithOptions(workdir, config.LoadOptions{SkipMinioCredentialGuard: true})
	if err != nil {
		return nil, err
	}
	return backup.List(cfg, backup.ListOptions{})
}

// v14BackupLister adapts backup.List to promote.BackupLister (ID, Tag, date).
func v14BackupLister(workdir string) promote.BackupLister {
	return func() ([]promote.BackupRef, error) {
		entries, err := v14ListBackups(workdir)
		if err != nil {
			return nil, err
		}
		refs := make([]promote.BackupRef, 0, len(entries))
		for _, e := range entries {
			refs = append(refs, promote.BackupRef{ID: e.ID, Tag: e.Tag, Date: e.Date})
		}
		return refs, nil
	}
}

// v14PromoteTarget is the target of the newest promotion record that carries a
// backup tag; "" when none can be read.
func v14PromoteTarget(workdir string) string {
	dir := filepath.Join(workdir, ".nself", "promotions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for i := len(entries) - 1; i >= 0; i-- {
		data, err := os.ReadFile(filepath.Join(dir, entries[i].Name()))
		if err != nil {
			continue
		}
		var rec promote.PromotionRecord
		if json.Unmarshal(data, &rec) == nil && rec.BackupTag != "" {
			return rec.Target
		}
	}
	return ""
}

// rollbackV14 restores the backup tagged tag by id with the typed project.
func rollbackV14(cmd *cobra.Command, workdir, target, tag string) error {
	typed, _ := cmd.Flags().GetString("confirm-project")
	inv, _ := controlplane.Load(workdir)
	prod := target == "" || prodClassFn(inv, target)
	project, err := rollbackProject(typed, workdir, target, true, prod)
	if err != nil {
		return err
	}
	return promote.RollbackProject(cmd.Context(), workdir, project, tag, v14BackupLister(workdir))
}
