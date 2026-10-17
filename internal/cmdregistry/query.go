package cmdregistry

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Marshal renders the registry as the published JSON document: 2-space indent,
// no HTML escaping, trailing newline.
func (r *Registry) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Lookup finds a command by its full path ("nself config get"); the path
// without the root name ("config get") is accepted too.
func (r *Registry) Lookup(path string) (*Command, bool) {
	if i, ok := r.index[path]; ok {
		return &r.Commands[i], true
	}
	if i, ok := r.index[r.rootPath+" "+strings.TrimSpace(path)]; ok {
		return &r.Commands[i], true
	}
	return nil, false
}

// WithInheritedFlags returns cmd with the persistent flags of its non-root
// ancestors appended to Flags, so a machine request can set a flag the command
// only inherits (config get --env). The nearest declaring ancestor wins, a flag
// the command declares itself is kept as is, and root flags are left out (they
// are the invoker's switches). cmd is not modified; a command with nothing to
// inherit is returned as is.
func (r *Registry) WithInheritedFlags(cmd *Command) *Command {
	if r == nil || cmd == nil {
		return cmd
	}
	have := map[string]bool{}
	for _, f := range cmd.Flags {
		have[f.Name] = true
	}
	var extra []Flag
	seen := map[string]bool{}
	walked := map[string]bool{cmd.Path: true}
	for p := cmd.Parent; p != ""; {
		parent, ok := r.Lookup(p)
		if !ok || walked[parent.Path] { // a parent cycle ends the walk
			break
		}
		walked[parent.Path] = true
		for _, f := range parent.Flags {
			if f.Persistent && !have[f.Name] && !seen[f.Name] {
				seen[f.Name] = true
				extra = append(extra, f)
			}
		}
		p = parent.Parent
	}
	if len(extra) == 0 {
		return cmd
	}
	out := *cmd
	out.Flags = append(append([]Flag{}, cmd.Flags...), extra...)
	return &out
}

// Subtree returns a registry holding the command at path and every descendant
// (nil when the path is unknown). Counts are recomputed over the subset except
// core_missing, which stays registry-wide. The result shares no slice with r.
func (r *Registry) Subtree(path string) *Registry {
	head, ok := r.Lookup(path)
	if !ok {
		return nil
	}
	sub := &Registry{
		SchemaVersion: r.SchemaVersion,
		Verbs:         append([]string{}, r.Verbs...),
		Root:          Root{Summary: r.Root.Summary, Flags: append([]Flag{}, r.Root.Flags...)},
		Commands:      []Command{},
		rootPath:      r.rootPath,
	}
	for _, c := range r.Commands {
		if c.Path == head.Path || strings.HasPrefix(c.Path, head.Path+" ") {
			sub.Commands = append(sub.Commands, c)
		}
	}
	sub.reindex()
	sub.Counts = computeCounts(sub.Verbs, sub.Commands, sub.rootPath, append([]string{}, r.Counts.CoreMissing...))
	return sub
}
