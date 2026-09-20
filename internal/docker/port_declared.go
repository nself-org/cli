package docker

// Purpose: Determine the host ports a stack will actually bind, by reading the
//   resolved compose configuration rather than assuming nSelf defaults.
// Inputs:  workdir + the compose files the caller is about to bring up.
// Outputs: sorted host ports, and a port -> service name map for diagnostics.
// Constraints: no side effects; `docker compose config` does not start anything.
//   Never returns an empty list silently on failure — the caller must be able to
//   tell "this stack binds nothing" from "we could not work it out", because
//   checking an empty list would be a port check that cannot fail.
// SPORT: CLI-CMD-START-001

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// composeConfigPort mirrors the `ports` entries in `docker compose config
// --format json`. Compose normalises every short-form ("5433:5432",
// "127.0.0.1:5433:5432") into this long form, so only this shape is parsed.
type composeConfigPort struct {
	Published any    `json:"published"` // string in most versions, number in some
	Target    int    `json:"target"`
	Protocol  string `json:"protocol"`
	HostIP    string `json:"host_ip"`
}

type composeConfigDoc struct {
	Services map[string]struct {
		Ports []composeConfigPort `json:"ports"`
	} `json:"services"`
}

// publishedPort normalises the published field, which Compose emits as a string
// in most versions and as a number in others. Ranges ("8000-8010") take the
// first port; a range that partially conflicts is still a conflict.
func (p composeConfigPort) publishedPort() int {
	var s string
	switch v := p.Published.(type) {
	case string:
		s = v
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
	if s == "" {
		return 0
	}
	if idx := strings.Index(s, "-"); idx > 0 {
		s = s[:idx]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// DeclaredHostPorts returns the host ports the given compose files publish,
// together with a port -> service name map for error messages.
//
// This exists because checking a fixed list of default ports is wrong for any
// stack that was configured off the defaults. A second nSelf project on one
// host must move its ports (POSTGRES_PORT=5433, HASURA_PORT=8181, ...); with a
// fixed list it is then blocked by the first project holding 5432 and 8080,
// ports it was never going to bind. That is what kept a staging stack down.
//
// An error here means "could not determine", never "binds nothing". The caller
// must fall back to a conservative default list rather than check nothing.
// envFiles are passed as --env-file. Without them an installed plugin's
// docker-compose.plugin.yml cannot resolve ${DOCKER_NETWORK} and docker
// rejects the project as invalid, so this returns an error and the caller
// silently falls back to the default port list.
func DeclaredHostPorts(ctx context.Context, workdir string, envFiles []string, composeFiles ...string) ([]int, map[int]string, error) {
	c := NewCompose(composeFiles...)
	c.EnvFiles = envFiles
	args := c.buildBaseArgs()
	args = append(args, "config", "--format", "json")

	cmd := exec.CommandContext(ctx, c.dockerBin(), args...)
	cmd.Dir = workdir
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("docker compose config: %w", err)
	}

	var doc composeConfigDoc
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, nil, fmt.Errorf("parsing compose config JSON: %w", err)
	}
	if len(doc.Services) == 0 {
		return nil, nil, fmt.Errorf("compose config listed no services")
	}

	ports, names := collectPublishedPorts(doc)
	return ports, names, nil
}

// collectPublishedPorts turns a parsed compose config into the sorted set of
// TCP host ports and a port -> service name map.
//
// Split out of DeclaredHostPorts so tests exercise THIS function rather than
// re-implementing the same loop. A test that reproduces the logic it is
// checking passes against broken production code, which is how a hardcoded
// flag list survived here for months.
func collectPublishedPorts(doc composeConfigDoc) ([]int, map[int]string) {
	names := make(map[int]string)
	seen := make(map[int]bool)
	for svc, s := range doc.Services {
		for _, p := range s.Ports {
			// UDP publishes do not collide with the TCP probe used by
			// CheckPort, so including them would produce false conflicts.
			if p.Protocol != "" && !strings.EqualFold(p.Protocol, "tcp") {
				continue
			}
			hp := p.publishedPort()
			if hp <= 0 || seen[hp] {
				continue
			}
			seen[hp] = true
			names[hp] = svc
		}
	}

	ports := make([]int, 0, len(seen))
	for p := range seen {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	return ports, names
}

// DefaultPortServiceNames maps the ReservedPorts defaults to human names.
// Used only on the fallback path, when the published ports could not be read
// from compose; the normal path names services from the compose config itself,
// so a project on custom ports still gets "Port 8181 (hasura)" and not
// "unknown service".
func DefaultPortServiceNames() map[int]string {
	return map[int]string{
		80:   "HTTP (Nginx)",
		443:  "HTTPS (Nginx)",
		5432: "PostgreSQL",
		8080: "Hasura GraphQL",
		4000: "Auth",
		6379: "Redis",
		9000: "MinIO",
		9001: "MinIO Console",
		7700: "MeiliSearch",
		3021: "nSelf Admin",
		1025: "Mailpit SMTP",
		8025: "Mailpit UI",
		3008: "Functions",
		5000: "MLflow",
	}
}
