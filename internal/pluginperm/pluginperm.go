// Package pluginperm checks that a plugin binary path cannot be swapped by
// another local user before the CLI executes it (D-1396). It imports nothing
// from the module, so it adds no importer to internal/errs (fan-in ratchet).
package pluginperm

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Resolve is the check a plugin binary passes before exec: candidate resolves
// (through symlinks) to a path inside rootDir, every component up to rootDir is
// safe (see Verify), and the target is a regular executable file. It returns
// the resolved path. Errors name slug and carry no code; the caller wraps them.
func Resolve(candidate, rootDir, slug string) (string, error) {
	resolvedRoot, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		return "", fmt.Errorf("plugin %q binary directory is unavailable", slug)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("plugin %q binary is unavailable", slug)
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("plugin %q binary resolves outside the plugins directory", slug)
	}
	if err := Verify(resolved, resolvedRoot); err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0) {
		return "", fmt.Errorf("plugin %q binary is not executable", slug)
	}
	return resolved, nil
}
