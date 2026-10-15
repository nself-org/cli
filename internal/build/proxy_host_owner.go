package build

// Purpose: tell the Traefik plugin which uid:gid owns the 0600 private keys the
// build issues (P7-DEPL-03, DEPL EPIC ruling 2026-10-12, plugins b3f4cc1f).
// Inputs: the build's loaded config, workdir and plugin dir.
// Outputs: NSELF_HOST_UID and NSELF_HOST_GID for .nself/compose.env, or nil.
// Constraints: only when the provider is Traefik and the host is Linux, where
// bind-mounted keys keep their host owner and a root container with every
// capability dropped cannot read them. Elsewhere the fragment's 0:0 default
// stays, so an nginx project's compose.env is unchanged. The build process
// issues the keys, so its own ids are their owner.
// Rootless/userns daemons keep the fragment's default user because the host
// ids would be remapped. Offline builds retain the existing host-id export.

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
)

// proxyHostOwner returns the ids Traefik runs as, or nil when nothing is exported.
func proxyHostOwner(cfg *config.Config, workdir, pluginDir string) map[string]string {
	if runtime.GOOS != "linux" {
		return nil
	}
	if provider, err := ProxyProvider(cfg, workdir, pluginDir); err != nil || provider != ProviderTraefik {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if remapped, err := docker.UserNamespaceRemapped(ctx); err == nil && remapped {
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
