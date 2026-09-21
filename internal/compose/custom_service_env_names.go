package compose

// Purpose: expose, as NAMES only (never values), the environment variables
// one custom service will receive — the fixed core set every service gets
// plus whatever CS_N_ENV_PASSTHROUGH/CS_N_ENV add. Used by `nself deploy
// --dry-run` (cmd/commands/deploy_build_root.go) so an operator can see, per
// custom service, that a plain .env.prod addition does NOT reach that
// service's container unless CS_N_ENV_PASSTHROUGH or CS_N_ENV names it
// explicitly — the surface custom_services.go's coreEnvVars deliberately
// keeps narrow (see its own header comment).
// Inputs:  a config.CustomService (for the per-service allowlist/overrides).
// Outputs: []string of env var names, safe to print — no secret values.
// Constraints: FixedCoreEnvVarNames is derived from fixedCoreEnvVars itself
// (zero-value cfg/svc — the key SET never depends on field values) rather
// than a hand-maintained duplicate list, so it cannot silently drift from
// what a custom service actually receives.

import (
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// FixedCoreEnvVarNames returns the environment variable names every custom
// service receives regardless of CS_N_ENV_PASSTHROUGH or CS_N_ENV, sorted
// for stable output.
func FixedCoreEnvVarNames() []string {
	env := fixedCoreEnvVars(&config.Config{}, config.CustomService{})
	names := make([]string, 0, len(env))
	for k := range env {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// CustomServiceEnvVarNames returns every environment variable name svc will
// receive: the fixed core set (FixedCoreEnvVarNames), then its
// CS_N_ENV_PASSTHROUGH allowlist names in declared order, then its CS_N_ENV
// explicit keys in declared order. Names only — never the resolved values —
// so this is safe to print in a dry-run summary.
func CustomServiceEnvVarNames(svc config.CustomService) []string {
	names := FixedCoreEnvVarNames()

	if svc.EnvPassthrough != "" {
		for _, name := range strings.Split(svc.EnvPassthrough, ",") {
			if name = strings.TrimSpace(name); name != "" {
				names = append(names, name)
			}
		}
	}

	if svc.ExtraEnv != "" {
		for _, pair := range strings.Split(svc.ExtraEnv, ",") {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) != 2 {
				continue
			}
			if k := strings.TrimSpace(parts[0]); k != "" {
				names = append(names, k)
			}
		}
	}

	return names
}
