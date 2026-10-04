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
