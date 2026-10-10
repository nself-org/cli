// Package schemas exposes the committed generated JSON schemas as bytes.
package schemas

// Purpose: reuse one schema artifact across MCP and HTTP surfaces.
// Inputs: paths relative to schemas/. Outputs: embedded bytes and sorted names.
// Constraints: standard library only; generated JSON remains the source of truth.

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed all:*.json all:commands/*.json all:invoke/*.json
var files embed.FS

// Lookup returns a copy of a committed schema by its relative path.
func Lookup(name string) ([]byte, bool) {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "..") || !strings.HasSuffix(name, ".json") {
		return nil, false
	}
	b, err := files.ReadFile(name)
	return b, err == nil
}

// Names lists every embedded schema path in lexical order.
func Names() []string {
	var names []string
	_ = fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".json") {
			names = append(names, path)
		}
		return nil
	})
	sort.Strings(names)
	return names
}
