package commands

// Purpose: derive the rolling-restart service order from the project's own
// resolved compose file instead of a hardcoded list. Inputs are the set of
// service names actually present in the project's docker-compose.yml
// (`docker compose config --services`) and the set contributed by installed
// plugin compose fragments; output is the restart order to use.
// Constraints: production incident 2026-09-20 — deployServiceOrder was a
// fixed [postgres hasura auth storage plugins] list. A real project's
// compose named object storage "minio" (never "storage") and had no
// "plugins" placeholder service at all, so the rolling restart recreated
// postgres/hasura/auth, then aborted with "no such service: storage" —
// the deploy stopped mid-restart with the stack in a half-recreated state.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/build"

	"gopkg.in/yaml.v3"
)

// composeCoreOrder is the fixed dependency-ordered prefix applied when the
// corresponding service is actually present in the resolved compose file.
// postgres must be healthy before hasura connects to it; hasura must be up
// before auth (hasura-auth) issues JWTs Hasura will authorise against.
var composeCoreOrder = []string{"postgres", "hasura", "auth"}

// resolveServiceOrder computes the rolling-restart order for a project:
//  1. composeCoreOrder, filtered to services actually present.
//  2. Every other present, non-plugin service, in the order docker reported
//     them (present's own order — stable, not re-sorted).
//  3. Every present plugin-contributed service last, again in present's
//     order.
//
// present is normally the output of `docker compose config --services`.
// pluginServices is the set of service names contributed by installed
// plugin compose fragments (see discoverPluginServiceNames). Passing a name
// that isn't in present is impossible by construction — only services that
// actually exist in the resolved compose ever appear in the result, which
// is what prevents restarting a nonexistent placeholder name (e.g. the old
// fixed list's "storage" against a project that only has "minio").
func resolveServiceOrder(present []string, pluginServices map[string]bool) []string {
	presentSet := make(map[string]bool, len(present))
	for _, s := range present {
		presentSet[s] = true
	}

	order := make([]string, 0, len(present))
	seen := make(map[string]bool, len(present))

	for _, core := range composeCoreOrder {
		if presentSet[core] && !seen[core] {
			order = append(order, core)
			seen[core] = true
		}
	}

	for _, s := range present {
		if seen[s] || pluginServices[s] {
			continue
		}
		order = append(order, s)
		seen[s] = true
	}

	for _, s := range present {
		if seen[s] {
			continue
		}
		if pluginServices[s] {
			order = append(order, s)
			seen[s] = true
		}
	}

	return order
}

// composeServiceNames runs `docker compose config --services` in workdir and
// returns the service names it reports, in the order docker printed them.
func composeServiceNames(ctx context.Context, workdir string) ([]string, error) {
	c := exec.CommandContext(ctx, "docker", "compose", "config", "--services")
	c.Dir = workdir
	c.Env = os.Environ()
	out, err := c.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("docker compose config --services: %w\n%s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("docker compose config --services: %w", err)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			names = append(names, s)
		}
	}
	return names, nil
}

// pluginComposeDoc is the minimal shape needed to read a plugin's compose
// fragment's top-level service names — mirrors the pattern
// internal/build/postvalidate.go's checkComposeYAML uses for the project's
// own compose file.
type pluginComposeDoc struct {
	Services map[string]interface{} `yaml:"services"`
}

// discoverPluginServiceNames returns the set of compose service names
// contributed by every installed, enabled plugin under workdir's plugin
// directory. Best-effort: a missing plugin dir, an unreadable fragment, or a
// parse failure for one plugin is silently skipped rather than failing the
// whole deploy — this set only affects restart ORDER (plugins last), never
// whether a service is restarted at all, so a partial result degrades
// gracefully to "some plugin services sort with the general group instead
// of last."
func discoverPluginServiceNames(workdir string) map[string]bool {
	names := map[string]bool{}
	composeFiles, err := build.DiscoverPluginComposeFiles(workdir, build.DefaultPluginDir())
	if err != nil {
		return names
	}
	for _, path := range composeFiles {
		data, readErr := os.ReadFile(filepath.Clean(path))
		if readErr != nil {
			continue
		}
		var doc pluginComposeDoc
		if yaml.Unmarshal(data, &doc) != nil {
			continue
		}
		for svc := range doc.Services {
			names[svc] = true
		}
	}
	return names
}

// projectServiceOrder resolves the rolling-restart order for workdir by
// combining composeServiceNames (what actually exists) with
// discoverPluginServiceNames (what sorts last). Returns an error only when
// the compose services themselves cannot be determined — plugin discovery
// failures never block a deploy (see discoverPluginServiceNames).
func projectServiceOrder(ctx context.Context, workdir string) ([]string, error) {
	present, err := composeServiceNames(ctx, workdir)
	if err != nil {
		return nil, err
	}
	return resolveServiceOrder(present, discoverPluginServiceNames(workdir)), nil
}
