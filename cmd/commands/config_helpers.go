package commands

// Purpose: shared config helpers: secret-key detection/masking, resolving the
// active env file name, and resolving the project directory. Inputs are a
// config key/value or project/env flags; outputs are bools, masked strings, or
// a resolved path.
// Constraints: split out of config.go (CLI-R12) as a pure move, no behavior change.
// The secret-key rule is config.IsSecretKey (internal/config/secretkeys.go),
// the one rule shared with the machine audit log (P7-SURF-12).

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// maskValue returns "***" when reveal is false and the key is a secret key,
// and otherwise the value with any URL credentials masked
// (config.MaskURLCredentials). reveal prints the stored value as is.
func maskValue(key, value string, reveal bool) string {
	if reveal {
		return value
	}
	if config.IsSecretKey(key) && value != "" {
		return "***"
	}
	return config.MaskURLCredentials(value)
}

// envFileName returns the .env filename to use for the given env flag value.
// An empty envFlag means use the plain ".env" file.
func envFileName(projectDir, envFlag string) string {
	if envFlag == "" {
		return filepath.Join(projectDir, ".env")
	}
	return filepath.Join(projectDir, ".env."+envFlag)
}

// mcpEnvNameRe is the shape of an env suffix taken from an MCP tool argument:
// lowercase letters, digits and hyphens, starting with a letter or digit.
var mcpEnvNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// validateMCPEnvName checks the env argument of the legacy MCP config tools
// before it reaches envFileName, which joins ".env."+env onto the project
// directory. An empty name means the plain .env file and is accepted; anything
// else must match mcpEnvNameRe, so path separators, "..", absolute paths and
// control characters are refused (D-0738).
// Inputs: the raw env argument. Outputs: nil, or an E401 usage error with why/fix.
func validateMCPEnvName(env string) error {
	if env == "" || mcpEnvNameRe.MatchString(env) {
		return nil
	}
	return errs.Newf("E401", "invalid env name %q", env).
		WithWhy("the env name becomes part of a file name (.env.<env>) inside the project, so it may hold only lowercase letters, digits and hyphens").
		WithFix("Use a name such as dev, staging or prod, or leave env empty for the plain .env file.")
}

// resolveProjectDir returns the nself project root, used by all config
// subcommands that need to locate .env files.
func resolveProjectDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	dir, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return "", fmt.Errorf("no nself project found in current directory or parents — run 'nself init' first")
	}
	return dir, nil
}

// --- S4-T01: config show ---
