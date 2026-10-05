package main

// Inventory projection of the command registry.
//
// Purpose: .github/command-inventory.json, the wiki command table and the
// parity matrix keep their exact historical shape and bytes, but they are now
// derived from the registry (internal/cmdregistry) instead of a second cobra
// tree walker, so there is one source for every command fact.
//
// Inputs: *cmdregistry.Registry built from the live tree.
//
// Outputs: the nested Command tree the json, names and markdown formats print.
//
// Constraints: byte compatibility with the former walker. Siblings are sorted
// by name, `help` is skipped at every level (it is not part of the inventory
// or the surface budget), hidden commands drop their whole subtree unless
// includeHidden, flags are the command's own flags as "--name" sorted, and
// aliases keep the registry's sorted order, and canon "plugin" entries (a mounted
// plugin command) are dropped with their subtree: core does not document them (the real tree has no command
// whose declared alias order differs; TestProjectionMatchesCommittedInventory
// pins the bytes).

import (
	"sort"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
)

// Command is one node of the inventory.
type Command struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Short       string    `json:"short"`
	Hidden      bool      `json:"hidden"`
	Deprecated  string    `json:"deprecated,omitempty"`
	GroupID     string    `json:"group_id,omitempty"`
	Aliases     []string  `json:"aliases,omitempty"`
	Flags       []string  `json:"flags,omitempty"`
	Subcommands []Command `json:"subcommands,omitempty"`
}

// project derives the inventory tree below the root to the requested depth
// (1 = top level only; depth <= 0 yields nothing).
func project(reg *cmdregistry.Registry, depth int, includeHidden bool) []Command {
	children := map[string][]*cmdregistry.Command{}
	for i := range reg.Commands {
		c := &reg.Commands[i]
		children[c.Parent] = append(children[c.Parent], c)
	}
	return projectLevel(children, rootPath(reg), depth, includeHidden)
}

// rootPath is the parent path of top-level commands: the registry holds no
// root entry, but every top-level command names it as its parent.
func rootPath(reg *cmdregistry.Registry) string {
	for _, c := range reg.Commands {
		if c.Parent != "" && !hasCommandPath(reg, c.Parent) {
			return c.Parent
		}
	}
	return ""
}

func hasCommandPath(reg *cmdregistry.Registry, path string) bool {
	_, ok := reg.Lookup(path)
	return ok
}

func projectLevel(children map[string][]*cmdregistry.Command, parent string, depth int, includeHidden bool) []Command {
	if depth <= 0 {
		return nil
	}
	var out []Command
	for _, c := range children[parent] {
		if c.Name == "help" || c.Canon == canon.CanonPlugin || (c.Hidden && !includeHidden) {
			continue
		}
		out = append(out, node(c, projectLevel(children, c.Path, depth-1, includeHidden)))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// node maps one registry command onto the inventory shape.
func node(c *cmdregistry.Command, subs []Command) Command {
	n := Command{
		Name:        c.Name,
		Path:        c.Path,
		Short:       c.Summary,
		Hidden:      c.Hidden,
		Aliases:     c.Aliases,
		Subcommands: subs,
	}
	if c.Deprecated != nil {
		n.Deprecated = *c.Deprecated
	}
	if c.Group != nil {
		n.GroupID = *c.Group
	}
	for _, f := range c.Flags {
		n.Flags = append(n.Flags, "--"+f.Name)
	}
	return n
}
