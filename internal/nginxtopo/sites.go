package nginxtopo

// Purpose: keep generated site writes inside the stack that serves them.
// Inputs: project root and fronting stack name. Outputs: the lexical sites
// directory or an error. Constraints: resolve existing ancestors, including
// symlinks; missing sites are allowed, unresolved links are refused.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ServedSitesDir validates sites against the canonical served root. Returning
// the lexical spelling preserves the artifact paths used by the build plan.
func ServedSitesDir(projectDir, frontedBy string) (string, error) {
	root, err := ServedRoot(projectDir, frontedBy)
	if err != nil {
		return "", err
	}
	sites := filepath.Join(root, "nginx", "sites")
	realRoot, err := resolveExisting(root)
	if err != nil {
		return "", fmt.Errorf("resolving nginx stack root: %w", err)
	}
	realSites, err := resolveExisting(sites)
	if err != nil {
		return "", fmt.Errorf("refusing nginx sites %s: cannot rule out a link outside the served root: %w", sites, err)
	}
	rel, err := filepath.Rel(realRoot, realSites)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing nginx sites %s: resolved path %s is outside served root %s", sites, realSites, realRoot)
	}
	return sites, nil
}

// resolveExisting resolves the longest existing ancestor, then appends missing
// components. A dangling symlink is existing and must fail EvalSymlinks.
func resolveExisting(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(path); err == nil {
		return filepath.EvalSymlinks(path)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent, err := resolveExisting(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}
