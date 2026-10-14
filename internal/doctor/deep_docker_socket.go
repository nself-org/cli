package doctor

// deep_docker_socket.go — P7-DEPL-21 (ADR 0027 §3): `nself doctor --deep`
// check proxy-docker-socket. A running proxy (nginx, traefik) or plugin
// container that holds the Docker socket, by bind mount or a unix DOCKER_HOST,
// fails the check and is named. A plugin in the build guard's exception table
// (build.ActiveSocketException, the same table the build refuses against)
// warns until its removal version.
// Inputs:  the project dir; the running containers from the docker package's
//          compose ps and inspect (behind SocketInspector, faked in tests).
// Outputs: one CheckResult: pass, warn, fail, or unknown.
// Constraints: read-only. An inspection error is "unknown", never "pass"
//          (Constitution §4.1). The core admin service and custom services
//          are not proxy or plugin containers and are not checked.

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/docker"
)

const dockerSocketCheckName = "proxy-docker-socket"

// proxyServices are the compose services that front the stack.
var proxyServices = map[string]bool{"nginx": true, "traefik": true}

// SocketInspector returns the running containers of the project with Service,
// Mounts and Env filled.
type SocketInspector interface {
	RunningContainers(ctx context.Context) ([]docker.ContainerInfo, error)
}

// composeInspector lists through `docker compose ps`, then inspects each one.
type composeInspector struct {
	dir      string
	files    []string
	envFiles []string
}

func (c composeInspector) RunningContainers(ctx context.Context) ([]docker.ContainerInfo, error) {
	cmp := docker.NewCompose(c.files...)
	cmp.EnvFiles = c.envFiles
	listed, err := cmp.ComposePs(ctx, c.dir)
	if err != nil {
		return nil, err
	}
	var out []docker.ContainerInfo
	for _, l := range listed {
		if l.State != "running" {
			continue
		}
		info, err := docker.InspectContainer(ctx, l.Name)
		if err != nil {
			return nil, err
		}
		info.Service = l.Service
		out = append(out, *info)
	}
	return out, nil
}

// DockerSocketChecks is the DeepChecks entry point.
func DockerSocketChecks(ctx context.Context, projectDir string) []CheckResult {
	if _, err := os.Stat(filepath.Join(projectDir, "docker-compose.yml")); err != nil {
		return []CheckResult{socketResult("skip", "no docker-compose.yml in this project", "")}
	}
	files, err := build.ReadComposeManifest(projectDir)
	if err != nil {
		return []CheckResult{socketResult("unknown", "cannot read the compose manifest: "+err.Error(), "nself build")}
	}
	envFiles, err := build.ReadComposeEnvManifest(projectDir)
	if err != nil {
		return []CheckResult{socketResult("unknown", "cannot read the compose env manifest: "+err.Error(), "nself build")}
	}
	ins := composeInspector{dir: projectDir, files: files, envFiles: envFiles}
	return []CheckResult{CheckProxyDockerSocket(ctx, ins, build.PluginFragmentServices(build.DefaultPluginDir()))}
}

func socketResult(status, msg, fix string) CheckResult {
	return CheckResult{Section: "docker", Name: dockerSocketCheckName, Status: status, Message: msg, FixCmd: fix}
}

// CheckProxyDockerSocket judges the running containers. owners maps a compose
// service name to the plugin slug that declares it.
func CheckProxyDockerSocket(ctx context.Context, ins SocketInspector, owners map[string]string) CheckResult {
	containers, err := ins.RunningContainers(ctx)
	if err != nil {
		return socketResult("unknown", "cannot inspect containers, so the Docker socket exposure is unknown: "+err.Error(), "docker ps")
	}
	var failed, allowed []string
	for _, c := range containers {
		service := c.Service
		if service == "" {
			service = c.Labels["com.docker.compose.service"]
		}
		slug := owners[service]
		if !proxyServices[service] && slug == "" {
			continue
		}
		for _, detail := range containerSocketExposure(c) {
			who := fmt.Sprintf("proxy %s (container %s)", service, c.Name)
			if slug != "" {
				who = fmt.Sprintf("plugin %s service %s (container %s)", slug, service, c.Name)
			}
			if ex, ok := build.ActiveSocketException(slug); ok {
				allowed = append(allowed, fmt.Sprintf("%s %s; allowed until nself %s (%s)", who, detail, ex.RemovalAt, ex.Ticket))
				continue
			}
			failed = append(failed, who+" "+detail)
		}
	}
	sort.Strings(failed)
	sort.Strings(allowed)
	switch {
	case len(failed) > 0:
		return socketResult("fail", "Docker socket exposed (ADR 0027): "+strings.Join(append(failed, allowed...), "; "),
			"remove the socket mount and unix DOCKER_HOST from the service, then nself build && nself start")
	case len(allowed) > 0:
		return socketResult("warn", "excepted until removal: "+strings.Join(allowed, "; "), "")
	}
	return socketResult("pass", "no proxy or plugin container holds the Docker socket", "")
}

// containerSocketExposure lists how one container reaches the socket.
func containerSocketExposure(c docker.ContainerInfo) []string {
	var out []string
	for _, m := range c.Mounts {
		src := strings.ToLower(m.Source)
		switch path.Clean(m.Source) {
		case "/", "/var", "/var/run", "/run":
			out = append(out, fmt.Sprintf("mounts %s", m.Source))
		default:
			if strings.Contains(src, "docker.sock") || strings.Contains(strings.ToLower(m.Destination), "docker.sock") {
				out = append(out, fmt.Sprintf("mounts the Docker socket (%s)", m.Source))
			}
		}
	}
	if v := c.Env["DOCKER_HOST"]; strings.Contains(strings.ToLower(v), "unix:") {
		out = append(out, fmt.Sprintf("sets DOCKER_HOST=%s", v))
	}
	return out
}
