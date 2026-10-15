// Package clone provides embedded app-clone templates for nself init --template.
// Each template bundles a Postgres schema, Hasura metadata, seed data, and a
// backend configuration. Templates are embedded in the binary via go:embed so no
// network access is required at scaffold time.
package clone

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed airbnb-clone discord-clone notion-clone slack-clone substack-clone zoom-clone
var templateFS embed.FS

// Manifest describes a clone template.
type Manifest struct {
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Description     string   `json:"description"`
	RequiredPlugins []string `json:"required_plugins"`
}

// All returns the list of all bundled clone template names.
func All() []string {
	return []string{
		"airbnb-clone",
		"discord-clone",
		"notion-clone",
		"slack-clone",
		"substack-clone",
		"zoom-clone",
	}
}

// IsCloneTemplate reports whether name is one of the six bundled clone templates.
func IsCloneTemplate(name string) bool {
	for _, t := range All() {
		if t == name {
			return true
		}
	}
	return false
}

// GetManifest reads the manifest.json for the named template.
func GetManifest(name string) (Manifest, error) {
	data, err := templateFS.ReadFile(name + "/manifest.json")
	if err != nil {
		return Manifest{}, fmt.Errorf("reading manifest for %q: %w", name, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parsing manifest for %q: %w", name, err)
	}
	return m, nil
}

// ScaffoldOptions controls how a template is written to disk.
type ScaffoldOptions struct {
	// TargetDir is the directory to write files into. It must exist.
	TargetDir string
	// NoSeed skips 002_seed.sql.
	NoSeed bool
	// DryRun prints files without writing.
	DryRun bool
}

// Scaffold writes all template files for name into opts.TargetDir.
// Returns the list of files written (or that would be written on dry run).
func Scaffold(name string, opts ScaffoldOptions) ([]string, error) {
	root := name // no trailing slash — embed.FS WalkDir requires exact dir name
	prefix := name + "/"
	var written []string

	err := fs.WalkDir(templateFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel := strings.TrimPrefix(path, prefix)

		// Skip seed SQL when --no-seed is set.
		if opts.NoSeed && filepath.Base(rel) == "002_seed.sql" {
			return nil
		}

		written = append(written, rel)
		if opts.DryRun {
			return nil
		}

		dest := filepath.Join(opts.TargetDir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", rel, err)
		}

		data, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		if err := os.WriteFile(dest, data, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", dest, err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scaffolding template %q: %w", name, err)
	}
	return written, nil
}
