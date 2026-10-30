package commands

// Purpose: `nself db migrate status --detect` — classify each pending
// migration against the live schema (BASELINE/APPLY/CONFLICT) instead of
// only reporting the ledger. See internal/database/migrate_detect*.go for the
// classification logic. This file only prints, and for BASELINE-classified
// migrations prints the exact `db migrate baseline` invocation that would
// record them — baseline is never run automatically as a side effect.
// Inputs: the loaded *config.Config and the resolved migrations directory.
// Outputs: printed table + suggestions; a non-nil error only on a detection
// failure (e.g. cannot reach the database).
// Constraints: CONFLICT migrations are always listed separately and are
// never included in the suggested baseline command.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// detectMigrateStatusData is the v1.5 envelope payload for `db migrate status --detect`.
// The bare shape is the results slice; the envelope wraps it so machine callers
// get typed data instead of null (D-0435).
type detectMigrateStatusData struct {
	Migrations []database.DetectResult `json:"migrations"`
}

func runDBMigrateStatusDetect(cmd *cobra.Command, cfg *config.Config, migrationDir string) error {
	results, err := database.DetectMigrations(cmd.Context(), cfg, migrationDir)
	if err != nil {
		return fmt.Errorf("detect migrations: %w", err)
	}
	// D-0435: set envelope data before any output so --json callers get typed data.
	setData(cmd, detectMigrateStatusData{Migrations: results})
	if len(results) == 0 {
		if jsonEnvelopeOn(cmd) {
			return emitEnv(cmd, detectMigrateStatusData{Migrations: []database.DetectResult{}}, false)
		}
		fmt.Println("No pending migrations to classify.")
		return nil
	}

	tbl := ui.NewTable("MIGRATION", "CLASS", "PRESENT", "MISSING", "LINT")
	var baselineCandidates, conflicts []string
	for _, r := range results {
		lint := "-"
		if r.Lint != nil {
			lint = fmt.Sprintf("FAIL (%d)", r.Lint.Count)
		}
		tbl.AddRow(r.Name, string(r.Class), objectCountLabel(r.Present), objectCountLabel(r.Missing), lint)
		switch r.Class {
		case database.DetectBaseline:
			baselineCandidates = append(baselineCandidates, r.Name)
		case database.DetectConflict:
			conflicts = append(conflicts, r.Name)
		}
	}
	// In envelope mode the table is suppressed: the JSON carries the same data.
	if !jsonEnvelopeOn(cmd) {
		tbl.Render()
	}

	if len(conflicts) > 0 {
		if !jsonEnvelopeOn(cmd) {
			fmt.Println()
			ui.Warn(fmt.Sprintf(
				"%d migration(s) are CONFLICT: some of their objects exist, some don't. "+
					"Not auto-resolved — review each manually.", len(conflicts)))
			for _, name := range conflicts {
				fmt.Printf("  - %s\n", name)
			}
		}
	}

	if len(baselineCandidates) > 0 {
		if !jsonEnvelopeOn(cmd) {
			fmt.Println()
			ui.Info("Every object these migrations create already exists. Review, then run:")
			fmt.Printf("  nself db migrate baseline %s --yes\n", strings.Join(baselineCandidates, " "))
		}
	}

	if jsonEnvelopeOn(cmd) {
		return emitEnv(cmd, detectMigrateStatusData{Migrations: results}, false)
	}
	return nil
}

// objectCountLabel renders an object list as a compact count for the table,
// or "-" when there are none.
func objectCountLabel(objs []database.ObjectRef) string {
	if len(objs) == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", len(objs))
}
