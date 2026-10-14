package doctor

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/version"
)

const dockerSocketCheckName = "proxy-docker-socket"

type SocketInspector interface {
	RunningContainers(ctx context.Context) ([]docker.ContainerInfo, error)
}

type evidenceCertifier interface{ evidenceComplete() bool }

func DockerSocketChecks(ctx context.Context, projectDir string) []CheckResult {
	core, err := os.ReadFile(filepath.Join(projectDir, "docker-compose.yml"))
	if os.IsNotExist(err) {
		return []CheckResult{socketResult("skip", "no docker-compose.yml in this project", "")}
	}
	pluginDir := os.Getenv("NSELF_PLUGIN_DIR")
	if pluginDir == "" {
		home, _ := os.UserHomeDir()
		pluginDir = filepath.Join(home, ".nself", "plugins")
	}
	var rep *socketInventory
	var files, envFiles []string
	if err == nil {
		rep, err = readSocketInventory(projectDir, pluginDir, core)
	}
	if err == nil {
		files, err = socketManifest(projectDir, "compose-files.txt")
	}
	if err == nil {
		envFiles, err = socketManifest(projectDir, "compose-env-files.txt")
	}
	if err != nil {
		return []CheckResult{socketResult("unknown", "cannot read the compose inputs, so the Docker socket exposure is unknown: "+err.Error(), "nself build")}
	}
	return []CheckResult{CheckProxyDockerSocket(ctx, composeInspector{projectDir, files, envFiles}, rep)}
}

func socketResult(status, msg, fix string) CheckResult {
	return CheckResult{Section: "docker", Name: dockerSocketCheckName, Status: status, Message: msg, FixCmd: fix}
}

func CheckProxyDockerSocket(ctx context.Context, ins SocketInspector, rep *socketInventory) CheckResult {
	containers, err := ins.RunningContainers(ctx)
	if err != nil {
		return socketResult("unknown", "cannot inspect containers, so the Docker socket exposure is unknown: "+err.Error(), "docker ps")
	}
	certifier, _ := ins.(evidenceCertifier)
	certified := certifier != nil && certifier.evidenceComplete()
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
		proxy := socketProxyService(service)
		if !proxy && label == "" && len(owners) == 0 && isCoreService(rep, service) {
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
		if !certified && (len(c.Env) == 0 || len(c.Mounts) == 0) {
			unk = append(unk, "has an empty Config.Env or Mounts in its inspect output, which cannot prove the field was present")
		}
		for _, u := range unk {
			unknown = append(unknown, who+" "+u)
		}
		for _, detail := range found {
			ex, ok := activeSocketException(strings.Join(owners, ""))
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

func isCoreService(rep *socketInventory, name string) bool {
	for _, e := range compose.ServiceCatalog() {
		if e.Name == name {
			return true
		}
	}
	return rep.Core[name]
}

func remoteDocker() bool {
	h := strings.ToLower(os.Getenv("DOCKER_HOST"))
	return h != "" && !strings.HasPrefix(h, "unix:")
}

func containerSocketExposure(c docker.ContainerInfo, rep *socketInventory) (found, unknown []string) {
	for _, m := range c.Mounts {
		if m.Destination == "" {
			unknown = append(unknown, fmt.Sprintf("has a %q mount without a destination", m.Type))
			continue
		}
		if path.Base(m.Destination) == "docker.sock" {
			found = append(found, fmt.Sprintf("mounts the Docker socket at %s", m.Destination))
		}
		switch m.Type {
		case "bind":
			if m.Source == "" {
				unknown = append(unknown, "has a bind mount without a source")
				break
			}
			r, ok := resolveSocketPath("", m.Source)
			_, statErr := os.Lstat(m.Source)
			switch {
			case socketPathExposed(m.Source) || ok && socketPathExposed(r):
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

type socketException struct{ Slug, Ticket, RemovalAt string }

func activeSocketException(slug string) (socketException, bool) {
	ex, ok := map[string]socketException{"forgejo": {"forgejo", "P7-TRUST-21", "1.5.0"}, "monitoring": {"monitoring", "P7-PLUG-58", "1.5.0"}}[slug]
	var c, t [3]int
	nc, _ := fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(version.GetVersion()), "v"), "%d.%d.%d", &c[0], &c[1], &c[2])
	nt, _ := fmt.Sscanf(ex.RemovalAt, "%d.%d.%d", &t[0], &t[1], &t[2])
	return ex, ok && nc == 3 && nt == 3 && (c[0] < t[0] || c[0] == t[0] && (c[1] < t[1] || c[1] == t[1] && c[2] < t[2]))
}

type socketInventory struct {
	Owners  map[string][]string
	Core    map[string]bool
	Volumes map[string]string
}

func socketManifest(dir, name string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".nself", name))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	if name == "compose-env-files.txt" {
		if len(files) == 0 {
			if _, err := os.Lstat(filepath.Join(dir, ".nself", "compose.env")); !os.IsNotExist(err) {
				return nil, fmt.Errorf("missing compose env manifest; run nself build")
			}
		}
		return files, nil
	}
	if len(files) == 0 {
		files = []string{"docker-compose.yml"}
	}
	override := filepath.Join(dir, "docker-compose.override.yml")
	if _, err := os.Stat(override); err == nil {
		for _, file := range files {
			if filepath.Base(file) == filepath.Base(override) {
				return files, nil
			}
		}
		files = append(files, override)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return files, nil
}

func socketProxyService(n string) bool { return n == "nginx" || n == "traefik" }

func socketPathExposed(p string) bool {
	c := strings.TrimPrefix(path.Clean(strings.ToLower(strings.TrimSpace(p))), "/private")
	switch c {
	case "", "/", "/var", "/var/run", "/run":
		return true
	}
	return path.Base(c) == "docker.sock"
}

func resolveSocketPath(base, p string) (string, bool) {
	if rest, cut := strings.CutPrefix(p, "~"); cut && (rest == "" || rest[0] == '/') {
		h, _ := os.UserHomeDir()
		p = h + rest
	}
	if !filepath.IsAbs(p) {
		if base == "" {
			return "", false
		}
		p = base + string(filepath.Separator) + p
	}
	for rest := ""; ; {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, rest), true
		} else if _, serr := os.Lstat(p); serr == nil {
			return "", false // exists but does not resolve: dangling or looping
		}
		parent := filepath.Dir(p)
		if parent == p {
			return p, true
		}
		rest, p = filepath.Join(filepath.Base(p), rest), parent
	}
}

// recordSocketVolume retains definite backing evidence; other live volumes stay unknown.
func recordSocketVolume(rep *socketInventory, dir, name, device string) {
	resolved, ok := resolveSocketPath(dir, device)
	if device != "" && !strings.Contains(device, "$") && (socketPathExposed(device) || ok && socketPathExposed(resolved)) {
		rep.Volumes[name] = "is backed by " + device
	}
}
