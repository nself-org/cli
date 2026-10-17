package build

// Purpose: pre-create the traefik renderer's two output directories as the
// build user (D-0782) so Docker never creates them root-owned on Linux, which
// would stop an unprivileged build from removing them after a swap to nginx.
// Inputs: the build sink, the project workdir and the host owner map from
// proxyHostOwner (non-nil only for Traefik on Linux).
// Outputs: .nself/generated/traefik and .nself/generated/traefik-static, empty,
// mode 0755, through the sink (plan mode only records them).
// Constraints: a path that exists and is not a plain directory is left alone;
// no chown, no Docker calls. The traefik-config one-shot runs as the same
// NSELF_HOST_UID:NSELF_HOST_GID, so it can write into them.

import (
	"fmt"
	"os"
)

// prepareProxyOutputDirs creates the renderer output directories when owner is set.
func prepareProxyOutputDirs(sink Sink, workdir string, owner map[string]string) error {
	if len(owner) == 0 {
		return nil
	}
	for _, name := range proxyOutputNames {
		dir := proxyOutputPath(workdir, name)
		if _, err := os.Lstat(dir); err == nil {
			continue // already there (or a symlink or file the renderer reports): nothing to create
		}
		if err := sink.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating %s for the traefik renderer: %w", dir, err)
		}
	}
	return nil
}
