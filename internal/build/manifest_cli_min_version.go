package build

// Purpose: validate the optional minimum CLI version in nself.yaml.
// Inputs: a YAML string value. Outputs: whether it is a complete semver.
// Constraints: accept an optional v prefix and semver prerelease/build suffixes.
// SPORT: contract:config.nself-yaml.

import (
	"regexp"

	"gopkg.in/yaml.v3"
)

const CodeCLIMinVersion = "E061"

var cliMinVersionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

// ValidateCLIMinVersion reports whether value is a full semantic version.
func ValidateCLIMinVersion(value string) bool {
	return cliMinVersionPattern.MatchString(value)
}

func (c *collector) cliMinVersionErr(n *yaml.Node) {
	// compat.V15(P7-TRUTH-12): an invalid minimum version warns in v1.4 and fails in v1.5.
	c.add(n.Line, n.Column, "cli_min_version", CodeCLIMinVersion, "invalid CLI minimum version: expected semver X.Y.Z (optional v prefix)", "set cli_min_version to a version such as 1.4.0")
}
