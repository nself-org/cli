package build

import (
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
)

// Purpose: resolve CS_N_DEPENDS_ON names (P7-ADOPT-01, review F24). Compose
// generation only checks the syntax of a dependency name, because the plugin
// compose fragments do not exist yet at that point. This check runs from
// PostValidate, after the plugin step wrote .nself/compose-files.txt, so the
// first build already sees every plugin service.
// Inputs: the generated compose path (its directory is the project work dir)
// and the CS_<N>_* environment the build already loaded.
// Outputs: one E500 finding per unknown dependency name appended to
// result.Errors, which fails the build.
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

	for _, s := range cs {
		for _, dep := range s.DependsOn {
			if _, ok := known[dep.Name]; ok {
				continue
			}
			result.Errors = append(result.Errors, errs.Newf("E500",
				"CS_%d_DEPENDS_ON names %q, which is not a service of this project (core, custom or plugin)",
				s.Index, dep.Name).Error())
		}
	}
}
