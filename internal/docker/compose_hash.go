package docker

// compose_hash.go — compose config hashes and running-container hashes, the
// two inputs of the container-impact half of a change plan (P7-LIVE-03, EPIC
// D7).
//
// Purpose: `nself build --plan` must say which running services the next
// `nself start` would recreate. Docker Compose decides that by comparing the
// hash of a service's resolved configuration with the
// `com.docker.compose.config-hash` label of the running container, so the plan
// asks for the same two numbers and compares them.
// Inputs: the compose files and env files `nself start` passes, the project
// directory, and the compose project name.
// Outputs: service -> hash maps, the running services with their labels, and
// the set of services that mount a named volume.
// Constraints: every docker call stays in this package. Nothing here starts,
// stops or creates a container; `compose config` is client side and `docker
// ps` only reads.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// composeConfigHashLabel is the label Compose stamps with the hash of the
// resolved service configuration the container was created from.
const composeConfigHashLabel = "com.docker.compose.config-hash"

// ComposeConfigHashes runs `docker compose <-f ...> <--env-file ...>
// --project-directory <dir> config --hash '*'` and returns service -> hash.
//
// Inputs: the -f files and --env-file files in the order `start` passes them,
// and the project directory relative paths resolve against. Outputs: one entry
// per service in the merged configuration. A failure to run compose, or a
// compose too old to know --hash, is an error: the caller decides whether that
// means "impact unknown".
func ComposeConfigHashes(ctx context.Context, files, envFiles []string, projectDir string) (map[string]string, error) {
	args := []string{"compose"}
	for _, f := range files {
		args = append(args, "-f", f)
	}
	for _, e := range envFiles {
		args = append(args, "--env-file", e)
	}
	args = append(args, "--project-directory", projectDir, "config", "--hash", "*")
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.Output()
	if err != nil {
		detail := ""
		if ee, ok := err.(*exec.ExitError); ok {
			detail = ": " + lastLine(string(ee.Stderr))
		}
		return nil, fmt.Errorf("docker compose config --hash%s: %w", detail, err)
	}
	return parseConfigHashes(string(out))
}

// parseConfigHashes parses `service hash` lines. A line that is not exactly
// two fields is an error, never skipped: a silently dropped service would read
// as "no impact".
func parseConfigHashes(raw string) (map[string]string, error) {
	hashes := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 2 {
			return nil, fmt.Errorf("unexpected docker compose config --hash line %q", line)
		}
		hashes[f[0]] = f[1]
	}
	return hashes, nil
}

// RunningService is one container of the project as the daemon reports it.
type RunningService struct {
	Name       string
	Service    string
	State      string
	ConfigHash string // empty when the container has no config-hash label
}

// RunningConfigHashes lists every container (any state) carrying the compose
// project label, with its service and config-hash labels. An error means the
// daemon could not be asked.
func RunningConfigHashes(ctx context.Context, projectName string) ([]RunningService, error) {
	cmd := exec.CommandContext(ctx, "docker", runningPsArgs(projectName)...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps (project=%s): %w", projectName, err)
	}
	return parseRunningPs(string(out)), nil
}

// runningPsArgs is the `docker ps` invocation, pinned by a test.
func runningPsArgs(projectName string) []string {
	return []string{
		"ps", "-a",
		"--filter", fmt.Sprintf("label=%s=%s", composeProjectLabel, projectName),
		"--format", fmt.Sprintf(`{{.Names}}\t{{.State}}\t{{.Label %q}}\t{{.Label %q}}`, composeServiceLabel, composeConfigHashLabel),
	}
}

// parseRunningPs parses runningPsArgs' tab-separated output.
func parseRunningPs(raw string) []RunningService {
	var out []RunningService
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		p := strings.Split(line, "\t")
		for len(p) < 4 {
			p = append(p, "")
		}
		out = append(out, RunningService{Name: p[0], State: p[1], Service: p[2], ConfigHash: p[3]})
	}
	return out
}

// composeVolumes mirrors the `volumes` key of one service: short strings or
// long-form maps.
type composeVolumes struct {
	Services map[string]struct {
		Volumes []yaml.Node `yaml:"volumes"`
	} `yaml:"services"`
}

// StatefulServices returns the services that mount a named volume in any of
// the compose files (EPIC D7: a stateful service is one whose data lives in a
// named volume). A bind mount (a path) or tmpfs is not stateful. Missing files
// are skipped; a file that is not valid YAML is an error.
func StatefulServices(files []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading compose file %s: %w", path, err)
		}
		var doc composeVolumes
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parsing compose file %s: %w", path, err)
		}
		for name, svc := range doc.Services {
			for i := range svc.Volumes {
				if mountsNamedVolume(&svc.Volumes[i]) {
					out[name] = true
				}
			}
		}
	}
	return out, nil
}

// mountsNamedVolume reports whether one volumes entry names a volume rather
// than a host path.
func mountsNamedVolume(n *yaml.Node) bool {
	switch n.Kind {
	case yaml.ScalarNode:
		src, _, hasTarget := strings.Cut(n.Value, ":")
		return isNamedSource(src, hasTarget)
	case yaml.MappingNode:
		var long struct {
			Type   string `yaml:"type"`
			Source string `yaml:"source"`
		}
		if err := n.Decode(&long); err != nil {
			return false
		}
		return long.Type == "volume" && long.Source != ""
	}
	return false
}

// isNamedSource reports whether the source of a short volume entry is a volume
// name: a short entry without ":" is an anonymous volume (no name), and a
// source starting with ".", "/", "~" or a drive letter path is a bind mount.
func isNamedSource(src string, hasTarget bool) bool {
	if !hasTarget || src == "" {
		return false
	}
	if strings.HasPrefix(src, ".") || strings.HasPrefix(src, "/") || strings.HasPrefix(src, "~") ||
		strings.HasPrefix(src, "$") || strings.Contains(src, "/") || strings.Contains(src, `\`) {
		return false
	}
	return true
}
