package seed

// plugin.go — run a plugin's declared seed argv inside the plugin's service.
//
// Purpose: manifest v2 `seed.command` is an idempotent argv. RunPluginSeed
// runs it through the docker funnel (docker exec, argv form, no shell, no
// host process) inside the running container of the plugin's first compose
// service. Prod-class refusal and the JSON envelope live in the command layer.
// Inputs: a PluginTarget (plugin name, compose service, argv) and a
// PluginRuntime (the container lookup and exec seam; DockerRuntime is real).
// Outputs: the command's stdout; coded errors (E127 no command, E252 no
// running container, E250 the command failed).
// Constraints: this package never imports internal/plugin (layer L2); the
// caller loads the manifest and hands over the argv.

import (
	"context"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"gopkg.in/yaml.v3"
)

// PluginTarget identifies one plugin seed to run.
type PluginTarget struct {
	Name    string   // plugin name, for messages
	Service string   // compose service whose container runs the argv
	Argv    []string // manifest seed.command
}

// PluginRuntime is the container seam: find the running container of a
// compose service, and run an argv in it.
type PluginRuntime interface {
	FindContainer(ctx context.Context, service string) (string, error)
	Exec(ctx context.Context, container string, argv []string) (stdout, stderr string, err error)
}

// dockerRuntime is the real runtime, backed by the docker funnel.
type dockerRuntime struct {
	workingDirs []string
	project     string
}

// DockerRuntime returns the runtime that finds the plugin service container by
// compose labels (project working dirs or project name) and execs through
// internal/docker.
func DockerRuntime(projectDir, projectName string) PluginRuntime {
	return dockerRuntime{workingDirs: []string{projectDir}, project: projectName}
}

func (d dockerRuntime) FindContainer(ctx context.Context, service string) (string, error) {
	return docker.FindServiceContainer(ctx, docker.ServiceMatch{
		Service: service, WorkingDirs: d.workingDirs, Project: d.project,
	})
}

func (d dockerRuntime) Exec(ctx context.Context, container string, argv []string) (string, string, error) {
	return docker.ExecCapture(ctx, container, argv)
}

// ValidatePluginArgv returns E127 when the plugin declares no usable seed
// argv: none at all, or an empty program or empty argument element.
func ValidatePluginArgv(plugin string, argv []string) error {
	if len(argv) == 0 {
		return errs.Newf("E127", "plugin %q declares no seed command", plugin)
	}
	for _, a := range argv {
		if strings.TrimSpace(a) == "" {
			return errs.Newf("E127", "plugin %q seed.command has an empty element", plugin)
		}
	}
	return nil
}

// RunPluginSeed runs t.Argv in the container of t.Service. The argv reaches
// docker exec as separate arguments; nothing is interpolated or shell-parsed.
func RunPluginSeed(ctx context.Context, rt PluginRuntime, t PluginTarget) (string, error) {
	if err := ValidatePluginArgv(t.Name, t.Argv); err != nil {
		return "", err
	}
	if t.Service == "" {
		return "", errs.Newf("E252", "plugin %q has no compose service to seed", t.Name)
	}
	container, err := rt.FindContainer(ctx, t.Service)
	if err != nil {
		return "", errs.Wrap("E252", "plugin "+t.Name+" service "+t.Service+" is not running", err).
			WithFix("Start the stack with `nself start`, then run the seed again.")
	}
	stdout, stderr, err := rt.Exec(ctx, container, t.Argv)
	if err != nil {
		return stdout, errs.Wrap("E250", "plugin "+t.Name+" seed command failed", err).
			WithWhy(lastNonEmpty(stderr, err.Error())).
			WithFix("Run the seed command by hand in the container and read its output.")
	}
	return stdout, nil
}

// FirstComposeService returns the first service key (document order) of a
// plugin's docker-compose.plugin.yml.
func FirstComposeService(fragmentPath string) (string, error) {
	data, err := os.ReadFile(fragmentPath)
	if err != nil {
		return "", errs.Wrap("E252", "reading the plugin compose fragment", err)
	}
	var doc struct {
		Services yaml.Node `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", errs.Wrap("E106", "parsing the plugin compose fragment "+fragmentPath, err)
	}
	if doc.Services.Kind != yaml.MappingNode || len(doc.Services.Content) < 2 {
		return "", errs.Newf("E252", "plugin compose fragment %s declares no service", fragmentPath)
	}
	return doc.Services.Content[0].Value, nil
}

// lastNonEmpty returns the last non-empty line of s, or fallback.
func lastNonEmpty(s, fallback string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}
	return fallback
}
