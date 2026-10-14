package build

// socket_guard.go — P7-DEPL-21 (ADR 0027 §3): no proxy or plugin container
// gets the Docker socket. One merged view of the generated stack, plugin
// fragments and override file; only nginx/traefik and plugin-declared services
// are judged (core admin, CS_N are not). Bind sources resolve against the
// project dir (~ is home) with symlinks followed. Fail closed: unresolved
// values, external/renamed volumes, env_file, extends, volumes_from, include,
// bad shapes. Podman sockets and Docker data dirs are out of scope. Refuse() is
// E130; an excepted plugin warns once until removal_at; a proxy never inherits
// one. Shared with doctor; scanSocketService is in orchestrator_compose.go.

import (
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/joho/godotenv"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/version"
	"gopkg.in/yaml.v3"
)

// SocketException allows one plugin to keep a socket mount until RemovalAt.
type SocketException struct{ Slug, Ticket, RemovalAt string } // RemovalAt: first CLI version (X.Y.Z) that refuses it

var socketExceptions = map[string]SocketException{
	"forgejo":    {Slug: "forgejo", RemovalAt: "1.5.0", Ticket: "P7-TRUST-21"},
	"monitoring": {Slug: "monitoring", RemovalAt: "1.5.0", Ticket: "P7-PLUG-58"},
}

// ActiveSocketException returns slug's exception until it expires.
func ActiveSocketException(slug string) (SocketException, bool) {
	ex, ok := socketExceptions[slug]
	return ex, ok && !versionAtLeast(version.GetVersion(), ex.RemovalAt)
}

// versionAtLeast reports cur >= threshold on X.Y.Z (any -pre/+build ignored);
// a version that does not parse counts as at-least (fail closed).
func versionAtLeast(cur, threshold string) bool {
	var c, t [3]int
	nc, _ := fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(cur), "v"), "%d.%d.%d", &c[0], &c[1], &c[2])
	nt, _ := fmt.Sscanf(threshold, "%d.%d.%d", &t[0], &t[1], &t[2])
	return nc != 3 || nt != 3 || c[0] > t[0] || c[0] == t[0] && (c[1] > t[1] || c[1] == t[1] && c[2] >= t[2])
}

// SocketProxyService reports the services no exception can ever cover.
func SocketProxyService(n string) bool { return n == "nginx" || n == "traefik" }

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

// ResolveSocketPath makes a bind source absolute (~ is home, a relative path
// is relative to base) and follows symlinks through its longest existing
// prefix; ok is false when that fails, including a dangling or looping symlink.
func ResolveSocketPath(base, p string) (string, bool) {
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

// SocketEnv resolves a ${VAR} name.
type SocketEnv func(string) (string, bool)

// NewSocketEnv layers like Compose: the process env, then base (computed
// values), then the env files in order (later wins). godotenv expands nested,
// quoted and escaped values; an unreadable file adds nothing.
func NewSocketEnv(base map[string]string, files ...string) SocketEnv {
	vars := map[string]string{}
	for _, f := range files {
		m, _ := godotenv.Read(f)
		maps.Copy(vars, m)
	}
	maps.Copy(vars, base)
	return func(k string) (string, bool) {
		if v, ok := os.LookupEnv(k); ok {
			return v, true
		}
		v, ok := vars[k]
		return v, ok
	}
}

var socketVarRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}$]*))?\}`)

// expandSocket expands ${VAR} and ${VAR:-d} ($$ is a literal $); an unset
// variable without a default, a bare $VAR and every other form are not ok.
func expandSocket(s string, env SocketEnv) (string, bool) {
	ok := true
	out := socketVarRE.ReplaceAllStringFunc(strings.ReplaceAll(s, "$$", "\x00"), func(m string) string {
		g := socketVarRE.FindStringSubmatch(m)
		v, set := env(g[1])
		hasDef := strings.Contains(m, ":-")
		switch {
		case set && (v != "" || !hasDef):
			return v
		case hasDef:
			return g[2]
		}
		ok = false
		return ""
	})
	return strings.ReplaceAll(out, "\x00", "$"), ok && !strings.Contains(out, "$")
}

// SocketUse is one way a service reaches the socket, and which input said so.
type SocketUse struct{ Origin, Detail string }

// SocketReport is the merged view of every compose input.
type SocketReport struct {
	Uses    map[string][]SocketUse // service -> exposures
	Owners  map[string][]string    // service -> plugin slugs that declare it
	Core    map[string]bool        // services declared by the generated core stack
	Volumes map[string]string      // named volume -> "" or why it is socket-backed
	Mounted map[string]bool        // services whose inputs declare at least one volume
}

type socketDoc struct {
	origin, dir string
	top         map[string]any
}

// socketMap reads a mapping that may be absent or an empty `!reset []`.
func socketMap(v any, what string) (map[string]any, error) {
	if l, isList := v.([]any); isList && len(l) == 0 {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok && v != nil {
		return nil, fmt.Errorf("%s has an unsupported shape", what)
	}
	return m, nil
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
		docs = append(docs, socketDoc{origin, workdir, top})
		return nil
	}
	if err := add("core stack", core); err != nil {
		return nil, err
	}
	entries, _ := os.ReadDir(pluginDir)
	files := [][2]string{} // origin, file; enabled plugins in name order, then the override
	for _, e := range entries {
		if _, off := os.Stat(filepath.Join(pluginDir, e.Name(), ".disabled")); e.IsDir() && off != nil {
			files = append(files, [2]string{"plugin:" + e.Name(), filepath.Join(pluginDir, e.Name(), pluginComposeFilename)})
		}
	}
	files = append(files, [2]string{"override", filepath.Join(workdir, userComposeOverrideFile)})
	for _, f := range files {
		data, err := os.ReadFile(f[1])
		if os.IsNotExist(err) {
			continue
		}
		if err == nil {
			err = add(f[0], data)
		}
		if err != nil {
			return nil, err
		}
	}
	rep := &SocketReport{Uses: map[string][]SocketUse{}, Owners: map[string][]string{}, Core: map[string]bool{}, Volumes: map[string]string{}, Mounted: map[string]bool{}}
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
			r, rok := ResolveSocketPath(d.dir, x)
			rep.Volumes[name] += "" // defined; stays "" unless socket-backed
			if m["external"] != nil && m["external"] != false || m["name"] != nil {
				rep.Volumes[name] = "is external or renamed, so its backing cannot be proven"
			}
			if dev != "" && (!ok || !rok) {
				rep.Volumes[name] = fmt.Sprintf("is backed by %q, which cannot be resolved", dev)
			} else if dev != "" && (SocketPathExposed(x) || SocketPathExposed(r)) {
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
			} else if d.origin == "core stack" {
				rep.Core[name] = true // not the override: a ghost plugin service is no core service
			}
			if v, _ := body["volumes"].([]any); len(v) > 0 {
				rep.Mounted[name] = true
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
		own := len(slugs) == 1 // every exposure comes from the declaring plugin itself
		for _, u := range uses {
			own = own && u.Origin == "plugin:"+slugs[0]
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

// guardDockerSocket scans the would-be stack, plugin fragments and override file.
func (st *buildState) guardDockerSocket(g *compose.Generator) error {
	stack, err := g.Generate()
	if err != nil {
		return nil // the real render reports it
	}
	rep, err := AnalyzeSocket(st.workdir, st.pluginDir, NewSocketEnv(ComputePluginEnvVars(st.workdir, st.pluginDir, st.cfg), filepath.Join(st.workdir, ".env")), stack)
	if err != nil {
		return errs.Newf("E130", "compose inputs cannot be checked for the Docker socket: %v", err)
	}
	return rep.Refuse()
}
