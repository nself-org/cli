package build

// Purpose: tell the Traefik plugin which uid:gid owns the 0600 private keys the
// build issues (P7-DEPL-03, DEPL EPIC ruling 2026-10-12, plugins b3f4cc1f).
// Inputs: the build's loaded config, workdir and plugin dir.
// Outputs: NSELF_HOST_UID and NSELF_HOST_GID for .nself/compose.env, or nil.
// Constraints: only for Traefik on Linux, where bind-mounted keys keep their host
// owner and a capability-dropped root container cannot read them; a rootless or
// userns-remapped daemon maps uids to sub-uids, so 0:0 stays (D-0771). The build
// issues the keys, so its own ids are their owner.

import (
	"os"
	"runtime"
	"strconv"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
)

// proxyHostOwner returns the ids Traefik runs as, or nil when nothing is exported.
func proxyHostOwner(cfg *config.Config, workdir, pluginDir string) map[string]string {
	if runtime.GOOS != "linux" {
		return nil
	}
	if provider, err := ProxyProvider(cfg, workdir, pluginDir); err != nil || provider != ProviderTraefik || docker.RemapsUIDsOnce() {
		return nil
	}
	return map[string]string{
		"NSELF_HOST_UID": strconv.Itoa(os.Getuid()),
		"NSELF_HOST_GID": strconv.Itoa(os.Getgid()),
	}
}

// withHostOwner returns vars plus the host owner ids, without mutating vars.
func withHostOwner(vars, owner map[string]string) map[string]string {
	if len(owner) == 0 {
		return vars
	}
	merged := make(map[string]string, len(vars)+len(owner))
	for k, v := range vars {
		merged[k] = v
	}
	for k, v := range owner {
		merged[k] = v
	}
	return merged
}
