package build

// Purpose: expose profile-aware plugin interpolation values to the build pipeline.
// Inputs: project/plugin paths, loaded config, and whether the selected profile has storage.
// Outputs: values for .env.computed and .nself/compose.env.
// Constraints: the compatibility wrapper keeps existing app-profile callers stable.

import (
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

// ComputePluginEnvVars returns plugin values for the default app profile.
func ComputePluginEnvVars(workdir, pluginDir string, cfg *config.Config) map[string]string {
	return computePluginEnvVars(workdir, pluginDir, cfg, true)
}

// computePluginEnvVarsForProfile omits store values when the profile omits storage.
func computePluginEnvVarsForProfile(workdir, pluginDir string, cfg *config.Config, storeProfile bool) map[string]string {
	return computePluginEnvVars(workdir, pluginDir, cfg, storeProfile)
}

// computePluginEnvVarsForProfileName derives storage membership from the profile registry.
func computePluginEnvVarsForProfileName(workdir, pluginDir string, cfg *config.Config, profile compose.ProfileName) map[string]string {
	set, _ := compose.ProfileForName(profile)
	return computePluginEnvVarsForProfile(workdir, pluginDir, cfg, set.Minio)
}
