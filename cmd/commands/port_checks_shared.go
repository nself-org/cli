package commands

// Purpose: the single entry point `nself start` and `nself doctor` both use to
//   decide whether a host port is a real conflict, plus the resolution of the
//   compose inputs that decision needs.
// Inputs:  the project directory.
// Outputs: compose env files, compose files, and whether a built stack exists
//   in that directory.
// Constraints: no side effects; read-only filesystem access.
//
// Why a shared file: doctor probed a fixed default port list with no ownership
// filter, so a healthy stack's own nginx/postgres/hasura/auth/admin containers
// were reported as six port conflicts on a green run, while start (which does
// filter) reported none. Two commands disagreeing about the same port on the
// same machine is the bug; one path is the fix.
// SPORT: CLI-CMD-DOCTOR-001

import (
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/docker"
)

// projectPortConflicts is the shared resolve-and-probe step. It is a package
// var so tests can substitute a fake and assert that start and doctor reach
// the same verdict for the same port without needing a Docker daemon.
var projectPortConflicts = docker.ProjectPortConflicts

// projectPortInputs resolves the compose env files and compose files for
// projectDir, and reports whether a built stack actually exists there.
//
// hasProject matters because ReadComposeManifest never fails for a missing
// manifest — it returns the default "docker-compose.yml" — so its success says
// nothing about whether a project is present. Without the on-disk check,
// running doctor in a directory that was never built would fail the ownership
// query and suppress the port check entirely, which is a different false
// answer from the one being fixed.
func projectPortInputs(projectDir string) (envFiles, composeFiles []string, hasProject bool) {
	envFiles = build.ComposeEnvFiles(projectDir)

	composeFiles, err := build.ReadComposeManifest(projectDir)
	if err != nil {
		composeFiles = nil
	}

	for _, f := range composeFiles {
		path := f
		if !filepath.IsAbs(path) {
			path = filepath.Join(projectDir, path)
		}
		if _, statErr := os.Stat(path); statErr == nil {
			hasProject = true
			break
		}
	}
	return envFiles, composeFiles, hasProject
}
