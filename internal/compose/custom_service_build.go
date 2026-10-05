package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/config"
)

// Purpose: build-context resolution for custom services, split out of
// custom_services.go (P7-ADOPT-01) to keep that file under the 300-line
// ratchet. A pure move: the functions and their comments are unchanged.
// Inputs: a parsed config.CustomService and the project work dir.
// Outputs: the "./services/<dir>" build context string.
// Constraints: see each function.

// defaultCustomServiceBuildContext derives the default Dockerfile build
// context for a custom service when CS_N_PATH is not set.
//
// Purpose: a CS_N name may contain underscores (e.g. "ping_api"), but
// config.parseCustomServices sanitizes cs.Name into a hyphenated Docker
// service/container name ("ping-api") — see CustomService.RawName. Before
// this existed, the default build context was built from the sanitized
// cs.Name, so a project with a real `services/ping_api/` directory on disk
// got a build context of `./services/ping-api`, which doesn't exist, and
// `nself build`/`docker compose build` failed with a missing-context error
// even though the service directory was present.
//
// Inputs: cs — the parsed CustomService (cs.RawName is the pre-sanitization
// name, cs.Name the sanitized one; RawName is empty for a CustomService
// built by hand rather than via parseCustomServices, e.g. in tests).
// workDir — the project root used to probe which directory actually exists
// on disk; empty resolves relative to the process's current directory, same
// as loadCustomServiceEnvFile.
//
// Outputs: a "./services/<dir>" build context string.
//
// Constraints: prefers the raw (originally configured) name's directory
// when it's the one that exists on disk, since that's the name the user
// actually typed and the one `nself service create`-style scaffolding
// creates (internal/scaffold/service.go uses the raw name verbatim). Falls
// back to the sanitized name's directory when only that one exists (covers
// a service directory created under the hyphenated name directly). When
// RawName equals Name (no sanitization changed anything, e.g. an
// underscore-free name), no disk probe is needed. When neither directory
// exists, still returns the raw-name path so the resulting "context not
// found" build error names the directory the user actually configured
// rather than a hyphenated variant they never typed.
func defaultCustomServiceBuildContext(cs config.CustomService, workDir string) string {
	rawName := cs.RawName
	if rawName == "" {
		// Backward-compat: a hand-built CustomService (tests, or any future
		// caller that skips config.parseCustomServices) has no RawName —
		// cs.Name is the only name available.
		rawName = cs.Name
	}
	rawContext := fmt.Sprintf("./services/%s", rawName)
	if rawName == cs.Name {
		return rawContext
	}

	if customServiceDirExists(workDir, rawName) {
		return rawContext
	}
	if customServiceDirExists(workDir, cs.Name) {
		return fmt.Sprintf("./services/%s", cs.Name)
	}
	return rawContext
}

// customServiceDirExists reports whether services/<name> exists as a
// directory under workDir (or under the process's current directory when
// workDir is empty, mirroring loadCustomServiceEnvFile's empty-workDir
// fallback).
func customServiceDirExists(workDir, name string) bool {
	relPath := filepath.Join("services", name)
	path := relPath
	if workDir != "" {
		path = filepath.Join(workDir, relPath)
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
