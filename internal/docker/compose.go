package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Compose shells out to `docker compose` for lifecycle operations.
type Compose struct {
	// DockerPath overrides the docker binary location. Empty uses PATH lookup.
	DockerPath string

	// ComposeFiles lists compose file paths passed as -f flags.
	// Order matters: first file is the base, subsequent files extend/override.
	// If empty, no -f flags are added and Docker uses its default discovery.
	ComposeFiles []string

	// EnvFiles lists env file paths passed as --env-file flags for ${VAR}
	// interpolation (secrets are env-templated out of the generated compose
	// YAML — see internal/build secret templating). Order matters: later
	// files win on conflict. If empty, Docker Compose falls back to its
	// default .env discovery.
	EnvFiles []string
}

// NewCompose returns a Compose instance configured with the given compose files.
// If no files are provided, Docker Compose uses its default file discovery.
func NewCompose(files ...string) *Compose {
	return &Compose{ComposeFiles: files}
}

// dockerBin returns the path to the docker binary.
func (c *Compose) dockerBin() string {
	if c.DockerPath != "" {
		return c.DockerPath
	}
	return "docker"
}

// buildBaseArgs returns the base command arguments including any -f file flags.
// The returned slice always starts with "compose" followed by zero or more
// "-f <path>" pairs.
func (c *Compose) buildBaseArgs() []string {
	args := []string{"compose"}
	for _, f := range c.ComposeFiles {
		args = append(args, "-f", f)
	}
	for _, e := range c.EnvFiles {
		args = append(args, "--env-file", e)
	}
	return args
}

// withServices appends "--" and then services to args, so a service name that
// starts with "-" is never read as a flag. With no services it appends nothing
// (no trailing "--"): the command then acts on every service.
func withServices(args []string, services ...string) []string {
	if len(services) == 0 {
		return args
	}
	return append(append(args, "--"), services...)
}

// ComposeUp runs `docker compose up -d [services...]` in the given workdir.
func (c *Compose) ComposeUp(ctx context.Context, workdir string, services ...string) error {
	args := c.buildBaseArgs()
	args = withServices(append(args, "up", "-d"), services...)
	return c.Run(ctx, workdir, args...)
}

// ComposeDown runs `docker compose down` with the given options in the given workdir.
func (c *Compose) ComposeDown(ctx context.Context, workdir string, opts DownOptions) error {
	args := c.buildBaseArgs()
	args = append(args, "down")
	if opts.RemoveVolumes {
		args = append(args, "-v")
	}
	if opts.RemoveOrphans {
		args = append(args, "--remove-orphans")
	}
	if opts.Timeout > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", opts.Timeout))
	}
	return c.Run(ctx, workdir, args...)
}

// ComposeUpNoDeps runs `docker compose up -d --no-deps [services...]`.
//
// This is the primitive that means "make these containers match the compose
// file": Compose recreates a container whose definition changed and leaves an
// unchanged one alone. `restart` does neither — it bounces the existing
// container and never re-reads the file. --no-deps keeps the operation scoped
// to the named services instead of pulling their dependencies up with them.
func (c *Compose) ComposeUpNoDeps(ctx context.Context, workdir string, services ...string) error {
	args := c.buildBaseArgs()
	args = withServices(append(args, "up", "-d", "--no-deps"), services...)
	return c.Run(ctx, workdir, args...)
}

// ComposeRestart runs `docker compose restart [services...]` in the given workdir.
func (c *Compose) ComposeRestart(ctx context.Context, workdir string, services ...string) error {
	args := c.buildBaseArgs()
	args = withServices(append(args, "restart"), services...)
	return c.Run(ctx, workdir, args...)
}

// ComposePull runs `docker compose pull` in the given workdir, pulling images
// for all services defined in the compose file.
func (c *Compose) ComposePull(ctx context.Context, workdir string) error {
	args := c.buildBaseArgs()
	args = append(args, "pull")
	return c.Run(ctx, workdir, args...)
}

// ComposePullService runs `docker compose pull <service>` in the given workdir,
// pulling a fresh image only for the named service.
func (c *Compose) ComposePullService(ctx context.Context, workdir, service string) error {
	args := c.buildBaseArgs()
	args = withServices(append(args, "pull"), service)
	return c.Run(ctx, workdir, args...)
}

// ComposeStop runs `docker compose stop [services...]` in the given workdir.
// Unlike ComposeDown, this stops containers without removing them, preserving
// their state for a subsequent ComposeUp.
func (c *Compose) ComposeStop(ctx context.Context, workdir string, services ...string) error {
	args := c.buildBaseArgs()
	args = withServices(append(args, "stop"), services...)
	return c.Run(ctx, workdir, args...)
}

// ComposeScale runs `docker compose up -d --scale <service>=<n>` in the given
// workdir, adjusting the replica count for the named service.
func (c *Compose) ComposeScale(ctx context.Context, workdir, service string, replicas int) error {
	args := c.buildBaseArgs()
	args = withServices(append(args, "up", "-d", "--no-deps",
		fmt.Sprintf("--scale=%s=%d", service, replicas)), service)
	return c.Run(ctx, workdir, args...)
}

// composePsEntry maps the JSON output of `docker compose ps --format json`.
// Docker Compose v2 emits one JSON object per line (not an array).
type composePsEntry struct {
	ID      string `json:"ID"`
	Name    string `json:"Name"`
	Service string `json:"Service"`
	State   string `json:"State"`
	Health  string `json:"Health"`
	Image   string `json:"Image"`
	Status  string `json:"Status"`
	Ports   string `json:"Ports"`
}

// ComposePs runs `docker compose ps --format json` and parses the output
// into a slice of ContainerInfo.
//
// Docker Compose v2 changed its JSON output format across versions:
//   - v2.20 and earlier: one JSON object per stdout line (NDJSON)
//   - v2.21 and later:   a single JSON array containing all objects
//
// Both formats are handled: the raw output is first attempted as a JSON array;
// if that fails (or yields nothing), the output is re-scanned line by line as
// NDJSON. This makes the function version-agnostic.
func (c *Compose) ComposePs(ctx context.Context, workdir string) ([]ContainerInfo, error) {
	args := c.buildBaseArgs()
	args = append(args, "ps", "--format", "json")
	cmd := exec.CommandContext(ctx, c.dockerBin(), args...)
	cmd.Dir = workdir

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose ps: %w", err)
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}

	// Attempt 1: JSON array format (Docker Compose v2.21+).
	if strings.HasPrefix(raw, "[") {
		var entries []composePsEntry
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return nil, fmt.Errorf("parsing compose ps JSON array: %w", err)
		}
		return composePsEntriesToInfos(entries), nil
	}

	// Attempt 2: NDJSON format (Docker Compose v2.20 and earlier) —
	// one JSON object per line.
	var infos []ContainerInfo
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry composePsEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("parsing compose ps JSON: %w", err)
		}
		infos = append(infos, composePsEntryToInfo(entry))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading compose ps output: %w", err)
	}
	return infos, nil
}
