package build

// socket_guard.go — P7-DEPL-21 (ADR 0027 §3): no proxy or plugin container
// gets the Docker socket. One merged view of the generated stack, plugin
// fragments and override file; only nginx/traefik and plugin-declared services
// are judged (core admin, CS_N are not). Podman sockets and Docker data
// directories are out of scope. Refuse() is E130 naming plugin and service; an
// excepted plugin warns once until removal_at; a proxy never inherits one.
// Ungated by compat.V15 (ADR 0021). Unresolved values, env_file, extends,
// volumes_from, include and bad shapes fail closed. SocketReport is shared
// with `nself doctor --deep`; scanSocketService is in orchestrator_compose.go.

import (
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/version"
	"gopkg.in/yaml.v3"
)

// SocketException allows one plugin to keep a socket mount until RemovalAt.
type SocketException struct{ Slug, Ticket, RemovalAt string } // RemovalAt: first CLI version (X.Y.Z) that refuses it

// socketExceptions is keyed by plugin slug; an unparsable version is expired.
var socketExceptions = map[string]SocketException{
	"forgejo":    {Slug: "forgejo", RemovalAt: "1.5.0", Ticket: "P7-TRUST-21"},
	"monitoring": {Slug: "monitoring", RemovalAt: "1.5.0", Ticket: "P7-PLUG-58"},
}

// ActiveSocketException returns slug's exception until it expires.
func ActiveSocketException(slug string) (SocketException, bool) {
	ex, ok := socketExceptions[slug]
	if !ok || versionAtLeast(version.GetVersion(), ex.RemovalAt) {
		return SocketException{}, false
	}
	return ex, true
}

// versionAtLeast reports cur >= threshold on X.Y.Z (any -pre/+build ignored);
// a version that does not parse counts as at-least (fail closed).
func versionAtLeast(cur, threshold string) bool {
	c, okC := parseXYZ(cur)
	t, okT := parseXYZ(threshold)
	return !okC || !okT || c >= t
}

func parseXYZ(v string) (packed int, ok bool) {
	var a, b, c int
	n, _ := fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(v), "v"), "%d.%d.%d", &a, &b, &c)
	return a*1e8 + b*1e4 + c, n == 3 && min(a, b, c) >= 0 && max(b, c) < 1e4
}

// SocketProxyService reports the services no exception can ever cover.
func SocketProxyService(name string) bool { return name == "nginx" || name == "traefik" }

// SocketPathExposed reports whether a host path is the Docker socket or a
// directory that holds it (including the macOS /private alias).
func SocketPathExposed(p string) bool {
	c := path.Clean(strings.ToLower(strings.TrimSpace(p)))
	if rest, ok := strings.CutPrefix(c, "/private"); ok && (rest == "" || rest[0] == '/') {
		c = path.Clean("/" + rest)
	}
	switch c {
	case "/", "/var", "/var/run", "/run":
		return true
	}
	return path.Base(c) == "docker.sock"
}

// SocketEnv resolves a ${VAR} name.
type SocketEnv func(string) (string, bool)

// NewSocketEnv resolves from the process env, then the project's .env files,
// then the computed plugin variables (NSELF_PLUGIN_DIR and friends). cfg may be nil.
func NewSocketEnv(workdir, pluginDir string, cfg *config.Config) SocketEnv {
	vars := ComputePluginEnvVars(workdir, pluginDir, cfg)
	for _, f := range []string{".env", composeEnvFile} {
		data, _ := os.ReadFile(filepath.Join(workdir, f))
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "export "), "=")
			if k = strings.TrimSpace(k); ok && k != "" && !strings.HasPrefix(k, "#") {
				vars[k] = strings.Trim(strings.TrimSpace(v), `"'`)
			}
		}
	}
	return func(k string) (string, bool) {
		if v, ok := os.LookupEnv(k); ok {
			return v, true
		}
		v, ok := vars[k]
		return v, ok
	}
}

// expandSocket expands ${VAR}, ${VAR:-d} and ${VAR-d}; an unset variable without
// a default, a bare $VAR and any other form are not ok.
func expandSocket(s string, env SocketEnv) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 < len(s) && s[i+1] == '$' {
			b.WriteByte('$')
			i++
			continue
		}
		j, name, rest := i+1, "", ""
		if j >= len(s) || s[j] != '{' { // bare $VAR is not followed
			return "", false
		}
		if j = strings.IndexByte(s[i:], '}'); j < 0 {
			return "", false
		}
		j += i
		name = s[i+2 : j]
		if k := strings.IndexAny(name, ":-?+="); k >= 0 {
			name, rest = name[:k], name[k:]
		}
		i = j
		v, set := env(name)
		def, hasDef := strings.CutPrefix(strings.TrimPrefix(rest, ":"), "-")
		d, dok := "", true
		if hasDef {
			d, dok = expandSocket(def, env)
		}
		switch {
		case name == "" || (rest != "" && !hasDef):
			return "", false
		case set && (v != "" || !strings.HasPrefix(rest, ":")):
			b.WriteString(v)
		case hasDef && dok:
			b.WriteString(d)
		default:
			return "", false
		}
	}
	return b.String(), true
}

// SocketUse is one way a service reaches the socket, and which input said so.
type SocketUse struct{ Origin, Detail string }

// SocketReport is the merged view of every compose input.
type SocketReport struct {
	Uses    map[string][]SocketUse // service -> exposures
	Owners  map[string][]string    // service -> plugin slugs that declare it
	Core    map[string]bool        // services declared by the core stack or the override
	Volumes map[string]string      // named volume -> "" or why it is socket-backed
}

type socketDoc struct {
	origin string
	top    map[string]any
}

// socketMap reads a mapping that may be absent or an empty `!reset []`.
func socketMap(v any, what string) (map[string]any, error) {
	switch m := v.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return m, nil
	case []any:
		if len(m) == 0 {
			return nil, nil
		}
	}
	return nil, fmt.Errorf("%s has an unsupported shape", what)
}

// AnalyzeSocket scans the generated stack (core), every enabled plugin
// fragment and docker-compose.override.yml. The YAML decoder resolves anchors,
// merge keys and tags.
func AnalyzeSocket(workdir, pluginDir string, env SocketEnv, core []byte) (*SocketReport, error) {
	var docs []socketDoc
	add := func(origin string, data []byte) error {
		var top map[string]any
		if err := yaml.Unmarshal(data, &top); err != nil {
			return fmt.Errorf("%s: %w", origin, err)
		}
		if top["include"] != nil {
			return fmt.Errorf("%s uses include, which is not inspected", origin)
		}
		docs = append(docs, socketDoc{origin, top})
		return nil
	}
	if err := add("core stack", core); err != nil {
		return nil, err
	}
	entries, _ := os.ReadDir(pluginDir)
	for _, e := range entries { // enabled plugins only, in name order
		if _, off := os.Stat(filepath.Join(pluginDir, e.Name(), ".disabled")); !e.IsDir() || off == nil {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(pluginDir, e.Name(), pluginComposeFilename)); err == nil {
			if err := add("plugin:"+e.Name(), data); err != nil {
				return nil, err
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(workdir, userComposeOverrideFile)); err == nil {
		if err := add("override", data); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	rep := &SocketReport{Uses: map[string][]SocketUse{}, Owners: map[string][]string{}, Core: map[string]bool{}, Volumes: map[string]string{}}
	for _, d := range docs { // named volumes first: any input may mount another's
		vols, err := socketMap(d.top["volumes"], d.origin+" volumes")
		if err != nil {
			return nil, err
		}
		for name, v := range vols {
			m, _ := v.(map[string]any)
			opts, _ := m["driver_opts"].(map[string]any)
			dev, _ := opts["device"].(string)
			x, ok := expandSocket(dev, env)
			rep.Volumes[name] += "" // defined; stays "" unless socket-backed
			if dev != "" && !ok {
				rep.Volumes[name] = fmt.Sprintf("is backed by %q, which cannot be resolved", dev)
			} else if dev != "" && SocketPathExposed(x) {
				rep.Volumes[name] = "is backed by " + x
			}
		}
	}
	for _, d := range docs {
		svcs, err := socketMap(d.top["services"], d.origin+" services")
		if err != nil {
			return nil, err
		}
		for name, raw := range svcs {
			body, ok := raw.(map[string]any)
			if !ok && raw != nil {
				return nil, fmt.Errorf("%s service %q has an unsupported shape", d.origin, name)
			}
			if slug, isPlugin := strings.CutPrefix(d.origin, "plugin:"); isPlugin {
				rep.Owners[name] = append(rep.Owners[name], slug)
			} else {
				rep.Core[name] = true
			}
			uses, err := scanSocketService(d, name, body, env, rep.Volumes)
			if err != nil {
				return nil, err
			}
			rep.Uses[name] = append(rep.Uses[name], uses...)
		}
	}
	return rep, nil
}

// Refuse returns E130 for the first judged service that holds the socket.
func (r *SocketReport) Refuse() error {
	for _, svc := range slices.Sorted(maps.Keys(r.Uses)) {
		slugs, uses := r.Owners[svc], r.Uses[svc]
		if len(uses) == 0 || (len(slugs) == 0 && !SocketProxyService(svc)) {
			continue // clean, or core/custom: not judged
		}
		own := true // every exposure comes from the declaring plugin itself
		for _, u := range uses {
			own = own && len(slugs) == 1 && u.Origin == "plugin:"+slugs[0]
		}
		if ex, ok := ActiveSocketException(strings.Join(slugs, "")); ok && own && !SocketProxyService(svc) {
			warnOnce(fmt.Sprintf("plugin %q holds the Docker socket; allowed until nself %s (%s), then the build refuses it (E130)", ex.Slug, ex.RemovalAt, ex.Ticket))
			continue
		}
		owner := uses[0].Origin
		if slug, ok := strings.CutPrefix(owner, "plugin:"); ok {
			owner = fmt.Sprintf("plugin %q", slug)
		} else if len(slugs) > 0 {
			owner = fmt.Sprintf("plugin %q via %s", strings.Join(slugs, `", "`), owner)
		}
		return errs.Newf("E130", "%s service %q %s", owner, svc, uses[0].Detail)
	}
	return nil
}

// guardDockerSocket scans the stack the generator would write, every plugin
// fragment and the override file, before provider output moves.
func (st *buildState) guardDockerSocket(g *compose.Generator) error {
	stack, err := g.Generate()
	if err != nil {
		return nil // the real render reports it
	}
	rep, err := AnalyzeSocket(st.workdir, st.pluginDir, NewSocketEnv(st.workdir, st.pluginDir, st.cfg), stack)
	if err != nil {
		return errs.Newf("E130", "compose inputs cannot be checked for the Docker socket: %v", err)
	}
	return rep.Refuse()
}
