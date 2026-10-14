package doctor

// deep_docker_socket.go — P7-DEPL-21 (ADR 0027 §3): `nself doctor --deep`
// check proxy-docker-socket. A live proxy (nginx, traefik) or plugin container
// that holds the Docker socket (bind mount, its parent directory, a
// socket-backed named volume, or a unix DOCKER_HOST) fails and is named.
// Inputs:  the project dir; the live containers from the docker package's
//          compose ps and inspect (behind SocketInspector, faked in tests);
//          build.AnalyzeSocket's view of the compose inputs.
// Outputs: one CheckResult: fail > unknown > warn > pass (skip: no project).
// Constraints: read-only. Plugin containers are every non-proxy service the
//          core stack does not declare, so a deleted or disabled fragment
//          cannot hide one; the exception table is build's own, applies to a
//          plugin the fragments name exactly once, never to a proxy. An
//          inspection error, a malformed field or any volume mount is
//          "unknown", never "pass" (Constitution §4.1).

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

// SocketInspector returns the live (not exited) containers of the project with
// Service, State, Mounts and Env filled.
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
		if l.Name == "" || l.Service == "" || l.State == "" {
			return nil, fmt.Errorf("compose ps returned an entry without name, service or state: %+v", l)
		}
		if l.State == "exited" || l.State == "dead" || l.State == "removing" {
			continue
		}
		info, err := docker.InspectContainer(ctx, l.Name)
		if err != nil {
			return nil, err
		}
		if info == nil || info.ID == "" || info.Name == "" {
			return nil, fmt.Errorf("docker inspect %s returned no container", l.Name)
		}
		info.Service = l.Service
		out = append(out, *info)
	}
	return out, nil
}

// DockerSocketChecks is the DeepChecks entry point.
func DockerSocketChecks(ctx context.Context, projectDir string) []CheckResult {
	core, err := os.ReadFile(filepath.Join(projectDir, "docker-compose.yml"))
	if os.IsNotExist(err) {
		return []CheckResult{socketResult("skip", "no docker-compose.yml in this project", "")}
	}
	pluginDir := build.DefaultPluginDir()
	var rep *build.SocketReport
	var files, envFiles []string
	if err == nil {
		rep, err = build.AnalyzeSocket(projectDir, pluginDir, build.NewSocketEnv(projectDir, pluginDir, nil), core)
	}
	if err == nil {
		files, err = build.ReadComposeManifest(projectDir)
	}
	if err == nil {
		envFiles, err = build.ReadComposeEnvManifest(projectDir)
	}
	if err != nil {
		return []CheckResult{socketResult("unknown", "cannot read the compose inputs, so the Docker socket exposure is unknown: "+err.Error(), "nself build")}
	}
	return []CheckResult{CheckProxyDockerSocket(ctx, composeInspector{projectDir, files, envFiles}, rep)}
}

func socketResult(status, msg, fix string) CheckResult {
	return CheckResult{Section: "docker", Name: dockerSocketCheckName, Status: status, Message: msg, FixCmd: fix}
}

// CheckProxyDockerSocket judges the live containers against rep.
func CheckProxyDockerSocket(ctx context.Context, ins SocketInspector, rep *build.SocketReport) CheckResult {
	containers, err := ins.RunningContainers(ctx)
	if err != nil {
		return socketResult("unknown", "cannot inspect containers, so the Docker socket exposure is unknown: "+err.Error(), "docker ps")
	}
	var failed, unknown, allowed []string
	for _, c := range containers {
		service := c.Service
		if service == "" {
			service = c.Labels["com.docker.compose.service"]
		}
		if service == "" || c.Name == "" || c.State == "" {
			unknown = append(unknown, fmt.Sprintf("container %q has no service, name or state", c.Name))
			continue
		}
		owners, label := rep.Owners[service], c.Labels["nself.plugin"]
		proxy := build.SocketProxyService(service)
		if !proxy && label == "" && len(owners) == 0 && rep.Core[service] {
			continue // a core or custom service: not judged
		}
		who := fmt.Sprintf("service %s (container %s)", service, c.Name)
		switch {
		case proxy:
			who = "proxy " + who
		case label != "":
			who = fmt.Sprintf("plugin %s %s", label, who)
		case len(owners) > 0:
			who = fmt.Sprintf("plugin %s %s", strings.Join(owners, ", "), who)
		default:
			who = "plugin (no fragment names it) " + who
		}
		found, unk := containerSocketExposure(c, rep)
		for _, u := range unk {
			unknown = append(unknown, who+" "+u)
		}
		for _, detail := range found {
			// An exception needs the fragments to name exactly one owner, and a
			// label (which the plugin itself writes) may not name another.
			ex, ok := build.ActiveSocketException(strings.Join(owners, ""))
			if ok && !proxy && len(owners) == 1 && (label == "" || label == owners[0]) {
				allowed = append(allowed, fmt.Sprintf("%s %s; allowed until nself %s (%s)", who, detail, ex.RemovalAt, ex.Ticket))
				continue
			}
			failed = append(failed, who+" "+detail)
		}
	}
	for _, l := range [][]string{failed, unknown, allowed} {
		sort.Strings(l)
	}
	switch {
	case len(failed) > 0:
		return socketResult("fail", "Docker socket exposed (ADR 0027): "+strings.Join(append(failed, append(unknown, allowed...)...), "; "),
			"remove the socket mount and unix DOCKER_HOST from the service, then nself build && nself start")
	case len(unknown) > 0:
		return socketResult("unknown", "cannot tell whether the Docker socket is exposed: "+strings.Join(append(unknown, allowed...), "; "), "nself build && nself start")
	case len(allowed) > 0:
		return socketResult("warn", "excepted until removal: "+strings.Join(allowed, "; "), "")
	}
	return socketResult("pass", "no proxy or plugin container holds the Docker socket", "")
}

// remoteDocker reports a DOCKER_HOST that is not a local unix socket.
func remoteDocker() bool {
	h := strings.ToLower(os.Getenv("DOCKER_HOST"))
	return h != "" && !strings.HasPrefix(h, "unix:")
}

// containerSocketExposure lists how one live container reaches the socket,
// and what could not be established.
func containerSocketExposure(c docker.ContainerInfo, rep *build.SocketReport) (found, unknown []string) {
	for _, m := range c.Mounts {
		if path.Base(m.Destination) == "docker.sock" {
			found = append(found, fmt.Sprintf("mounts the Docker socket at %s", m.Destination))
		}
		switch m.Type {
		case "bind":
			// A bind source is a host path; follow symlinks where this host can
			// see it. A missing, dangling or relative source, or a remote
			// DOCKER_HOST (whose paths are not ours), cannot be verified.
			r, ok := build.ResolveSocketPath("", m.Source)
			_, statErr := os.Lstat(m.Source)
			switch {
			case build.SocketPathExposed(m.Source) || ok && build.SocketPathExposed(r):
				found = append(found, fmt.Sprintf("mounts the Docker socket (%s)", m.Source))
			case !ok || statErr != nil || remoteDocker():
				unknown = append(unknown, fmt.Sprintf("has a bind source %q that cannot be verified on this host", m.Source))
			}
		case "volume":
			name, ok := strings.CutPrefix(filepath.ToSlash(m.Source), "/var/lib/docker/volumes/")
			name, _, _ = strings.Cut(name, "/")
			if !ok || name == "" {
				unknown = append(unknown, fmt.Sprintf("has a volume mount %q whose volume cannot be identified", m.Source))
				break
			}
			// The docker package cannot show a volume's live backing, so a
			// Compose declaration (or a name) never proves it safe: only a
			// declared socket-backed volume is a definite exposure.
			for logical, why := range rep.Volumes {
				if strings.HasPrefix(why, "is backed by") && (name == logical || strings.HasSuffix(name, "_"+logical)) {
					found = append(found, fmt.Sprintf("mounts named volume %s, which %s", name, why))
				}
			}
			unknown = append(unknown, fmt.Sprintf("mounts volume %s, whose live backing cannot be verified", name))
		case "tmpfs", "image":
		default:
			unknown = append(unknown, fmt.Sprintf("has a mount of unknown type %q", m.Type))
		}
	}
	if v := c.Env["DOCKER_HOST"]; strings.Contains(strings.ToLower(v), "unix:") {
		found = append(found, fmt.Sprintf("sets DOCKER_HOST=%s", v))
	}
	return found, unknown
}
