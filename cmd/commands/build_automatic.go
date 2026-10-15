package commands

// Purpose: route automatic rebuilds through the same gates as an explicit build.
// Inputs: project directory and build options. Outputs: build result or refusal.
// Constraints: callers cannot implicitly confirm prod changes or hand edits.

import (
	"fmt"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

func runAutomaticBuild(workdir string, opts build.BuildOptions) (*build.BuildResult, error) {
	opts.Force = false
	result, err := runBuildApply(&cobra.Command{Use: "build"}, workdir, opts, buildPlanFlags{}, false, false, false)
	if detail := errs.Describe(err); detail != nil && detail.Code == "E403" {
		return nil, fmt.Errorf("automatic rebuild refused: run 'nself build --yes' (add --force to overwrite hand-edited generated files), then retry: %w", err)
	}
	return result, err
}
