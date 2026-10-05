package build

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"gopkg.in/yaml.v3"
)

// Purpose: resolve CS_N_DEPENDS_ON names (P7-ADOPT-01, review F24). Compose
// generation only checks the syntax of a dependency name, because the plugin
// compose fragments do not exist yet at that point. This check runs from
// PostValidate, after the plugin step wrote .nself/compose-files.txt, so the
// first build already sees every plugin service.
// Inputs: the generated compose path (its directory is the project work dir)
// and the CS_<N>_* environment the build already loaded.
// Outputs: one E500 finding per unknown dependency name, and one per
// dependency cycle that passes through a custom service (the full path, e.g.
// "api -> claw-api -> api"), appended to result.Errors, which fails the build.
// Constraints: a service counts as known when it is in the generated compose,
// in any file the manifest lists (plugin fragments, user override), i.e. a
// core service, another custom service or a plugin service. A config parse
// error is not reported here: the build failed on it earlier.

// checkCustomServiceDeps appends an E500 finding for every CS_N_DEPENDS_ON
// name that is not a service of the project.
func checkCustomServiceDeps(composePath string, result *PostValidateResult) {
	cs, err := config.CustomServicesFromEnv()
	if err != nil {
		return
	}
	needed := false
	for _, s := range cs {
		if len(s.DependsOn) > 0 {
			needed = true
		}
	}
	if !needed {
		return
	}

	files := []string{composePath}
	workdir := filepath.Dir(composePath)
	if _, statErr := os.Stat(filepath.Join(workdir, composeManifestFile)); statErr == nil {
		if listed, readErr := ReadComposeManifest(workdir); readErr == nil {
			files = append(files, listed...)
		}
	}
	known, err := docker.ComposeServiceNames(files)
	if err != nil {
		result.Warnings = append(result.Warnings,
			"CS_N_DEPENDS_ON check skipped: "+err.Error())
		return
	}

	unknown := false
	for _, s := range cs {
		for _, dep := range s.DependsOn {
			if _, ok := known[dep.Name]; ok {
				continue
			}
			unknown = true
			result.Errors = append(result.Errors, errs.Newf("E500",
				"CS_%d_DEPENDS_ON names %q, which is not a service of this project (core, custom or plugin)",
				s.Index, dep.Name).Error())
		}
	}
	if unknown {
		return
	}

	// Cycles over the whole compose set (core, custom, plugin fragments):
	// compose refuses to start a cyclic stack, so fail the build instead.
	edges, err := composeDependsEdges(files)
	if err != nil {
		result.Warnings = append(result.Warnings, "CS_N_DEPENDS_ON cycle check skipped: "+err.Error())
		return
	}
	for _, s := range cs {
		if len(s.DependsOn) == 0 {
			continue
		}
		if cycle := findDependsCycle(edges, s.Name); cycle != nil {
			result.Errors = append(result.Errors, errs.Newf("E500",
				"CS_%d_DEPENDS_ON forms a dependency cycle: %s (docker compose would refuse to start it)",
				s.Index, strings.Join(cycle, " -> ")).Error())
		}
	}
}

// composeDependsEdges reads every service's depends_on (list or map form) from
// the compose files and returns name -> dependencies, merged across files.
func composeDependsEdges(files []string) (map[string][]string, error) {
	edges := map[string][]string{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		var doc struct {
			Services map[string]struct {
				DependsOn any `yaml:"depends_on"`
			} `yaml:"services"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
		for name, svc := range doc.Services {
			switch d := svc.DependsOn.(type) {
			case []any:
				for _, x := range d {
					if n, ok := x.(string); ok {
						edges[name] = append(edges[name], n)
					}
				}
			case map[string]any:
				keys := make([]string, 0, len(d))
				for k := range d {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				edges[name] = append(edges[name], keys...)
			}
		}
	}
	return edges, nil
}

// findDependsCycle returns the cycle through start as "start -> ... -> start",
// or nil.
func findDependsCycle(edges map[string][]string, start string) []string {
	var path []string
	onPath := map[string]bool{}
	var visit func(n string) []string
	visit = func(n string) []string {
		path = append(path, n)
		onPath[n] = true
		defer func() { path = path[:len(path)-1]; onPath[n] = false }()
		for _, next := range edges[n] {
			if next == start {
				return append(append([]string(nil), path...), start)
			}
			if onPath[next] {
				continue
			}
			if c := visit(next); c != nil {
				return c
			}
		}
		return nil
	}
	return visit(start)
}
