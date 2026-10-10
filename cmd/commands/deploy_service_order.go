package commands

// Purpose: derive the rolling-restart service order from the project's own
// resolved compose model instead of a hardcoded list. Inputs are the service
// names `docker compose config --services` reports over the compose manifest
// (.nself/compose-files.txt plus the env files, the same inputs restart uses)
// and the set contributed by the manifest's plugin fragments; output is the
// restart order to use.
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
	"path/filepath"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/docker"

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
// pluginServices is the set of service names contributed by the manifest's
// plugin compose fragments (see manifestPluginServices). Passing a name
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

// deployCompose builds the Compose for workdir the way restart does: every file
// in .nself/compose-files.txt as -f and build.ComposeEnvFiles as --env-file, so
// plugin fragments and the variables computed for them are part of every call.
func deployCompose(workdir string) (*docker.Compose, []string, error) {
	files, err := build.ReadComposeManifest(workdir)
	if err != nil {
		return nil, nil, fmt.Errorf("reading compose manifest: %w", err)
	}
	c := docker.NewCompose(files...)
	c.EnvFiles = build.ComposeEnvFiles(workdir)
	return c, files, nil
}

// composeFragmentDoc is the minimal shape needed to read a compose file's
// top-level service names.
type composeFragmentDoc struct {
	Services map[string]interface{} `yaml:"services"`
}

// manifestPluginServices returns the service names the manifest's plugin
// fragments add on top of the base file (files[0]). The hand-written
// docker-compose.override.yml and the image override file only adjust existing
// services, so they never count. Best-effort: an unreadable fragment is
// skipped, which only moves a plugin service out of the "last" group (order,
// never whether a service is restarted).
func manifestPluginServices(workdir string, files []string) map[string]bool {
	names := map[string]bool{}
	if len(files) < 2 {
		return names
	}
	base := composeServiceSet(workdir, files[0])
	for _, f := range files[1:] {
		switch filepath.Base(f) {
		case "docker-compose.override.yml", filepath.Base(docker.ImageOverrideFile):
			continue
		}
		for svc := range composeServiceSet(workdir, f) {
			if !base[svc] {
				names[svc] = true
			}
		}
	}
	return names
}

// composeServiceSet reads the top-level service names of one compose file.
func composeServiceSet(workdir, path string) map[string]bool {
	set := map[string]bool{}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workdir, path)
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return set
	}
	var doc composeFragmentDoc
	if yaml.Unmarshal(data, &doc) != nil {
		return set
	}
	for svc := range doc.Services {
		set[svc] = true
	}
	return set
}

// projectServiceOrder resolves the rolling-restart order: what the manifest
// compose reports (`config --services`) arranged by resolveServiceOrder, with
// the manifest's plugin-fragment services last. It errors only when the
// services themselves cannot be determined.
func projectServiceOrder(ctx context.Context, compose *docker.Compose, workdir string, files []string) ([]string, error) {
	present, err := compose.ComposeServiceNames(ctx, workdir)
	if err != nil {
		return nil, err
	}
	return resolveServiceOrder(present, manifestPluginServices(workdir, files)), nil
}
