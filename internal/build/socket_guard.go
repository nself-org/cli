package build

// socket_guard.go — P7-DEPL-21 (ADR 0027 §3): no generated stack mounts the
// Docker socket into a proxy or plugin container.
// Inputs:  the plugin directory (every enabled docker-compose.plugin.yml) and
//          the final generated compose YAML of the core stack.
// Outputs: an E130 error naming the plugin (or the core stack) and the service,
//          or nil. An excepted plugin is allowed with one warning until its
//          removal_at version; the exception table is exported so
//          `nself doctor --deep` (proxy-docker-socket) applies the same rule.
// Constraints: ungated by compat.V15 (ADR 0021): no shipped plugin outside the
//          exception table mounts the socket. The core admin service and
//          CS_N custom services (E502 owns those) are not checked. A
//          fragment that cannot be parsed is refused, never passed.

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/version"
	"gopkg.in/yaml.v3"
)

// SocketException allows one plugin to keep a socket mount until RemovalAt.
type SocketException struct {
	Slug      string // plugin directory name
	RemovalAt string // first CLI version (X.Y.Z) at which the exception stops applying
	Ticket    string // ticket that removes the mount
}

// socketExceptions is keyed by plugin slug; an unparsable RemovalAt is expired.
var socketExceptions = map[string]SocketException{
	"forgejo":    {Slug: "forgejo", RemovalAt: "1.5.0", Ticket: "P7-TRUST-21"},
	"monitoring": {Slug: "monitoring", RemovalAt: "1.5.0", Ticket: "P7-PLUG-58"},
}

// ActiveSocketException returns the exception for slug while it has not
// expired at the running CLI version.
func ActiveSocketException(slug string) (SocketException, bool) {
	ex, ok := socketExceptions[slug]
	if !ok || versionAtLeast(version.GetVersion(), ex.RemovalAt) {
		return SocketException{}, false
	}
	return ex, true
}

// versionAtLeast reports cur >= threshold on X.Y.Z, ignoring any -pre/+build
// suffix. A version that does not parse counts as at-least (fail closed).
func versionAtLeast(cur, threshold string) bool {
	c, okC := parseXYZ(cur)
	t, okT := parseXYZ(threshold)
	if !okC || !okT {
		return true
	}
	for i := range c {
		if c[i] != t[i] {
			return c[i] > t[i]
		}
	}
	return true
}

func parseXYZ(v string) (out [3]int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// SocketFinding is one socket exposure in one compose service.
type SocketFinding struct {
	Service string
	Detail  string
}

type socketDoc struct {
	Services map[string]struct {
		Volumes     []yaml.Node `yaml:"volumes"`
		Environment yaml.Node   `yaml:"environment"`
	} `yaml:"services"`
	Volumes map[string]struct {
		DriverOpts map[string]any `yaml:"driver_opts"`
	} `yaml:"volumes"`
}

// SocketFindings parses one compose document and lists every service that
// mounts the Docker socket (or its parent directory) or points DOCKER_HOST at
// a unix socket. Services in skip are not inspected. Output is sorted.
func SocketFindings(doc []byte, skip map[string]bool) ([]SocketFinding, error) {
	var d socketDoc
	if err := yaml.Unmarshal(doc, &d); err != nil {
		return nil, err
	}
	sockVol := map[string]bool{} // named volumes backed by the socket
	for name, v := range d.Volumes {
		if strings.Contains(strings.ToLower(fmt.Sprint(v.DriverOpts["device"])), "docker.sock") {
			sockVol[name] = true
		}
	}
	var out []SocketFinding
	for svc, s := range d.Services {
		if skip[svc] {
			continue
		}
		for i := range s.Volumes {
			if raw, bad := socketVolume(&s.Volumes[i], sockVol); bad {
				out = append(out, SocketFinding{svc, fmt.Sprintf("mounts the Docker socket (volume %q)", raw)})
			}
		}
		for _, kv := range envPairs(&s.Environment) {
			if kv[0] == "DOCKER_HOST" && strings.Contains(strings.ToLower(kv[1]), "unix:") {
				out = append(out, SocketFinding{svc, fmt.Sprintf("sets DOCKER_HOST to %q", kv[1])})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Service != out[j].Service {
			return out[i].Service < out[j].Service
		}
		return out[i].Detail < out[j].Detail
	})
	return out, nil
}

func deref(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

// socketVolume reports whether one volumes: entry (short or long syntax)
// exposes the Docker socket, and returns the entry as written.
func socketVolume(n *yaml.Node, sockVol map[string]bool) (string, bool) {
	n = deref(n)
	var raw, src string
	switch n.Kind {
	case yaml.ScalarNode:
		raw = n.Value
		if !strings.HasPrefix(raw, "$") {
			src = strings.SplitN(raw, ":", 2)[0]
		}
	case yaml.MappingNode:
		var tgt string
		for i := 0; i+1 < len(n.Content); i += 2 {
			switch n.Content[i].Value {
			case "source":
				src = deref(n.Content[i+1]).Value
			case "target":
				tgt = deref(n.Content[i+1]).Value
			}
		}
		raw = src + ":" + tgt
	default:
		return "", false
	}
	if strings.Contains(strings.ToLower(raw), "docker.sock") || sockVol[src] {
		return raw, true
	}
	switch path.Clean(src) {
	case "/", "/var", "/var/run", "/run":
		return raw, true
	}
	return raw, false
}

// envPairs reads an environment: block in map or list form.
func envPairs(n *yaml.Node) [][2]string {
	n = deref(n)
	var out [][2]string
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			out = append(out, [2]string{n.Content[i].Value, deref(n.Content[i+1]).Value})
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			k, v, _ := strings.Cut(deref(c).Value, "=")
			out = append(out, [2]string{k, v})
		}
	}
	return out
}

func socketRefusal(owner, service, detail string) error {
	return errs.Newf("E130", "%s service %q %s", owner, service, detail).
		WithWhy("ADR 0027: the Docker socket is root-equivalent on the host; no generated stack hands it to a proxy or plugin container.").
		WithFix("Remove the socket volume and the unix DOCKER_HOST from the service, or remove the plugin.")
}

// PluginFragmentServices maps each service in an enabled plugin fragment to its plugin slug.
func PluginFragmentServices(pluginDir string) map[string]string {
	out := map[string]string{}
	for _, slug := range fragmentSlugs(pluginDir) {
		data, err := os.ReadFile(filepath.Join(pluginDir, slug, pluginComposeFilename))
		if err != nil {
			continue
		}
		var d socketDoc
		if yaml.Unmarshal(data, &d) == nil {
			for svc := range d.Services {
				out[svc] = slug
			}
		}
	}
	return out
}

// fragmentSlugs lists the enabled plugin directories, sorted.
func fragmentSlugs(pluginDir string) []string {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil
	}
	var slugs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(pluginDir, e.Name(), ".disabled")); err == nil {
			continue
		}
		slugs = append(slugs, e.Name())
	}
	sort.Strings(slugs)
	return slugs
}

// guardPluginFragments refuses a non-excepted plugin fragment that exposes
// the socket; an excepted one warns once per process.
func guardPluginFragments(pluginDir string) error {
	for _, slug := range fragmentSlugs(pluginDir) {
		data, err := os.ReadFile(filepath.Join(pluginDir, slug, pluginComposeFilename))
		if err != nil {
			continue // no fragment: nothing to check (E128 owns a missing one)
		}
		found, err := SocketFindings(data, nil)
		if err != nil {
			return errs.Newf("E130", "plugin %q fragment cannot be checked for the Docker socket: %v", slug, err).
				WithFix("Fix the YAML in the plugin's docker-compose.plugin.yml, or remove the plugin.")
		}
		for _, f := range found {
			if ex, ok := ActiveSocketException(slug); ok {
				warnOnce(fmt.Sprintf("plugin %q service %q %s; allowed until nself %s (%s), then the build refuses it (E130)", slug, f.Service, f.Detail, ex.RemovalAt, ex.Ticket))
				continue
			}
			return socketRefusal(fmt.Sprintf("plugin %q", slug), f.Service, f.Detail)
		}
	}
	return nil
}

// guardGeneratedStack refuses a socket exposure in the core stack's YAML.
func guardGeneratedStack(stack []byte, skip map[string]bool) error {
	found, err := SocketFindings(stack, skip)
	if err != nil {
		return errs.Newf("E130", "generated stack cannot be checked for the Docker socket: %v", err)
	}
	if len(found) > 0 {
		return socketRefusal("core stack", found[0].Service, found[0].Detail)
	}
	return nil
}

// guardDockerSocket checks every plugin fragment, then the stack the generator
// would write (rendered here so the guard runs before provider output is
// cleaned up; a generator error is left for the real render to report).
func (st *buildState) guardDockerSocket(g *compose.Generator) error {
	if err := guardPluginFragments(st.pluginDir); err != nil {
		return err
	}
	stack, err := g.Generate()
	if err != nil {
		return nil
	}
	skip := map[string]bool{compose.AdminServiceName: true}
	for _, cs := range st.cfg.CustomServices {
		skip[cs.Name] = true
	}
	return guardGeneratedStack(stack, skip)
}
