// Package model detects which plugin distribution model is in effect for a
// given plugin in the current project: Model A (Docker compose-managed,
// source under plugins-pro-src/) or Model B (single-binary, installed under
// ~/.nself/plugins/<name>/).
//
// The detector is the foundation for any command that must apply a plugin
// operation correctly across both models — most importantly enable/disable,
// which must edit compose files for Model A but only flip the .disabled
// marker for Model B.
package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/docker"
)

// Model identifies the distribution model of a plugin.
type Model int

const (
	// ModelUnknown means the plugin was not found by any detection step.
	ModelUnknown Model = iota
	// ModelB means the plugin is installed as a single binary in ~/.nself/plugins/<name>/.
	ModelB
	// ModelA means the plugin is compose-managed (source dir present, OR running compose service).
	ModelA
	// ModelMixed means both Model A and Model B artifacts were detected.
	// This is unexpected and the caller should surface a warning.
	ModelMixed
)

// String returns a human-readable name for the model.
func (m Model) String() string {
	switch m {
	case ModelB:
		return "B"
	case ModelA:
		return "A"
	case ModelMixed:
		return "Mixed"
	default:
		return "Unknown"
	}
}

// resolveOpts captures the lookup paths used by DetectModel. They are
// pulled into a struct so tests can override them without touching env vars.
type resolveOpts struct {
	homeDir       string
	projectDir    string
	pluginsProSrc string // optional override: full path to plugins-pro-src root
	dockerCheck   func(plugin string) bool
}

// defaultResolveOpts returns the production lookup paths.
func defaultResolveOpts() resolveOpts {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	return resolveOpts{
		homeDir:    home,
		projectDir: cwd,
		dockerCheck: func(name string) bool {
			// Best-effort: ask docker compose for a service named plugin-<name>.
			// Errors (no docker, not a project) are treated as "no service".
			out, err := docker.ComposeOutput(context.Background(), cwd, "ps", "--services")
			if err != nil {
				return false
			}
			service := "plugin-" + name
			for _, line := range strings.Split(string(out), "\n") {
				if strings.TrimSpace(line) == service {
					return true
				}
			}
			return false
		},
	}
}

// DetectModel returns the distribution model for the named plugin in the
// current working project, using the environment-driven defaults. The
// result is informational: ModelUnknown is returned with a nil error when
// no artifacts are found.
//
// Detection order (first match wins, but a Mixed result is returned when
// both A and B are detected):
//  1. ~/.nself/plugins/<name>/ exists                → ModelB
//  2. <project>/plugins-pro-src/paid/<name>/ exists  → ModelA
//  3. docker compose ps shows service plugin-<name>  → ModelA
func DetectModel(pluginName string) (Model, error) {
	if pluginName == "" {
		return ModelUnknown, fmt.Errorf("plugin name is empty")
	}
	return detectWithOpts(pluginName, defaultResolveOpts())
}

// detectWithOpts is the test seam for DetectModel.
func detectWithOpts(pluginName string, opts resolveOpts) (Model, error) {
	hasB := false
	hasA := false

	// Step 1: Model B check — single-binary install dir
	if opts.homeDir != "" {
		bDir := filepath.Join(opts.homeDir, ".nself", "plugins", pluginName)
		if fi, err := os.Stat(bDir); err == nil && fi.IsDir() {
			hasB = true
		}
	}

	// Step 2: Model A source dir check
	if opts.projectDir != "" {
		searchRoots := []string{opts.projectDir}
		// Also walk up to /opt/<project>/ when the plugin is deployed: this is
		// best-effort, look one level up for plugins-pro-src.
		if parent := filepath.Dir(opts.projectDir); parent != "" && parent != opts.projectDir {
			searchRoots = append(searchRoots, parent)
		}
		if opts.pluginsProSrc != "" {
			searchRoots = append([]string{filepath.Dir(opts.pluginsProSrc)}, searchRoots...)
		}
		for _, root := range searchRoots {
			candidate := filepath.Join(root, "plugins-pro-src", "paid", pluginName)
			if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
				hasA = true
				break
			}
			candidate = filepath.Join(root, "plugins-pro-src", pluginName)
			if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
				hasA = true
				break
			}
		}
	}

	// Step 3: docker compose service check (only if not already classified A).
	if !hasA && opts.dockerCheck != nil {
		if opts.dockerCheck(pluginName) {
			hasA = true
		}
	}

	switch {
	case hasA && hasB:
		return ModelMixed, nil
	case hasA:
		return ModelA, nil
	case hasB:
		return ModelB, nil
	default:
		return ModelUnknown, nil
	}
}
