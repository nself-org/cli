package nginx

// routes_helpers.go — small helpers shared by route generation.
//
// Purpose: detect domain conflicts, identify the Linux server case, and resolve the MinIO console and Hasura container ports, used throughout routes.go and its split files.
// Inputs: the generated []NginxRoute list, or the loaded Config.
// Outputs: a conflict decision, an OS check, or a resolved port number.
// Constraints: pure move from routes.go (CLI-R12 Batch E); no behaviour change.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginxtopo"
)

// hasDomainConflict checks whether a server_name conflicts with an existing
// route. It checks two sources:
//  1. The current generation batch (duplicate routes within this build).
//  2. Hand-managed nginx/conf.d/ files that already define the server_name.
//
// Returns true if the route should be skipped.
func (g *Generator) hasDomainConflict(route, baseDomain string) bool {
	serverName, err := config.RouteToFQDN(route, baseDomain)
	if err != nil {
		return false
	}

	// Check within the current generation batch.
	if g.seenRoutes[serverName] {
		return true
	}
	g.seenRoutes[serverName] = true

	// Check if this server_name already exists in hand-managed nginx/conf.d/ files.
	// The served nginx dir (D-0045): a fronted project's hand-managed confs live
	// in the fronting stack's tree. An unconfirmed fronted layout falls back to
	// the project's own tree, as before (build refuses that layout elsewhere).
	nginxDir, err := nginxtopo.ServedNginxDir(g.workdir, g.cfg.Nginx.FrontedBy)
	if err != nil {
		nginxDir, _ = nginxtopo.ServedNginxDir(g.workdir, "")
	}
	confDirs := []string{
		filepath.Join(nginxDir, "conf.d"),
		filepath.Join(nginxDir, fmt.Sprintf("conf.d-%s", g.cfg.Env)),
	}

	for _, dir := range confDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || entry.Name() == "default.conf" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				continue
			}
			if strings.Contains(string(data), "server_name") && strings.Contains(string(data), serverName) {
				return true
			}
		}
	}
	return false
}

// isLinuxServer returns true when running on a Linux VPS (not Docker Desktop).
// Frontend app routes are skipped on Linux servers because host.docker.internal
// is not available.
func isLinuxServer() bool {
	return runtime.GOOS == "linux"
}

// minioConsolePort returns the MinIO console port from config with default fallback.
func minioConsolePort(cfg *config.Config) int {
	if cfg.Minio.ConsolePort != 0 {
		return cfg.Minio.ConsolePort
	}
	return 9001
}

// hasuraContainerPort is the port the Hasura GraphQL engine always listens on
// INSIDE its container, regardless of what host port it is mapped to via
// HASURA_PORT/cfg.Hasura.Port. nginx and Hasura share the same Docker network,
// so every in-network upstream (nginx, functions, admin, custom services) must
// address Hasura at this fixed port — never the host-mapped port (gap #7).
// See buildHasuraService in internal/compose/core_services.go, which maps
// "127.0.0.1:<cfg.Hasura.Port>:8080" — the "8080" here is this constant.
const hasuraContainerPort = 8080
