package build

// Purpose: validate the optional minimum CLI version in nself.yaml.
// Inputs: a YAML string value. Outputs: whether it is a complete semver.
// Constraints: accept an optional v prefix and semver prerelease/build suffixes.
// SPORT: contract:config.nself-yaml.

import (
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

const CodeCLIMinVersion = "E061"

var cliMinVersionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

// ValidateCLIMinVersion reports whether value is a full semantic version whose
// major, minor and patch fit an int, so the shared numeric comparator never
// sees a saturated component.
func ValidateCLIMinVersion(value string) bool {
	m := cliMinVersionPattern.FindStringSubmatch(value)
	if m == nil {
		return false
	}
	for _, part := range m[1:4] {
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}

func (c *collector) cliMinVersionErr(n *yaml.Node) {
	// compat.V15(P7-TRUTH-12): an invalid minimum version warns in v1.4 and fails in v1.5.
	c.add(n.Line, n.Column, "cli_min_version", CodeCLIMinVersion, "invalid CLI minimum version: expected semver X.Y.Z (optional v prefix)", "set cli_min_version to a version such as 1.4.0")
}
