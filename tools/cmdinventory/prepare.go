package main

// Preparation of the tree this tool documents.
//
// Purpose: every generator documents the v1.5 surface (EPIC P7-CANON D3/D8), so
// the committed artifacts stay truthful as move Tickets land.
//
// Inputs: none (process environment, the cobra tree).
//
// Outputs: NSELF_V15=1 for this process and the relocated v1.5 tree; the
// returned func undoes the relocation.
//
// Constraints: compat.V15 reads the environment on every call, so the variable
// is set before the tree is prepared. Installed plugins are never mounted.

import (
	"os"

	"github.com/nself-org/cli/cmd/commands"
)

// prepareV15 sets NSELF_V15=1 and prepares RootCmd as the v1.5 surface.
func prepareV15() func() {
	_ = os.Setenv("NSELF_V15", "1")
	return commands.PrepareTreeForGeneration()
}
