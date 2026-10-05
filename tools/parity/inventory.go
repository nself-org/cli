// Purpose:     list the top-level commands of the v1.5 surface as the parity
//
//	matrix's rows.
//
// Inputs:      the registry of the prepared tree (commands.BuildRegistry).
// Outputs:     one inventoryEntry per visible top-level command, sorted by name.
// Constraints: same selection as tools/cmdinventory's projection at depth 1:
//
//	`help`, hidden commands and canon "plugin" entries are dropped.
//	Subcommands are intentionally ignored: CLI-R17 scores top-level
//	commands only. The tree must already be prepared (prepareV15).
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/nself-org/cli/cmd/commands"
	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
)

// inventoryEntry is one top-level command.
type inventoryEntry struct {
	Name    string
	Path    string
	GroupID string
}

// prepareV15 sets NSELF_V15=1 and prepares RootCmd as the v1.5 surface; the
// returned func undoes the relocation.
func prepareV15() func() {
	_ = os.Setenv("NSELF_V15", "1")
	return commands.PrepareTreeForGeneration()
}

// liveInventory builds the registry of the prepared tree and projects it.
func liveInventory() ([]inventoryEntry, error) {
	reg, err := commands.BuildRegistry(true)
	if err != nil {
		return nil, fmt.Errorf("build registry: %w", err)
	}
	return topLevel(reg), nil
}

// topLevel selects the visible depth-1 commands of reg, sorted by name. A
// command is top level when its parent is not itself a registered command (the
// registry holds no root entry).
func topLevel(reg *cmdregistry.Registry) []inventoryEntry {
	paths := make(map[string]bool, len(reg.Commands))
	for _, c := range reg.Commands {
		paths[c.Path] = true
	}
	var out []inventoryEntry
	for _, c := range reg.Commands {
		if paths[c.Parent] || c.Name == "help" || c.Hidden || c.Canon == canon.CanonPlugin {
			continue
		}
		e := inventoryEntry{Name: c.Name, Path: c.Path}
		if c.Group != nil {
			e.GroupID = *c.Group
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
