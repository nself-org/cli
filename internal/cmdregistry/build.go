package cmdregistry

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/canon"
	"github.com/spf13/cobra"
)

// node is one cobra command with its canon key (path without the root name).
type node struct {
	cmd   *cobra.Command
	key   string
	depth int
}

// Build derives the registry from root, the declared canon data c and the
// registered JSON data types (keys are command paths without the leading
// "nself "; values are only tested for presence). It never mutates the cobra
// tree. Every violated rule is collected into one *canon.ValidationError that
// names each offending path; on error the registry is nil.
func Build(root *cobra.Command, c *canon.File, dataTypes map[string]any, opts BuildOptions) (*Registry, error) {
	if root == nil || c == nil {
		return nil, errors.New("cmdregistry: Build needs a root command and canon data")
	}
	var problems []string
	if err := c.Validate(); err != nil {
		var ve *canon.ValidationError
		if errors.As(err, &ve) {
			problems = append(problems, ve.Problems...)
		} else {
			problems = append(problems, err.Error())
		}
	}
	nodes := walk(root)
	byKey := map[string]*node{}
	annotated := map[string]bool{}
	for i := range nodes {
		byKey[nodes[i].key] = &nodes[i]
		if mountSlug(nodes[i].cmd) != "" {
			annotated[nodes[i].key] = true
		}
	}
	// A canon plugin entry is file-level invalid (the validator cannot see
	// mount annotations); for annotated nodes Build owns the rule and lifts
	// that one problem (plugin.go).
	problems = liftPluginReservation(problems, annotated)
	problems = append(problems, checkResolves(c, dataTypes, opts, byKey)...)

	rootPath := root.CommandPath()
	reg := &Registry{
		SchemaVersion: SchemaVersion,
		Verbs:         append([]string{}, c.Verbs...),
		Root:          Root{Summary: root.Short, Flags: localFlags(root)},
		Commands:      make([]Command, 0, len(nodes)),
		rootPath:      rootPath,
	}
	warnedPlugins := map[string]bool{}
	for _, n := range nodes {
		entry, ok := c.Commands[n.key]
		switch {
		case isInstalledMount(n.cmd):
			// Installed plugins need no canon entry; if one exists it must
			// not claim the path for a core command.
			if ok && entry.Canon != canon.CanonPlugin {
				problems = append(problems, fmt.Sprintf("commands[%q]: plugin-mounted command's canon entry must say canon plugin, not %q", n.key, entry.Canon))
				continue
			}
			reg.Commands = append(reg.Commands, buildInstalledCommand(n.cmd, warnedPlugins))
		case isBuiltinMount(n.cmd):
			// A builtin family's attributes come from its canon entry, which
			// says canon plugin only in v1.5. The v1.4 registry keeps the
			// presplit canon classification and has no plugin identity.
			if !ok {
				problems = append(problems, fmt.Sprintf("commands[%q]: builtin plugin family %q needs a canon entry saying canon plugin; add one to internal/canon/canon.yaml", n.key, mountSlug(n.cmd)))
				continue
			}
			if !opts.V15 {
				cmd, p := buildCommand(n, entry, rootPath, byKey, c, dataTypes, opts)
				problems = append(problems, p...)
				reg.Commands = append(reg.Commands, cmd)
				continue
			}
			if entry.Canon != canon.CanonPlugin {
				problems = append(problems, fmt.Sprintf("commands[%q]: builtin plugin family's canon entry must say canon plugin, not %q", n.key, entry.Canon))
				continue
			}
			cmd, p := buildCommand(n, entry, rootPath, byKey, c, dataTypes, opts)
			problems = append(problems, p...)
			slug := mountSlug(n.cmd)
			cmd.Plugin = &slug
			reg.Commands = append(reg.Commands, cmd)
		case !ok:
			problems = append(problems, fmt.Sprintf("commands[%q]: no canon entry for command %q; add one to internal/canon/canon.yaml", n.key, n.cmd.CommandPath()))
		default:
			cmd, p := buildCommand(n, entry, rootPath, byKey, c, dataTypes, opts)
			problems = append(problems, p...)
			reg.Commands = append(reg.Commands, cmd)
		}
	}
	if err := canon.NewValidationError(problems); err != nil {
		return nil, err
	}
	sort.Slice(reg.Commands, func(i, j int) bool { return reg.Commands[i].Path < reg.Commands[j].Path })
	reg.reindex()
	reg.Counts = computeCounts(reg.Verbs, reg.Commands, rootPath, nil)
	return reg, nil
}

// walk lists every command under root (hidden ones included, root excluded).
func walk(root *cobra.Command) []node {
	var out []node
	var rec func(c *cobra.Command)
	prefix := root.CommandPath() + " "
	rec = func(c *cobra.Command) {
		for _, ch := range c.Commands() {
			key := strings.TrimPrefix(ch.CommandPath(), prefix)
			out = append(out, node{cmd: ch, key: key, depth: canon.Depth(key)})
			rec(ch)
		}
	}
	rec(root)
	return out
}

// checkResolves reports canon entries, data types and V15OnlyEnvelope paths
// that do not name a command in the tree.
func checkResolves(c *canon.File, dataTypes map[string]any, opts BuildOptions, byKey map[string]*node) []string {
	var p []string
	for key := range c.Commands {
		if byKey[key] == nil {
			p = append(p, fmt.Sprintf("commands[%q]: entry does not resolve to a command in the tree", key))
		}
	}
	for key := range dataTypes {
		if byKey[key] == nil && (opts.V15 || !opts.V15OnlyEnvelope[key]) {
			p = append(p, fmt.Sprintf("dataTypes[%q]: registered data type does not resolve to a command in the tree", key))
		}
	}
	for key := range opts.V15OnlyEnvelope {
		if byKey[key] == nil && opts.V15 {
			p = append(p, fmt.Sprintf("V15OnlyEnvelope[%q]: path does not resolve to a command in the tree", key))
		}
	}
	return p
}

func (r *Registry) reindex() {
	r.index = make(map[string]int, len(r.Commands))
	for i := range r.Commands {
		r.index[r.Commands[i].Path] = i
	}
}
