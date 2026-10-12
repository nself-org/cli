package bundle

// Purpose: cross-bundle MAX-resolution (S2.T22) — resolving the newest required version of a shared plugin across installed bundles, plus the semver parsing/comparison it depends on.
// Inputs: plugin names and their candidate/required version strings.
// Outputs: the maximum resolved version, detected version conflicts, and semver comparison results.
// Constraints: split out of version_resolver.go as a pure move (CLI-R12); no behavior change.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/plugin"
)

// ---------------------------------------------------------------------------
// Cross-bundle MAX-resolution (S2.T22)
// ---------------------------------------------------------------------------

// ResolveMaxVersion returns the maximum (newest) semver string from versions.
// All versions must be valid semver (with or without a leading "v" prefix).
// Returns an error if versions is empty or any entry is not valid semver.
// The returned string preserves the "v" prefix of the winning candidate.
//
// Example:
//
//	ResolveMaxVersion("ai", []string{"1.0.0", "1.2.0", "1.1.0"}) → "1.2.0", nil
func ResolveMaxVersion(pluginName string, versions []string) (string, error) {
	if len(versions) == 0 {
		return "", fmt.Errorf("plugin %q: no version requirements provided", pluginName)
	}
	max := versions[0]
	for _, v := range versions[1:] {
		cmp, err := compareSemver(v, max)
		if err != nil {
			return "", fmt.Errorf("plugin %q: invalid semver %q: %w", pluginName, v, err)
		}
		if cmp > 0 {
			max = v
		}
	}
	// Validate the initial candidate too (catches bad first entry).
	if err := validateSemver(versions[0]); err != nil {
		return "", fmt.Errorf("plugin %q: invalid semver %q: %w", pluginName, versions[0], err)
	}
	return max, nil
}

// ResolveConflicts takes a map of plugin slug → list of version requirements
// from different bundles and returns the resolved map of plugin slug → max
// version. If strict is true, any plugin that has more than one distinct
// version requirement returns an error instead of resolving to the max.
//
// A "conflict" in strict mode means two or more bundles require different
// versions of the same plugin. When strict is false (the default), conflicts
// are silently resolved by taking the highest (MAX) version.
func ResolveConflicts(requirements map[string][]string, strict bool) (map[string]string, error) {
	resolved := make(map[string]string, len(requirements))
	for plugin, vers := range requirements {
		if len(vers) == 0 {
			return nil, fmt.Errorf("plugin %q: empty version list", plugin)
		}
		if strict && hasConflict(vers) {
			return nil, fmt.Errorf(
				"plugin %q has conflicting version requirements across bundles: %s",
				plugin, strings.Join(vers, ", "),
			)
		}
		max, err := ResolveMaxVersion(plugin, vers)
		if err != nil {
			return nil, err
		}
		resolved[plugin] = max
	}
	return resolved, nil
}

// hasConflict reports whether a slice contains more than one distinct version.
func hasConflict(versions []string) bool {
	if len(versions) <= 1 {
		return false
	}
	first := strings.TrimPrefix(strings.TrimSpace(versions[0]), "v")
	for _, v := range versions[1:] {
		if strings.TrimPrefix(strings.TrimSpace(v), "v") != first {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Semver helpers (one implementation: plugin.CompareSemver, D-0191)
// ---------------------------------------------------------------------------

// compareSemver returns the sign (-1, 0, 1) of a versus b, or an error when
// either is not a valid version. It delegates to plugin.CompareSemver.
func compareSemver(a, b string) (int, error) { return plugin.CompareSemver(a, b) }

// validateSemver returns an error when s is not a valid version string.
func validateSemver(s string) error {
	_, err := plugin.CompareSemver(s, s)
	return err
}
