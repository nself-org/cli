package main

// Preparation of the tree this tool documents.
//
// Purpose: every generator documents the v1.5 surface (EPIC P7-CANON D3/D8), so
// the committed artifacts stay truthful as move Tickets land. run() prepares
// the surface itself so an in-process render (the package tests) and the CLI
// walk the identical tree; before the first real moves landed the two paths
// happened to coincide and the split was invisible.
//
// Inputs: none (process environment, the cobra tree).
//
// Outputs: NSELF_V15=1 for the duration and the relocated v1.5 tree; the
// returned func undoes the relocation and restores the previous environment,
// so an in-process render cannot leak the mode into its caller.
//
// Constraints: compat.V15 reads the environment on every call, so the variable
// is set before the tree is prepared. Installed plugins are never mounted.

import (
	"os"

	"github.com/nself-org/cli/cmd/commands"
)

// prepareV15 sets NSELF_V15=1 and prepares RootCmd as the v1.5 surface.
func prepareV15() func() {
	prev, had := os.LookupEnv("NSELF_V15")
	_ = os.Setenv("NSELF_V15", "1")
	undoTree := commands.PrepareTreeForGeneration()
	return func() {
		undoTree()
		if had {
			_ = os.Setenv("NSELF_V15", prev)
			return
		}
		_ = os.Unsetenv("NSELF_V15")
	}
}
