package plugin

import (
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/version"
)

// Environment variable names of contract:cli.plugin-exec-env v1.
const (
	// EnvCLIVersion carries the cli's own version (semver, no leading "v").
	EnvCLIVersion = "NSELF_CLI_VERSION"
)

// ExecEnv returns the environment for a plugin process the cli starts
// (contract:cli.plugin-exec-env v1, P7-CI-59).
//
// Purpose: a plugin follows the cli's resolved compat mode and can check the
// cli version at startup, so the P7-SHIP-09 default flip needs no flip in the
// plugins.
//
// Inputs: base is the environment the plugin would otherwise get (a copy is
// returned; base is never modified). v15 resolves the compat mode; nil means
// compat.V15. It is called once per ExecEnv call and its result is not cached.
//
// Outputs: base plus NSELF_V15=1|0 and NSELF_CLI_VERSION=<semver without v>.
//
// Constraints: a NSELF_V15 already in base (the user's choice) passes through
// unchanged. Any NSELF_CLI_VERSION in base is replaced: it can only be a
// parent cli's value and must not outlive the cli that starts the plugin.
// Nothing else cli-internal is added.
func ExecEnv(base []string, v15 func() bool) []string {
	if v15 == nil {
		v15 = compat.V15
	}
	out := make([]string, 0, len(base)+2)
	hasMode := false
	for _, kv := range base {
		switch {
		case strings.HasPrefix(kv, compat.EnvVar+"="):
			hasMode = true
		case strings.HasPrefix(kv, EnvCLIVersion+"="):
			continue
		}
		out = append(out, kv)
	}
	if !hasMode {
		mode := "0"
		if v15() {
			mode = "1"
		}
		out = append(out, compat.EnvVar+"="+mode)
	}
	return append(out, EnvCLIVersion+"="+strings.TrimPrefix(version.GetVersion(), "v"))
}
