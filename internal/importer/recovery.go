package importer

// Purpose: tell the user where the report is and what to do after a failed import.
// Inputs: the run error, the run id and whether the target was written. Outputs: the same
// error with a fix hint added.
// Constraints: the hint holds a project-relative path and command names only, never a secret.
import (
	"errors"
	"path/filepath"

	"github.com/nself-org/cli/internal/errs"
)

// recoveryHint is the real reset sequence of this CLI: `db reset` drops and recreates the
// project database, and a stop/start lets Hasura and hasura-auth initialise it again.
const recoveryHint = "The target was written and now holds partial or unverified data; a rerun is refused with E519. " +
	"To import again, reset the target project database: run `nself db reset --yes`, then `nself stop` and `nself start`."

const untouchedHint = "Nothing was written to the target. Fix the cause named above and rerun the import; no reset is needed."

// withReportHint adds the report.json path to E521, E522 and E523, then either the recovery
// sequence (wrote) or the statement that nothing was written.
func withReportHint(err error, runID string, wrote bool) error {
	var cli *errs.CLIError
	if err == nil || runID == "" || !errors.As(err, &cli) {
		return err
	}
	switch cli.Code {
	case "E521", "E522", "E523":
		hint := untouchedHint
		if wrote {
			hint = recoveryHint
		}
		return cli.WithFix("Details: " + filepath.Join(".nself", "import", runID, "report.json") + ". " + hint)
	}
	return err
}
