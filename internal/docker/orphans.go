package docker

// orphans.go — detection and (opt-in) removal of "orphaned" containers: ones
// that were created by a previous `nself build` + `docker compose up` for
// THIS project but whose service definition no longer exists in the
// freshly generated compose files.
//
// WHY (CLI gap G-014): a service dropped from docker-compose.yml (a service
// removed from nself.yaml, a plugin uninstalled, a renamed service) leaves
// its container running forever — `nself build` never looked at the live
// daemon at all, so nothing noticed. Measured live on prod: four containers
// (nself-claw, nself-notify, nself-mux, nself-cron) had no service
// definition, no nginx vhost, and no traffic; two of them had been dead
// since a database DNS failure a MONTH earlier and nobody noticed, because
// their healthchecks could never report anything meaningful either (see
// deep_docker.go's healthcheck-binary check for the other half of that
// incident). Detection must be on by default; removal must be opt-in,
// because an operator may have started a container by hand for debugging
// and not want it silently reaped.
//
// PROJECT SCOPING (read this before changing the filter below): every
// container docker compose creates is labeled com.docker.compose.project=
// <name>, where <name> is exactly the compose file's top-level `name:`
// field — which nSelf sets to cfg.ProjectName (see internal/compose
// Generator.buildDockerCompose). DetectOrphans filters `docker ps` on that
// exact label before it ever looks at service names, so a container from a
// different project (different label value) or a container docker didn't
// create via compose (no label at all) can never be selected — the filter
// runs server-side in the docker CLI, not as a client-side name guess.
// Never widen this to a bare `docker ps -a` scan.

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// composeProjectLabel is the label Docker Compose stamps on every container
// it creates, set to the compose file's top-level `name:` (nSelf's
// cfg.ProjectName). Filtering on it is what makes orphan detection safe to
// run against a host with other, unrelated Docker workloads.
const composeProjectLabel = "com.docker.compose.project"

// composeServiceLabel is the label holding the service name a container was
// created for. Compared against the freshly generated compose files' service
// set to decide whether a container is an orphan.
const composeServiceLabel = "com.docker.compose.service"

// OrphanContainer describes a running-or-stopped container that belongs to
// this project (by compose project label) but has no matching service in
// the freshly generated compose files.
type OrphanContainer struct {
	ID      string
	Name    string
	Service string // may be empty if the container somehow lacks the label
	State   string
}

// minimalComposeFile mirrors only the piece of a docker-compose.yml this
// package needs: the set of service names. Deliberately not compose.DockerCompose
// (internal/compose) — plugin-authored compose fragments are arbitrary,
// hand-written YAML and must not be forced through the stricter generator
// struct just to read their top-level keys.
type minimalComposeFile struct {
	Services map[string]yaml.Node `yaml:"services"`
}

// ComposeServiceNames reads every compose file in composeFilePaths (the base
// docker-compose.yml plus any plugin compose fragments — the same file set
// `docker compose -f ... -f ...` is invoked with, see
// build.ReadComposeManifest) and returns the union of all service names they
// define. This is the "desired state" DetectOrphans compares live containers
// against.
//
// Inputs:  composeFilePaths — absolute paths to YAML files; missing files are
//
//	skipped (best-effort — a plugin fragment can be removed from disk
//	independently of the manifest that references it).
//
// Outputs: the union of service keys across all readable files, and the
//
//	first hard parse error encountered (a file that exists but is not
//	valid YAML is a real problem, not a missing-file gap).
//
// Constraints: pure I/O + YAML parsing, no docker daemon access.
func ComposeServiceNames(composeFilePaths []string) (map[string]struct{}, error) {
	names := make(map[string]struct{})
	for _, path := range composeFilePaths {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading compose file %s: %w", path, err)
		}
		var doc minimalComposeFile
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parsing compose file %s: %w", path, err)
		}
		for name := range doc.Services {
			names[name] = struct{}{}
		}
	}
	return names, nil
}

// buildOrphanPsArgs returns the `docker ps` arguments that list every
// container (running or stopped) carrying this project's compose-project
// label, one tab-separated line per container: ID, Name, Service label,
// State. Separated from DetectOrphans so the exact filter/format can be
// pinned by a test without a live daemon.
func buildOrphanPsArgs(projectName string) []string {
	return []string{
		"ps", "-a",
		"--filter", fmt.Sprintf("label=%s=%s", composeProjectLabel, projectName),
		"--format", fmt.Sprintf(`{{.ID}}\t{{.Names}}\t{{.Label %q}}\t{{.State}}`, composeServiceLabel),
	}
}

// parseOrphanPsOutput parses buildOrphanPsArgs' tab-separated output and
// returns the containers whose service label is not in defined. A container
// with a blank service label (should not normally happen for a
// compose-created container, but never assume) is treated as an orphan too —
// reporting an unexpected container is always safer than silently ignoring
// it.
func parseOrphanPsOutput(raw string, defined map[string]struct{}) []OrphanContainer {
	var orphans []OrphanContainer
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		id, name, service, state := parts[0], parts[1], parts[2], parts[3]
		if _, ok := defined[service]; ok {
			continue
		}
		orphans = append(orphans, OrphanContainer{ID: id, Name: name, Service: service, State: state})
	}
	return orphans
}

// DetectOrphans lists every container belonging to projectName (by compose
// project label — see the package-level scoping note above) and returns the
// ones whose service is not present in defined. Returns an error only when
// the docker CLI itself could not be run (daemon unreachable, binary
// missing); callers should treat that as advisory-only and skip reporting
// rather than failing the caller's own command (a `nself build` run in a
// docker-less CI image must still succeed).
func DetectOrphans(ctx context.Context, projectName string, defined map[string]struct{}) ([]OrphanContainer, error) {
	cmd := exec.CommandContext(ctx, "docker", buildOrphanPsArgs(projectName)...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps (project=%s): %w", projectName, err)
	}
	return parseOrphanPsOutput(string(out), defined), nil
}

// RemoveOrphans force-removes each orphan container by ID (docker rm -f —
// same primitive cleanup.go's forceRemoveContainer uses for init/zombie
// containers) and returns one error per failed removal. Never called unless
// the caller opted in (e.g. `nself build --remove-orphans`); detection alone
// never removes anything.
func RemoveOrphans(ctx context.Context, orphans []OrphanContainer) []error {
	var errs []error
	for _, o := range orphans {
		if err := forceRemoveContainer(ctx, o.ID); err != nil {
			errs = append(errs, fmt.Errorf("removing orphan container %s (%s): %w", o.Name, o.Service, err))
		}
	}
	return errs
}
