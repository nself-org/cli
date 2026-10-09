package main

// Registry lookup for the v1.5 wiki surface.
// Purpose: keep every generated command fact tied to the public registry.
// Inputs: the prepared, builtin-only Cobra tree.
// Outputs: entries indexed by their full command path.
// Constraints: callers undo tree preparation after rendering.

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/cmd/commands"
	"github.com/nself-org/cli/internal/cmdregistry"
)

func prepareWikiRegistry() (map[string]cmdregistry.Command, func(), error) {
	previous, had := os.LookupEnv("NSELF_V15")
	if err := os.Setenv("NSELF_V15", "1"); err != nil {
		return nil, nil, err
	}
	undoTree := commands.PrepareTreeForGeneration()
	undo := func() {
		undoTree()
		if had {
			_ = os.Setenv("NSELF_V15", previous)
		} else {
			_ = os.Unsetenv("NSELF_V15")
		}
	}
	reg, err := commands.BuildRegistry(true)
	if err != nil {
		undo()
		return nil, nil, fmt.Errorf("build wiki registry: %w", err)
	}
	return indexRegistry(reg.Commands), undo, nil
}

func indexRegistry(commands []cmdregistry.Command) map[string]cmdregistry.Command {
	entries := make(map[string]cmdregistry.Command, len(commands))
	for _, command := range commands {
		entries[command.Path] = command
	}
	return entries
}
