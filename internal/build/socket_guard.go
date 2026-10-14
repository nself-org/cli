package build

// socket_guard.go — P7-DEPL-21 (ADR 0027 §3): no proxy or plugin container gets
// the Docker socket. One merged view of the generated stack, plugin fragments
// and override file; only nginx/traefik and plugin-declared services are judged
// (core admin, CS_N are not). Bind sources resolve against the project dir (~ is
// home), symlinks followed. Fail closed: unresolved values, external/renamed
// volumes, env_file, extends, volumes_from, include, bad shapes. Podman sockets
// and Docker data dirs are out of scope. Refuse() is E130; an excepted plugin
// warns once until removal_at; a proxy never inherits one. Shared with doctor.

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

// ActiveSocketException returns slug's exception below its removal_at; an unparsable version is past it.
func ActiveSocketException(slug string) (SocketException, bool) {
	ex, ok := socketExceptions[slug]
	var c, t [3]int
	nc, _ := fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(version.GetVersion()), "v"), "%d.%d.%d", &c[0], &c[1], &c[2])
	nt, _ := fmt.Sscanf(ex.RemovalAt, "%d.%d.%d", &t[0], &t[1], &t[2])
	return ex, ok && nc == 3 && nt == 3 && (c[0] < t[0] || c[0] == t[0] && (c[1] < t[1] || c[1] == t[1] && c[2] < t[2]))
}

// SocketProxyService reports the services no exception can ever cover.
func SocketProxyService(n string) bool { return n == "nginx" || n == "traefik" }

// SocketPathExposed reports whether a host path is the Docker socket or a
// directory that holds it (including the macOS /private alias).
func SocketPathExposed(p string) bool {
	c := strings.TrimPrefix(path.Clean(strings.ToLower(strings.TrimSpace(p))), "/private")
	switch c {
	case "", "/", "/var", "/var/run", "/run":
		return true
	}
	return path.Base(c) == "docker.sock"
}

// ResolveSocketPath makes a bind source absolute (~ is home, relative to base)
// and follows symlinks; ok is false when that fails (dangling or looping link).
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

var envRefRE = regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)`)

// NewSocketEnv layers like Compose: process env, then base (the compose.env this
// build will write), then the env files in order (later wins). godotenv expands
// each file alone, so a `$`-bearing value is trusted only if every variable it
// names (transitively) is defined earlier in that file, nowhere else, and not
// contradicted by the process env; any other is the unresolvable marker "$".
func NewSocketEnv(base map[string]string, files ...string) SocketEnv {
	vars, refs, count := map[string]string{}, map[string][]string{}, map[string]int{}
	for _, f := range files {
		m, _ := godotenv.Read(f)
		maps.Copy(vars, m)
		data, _ := os.ReadFile(f)
		defined := map[string]bool{}
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "export "), "=")
			if k = strings.TrimSpace(k); !ok || k == "" || k[0] == '#' {
				continue
			}
			count[k]++
			refs[k] = nil
			for _, r := range envRefRE.FindAllStringSubmatch(v, -1) {
				refs[k] = append(refs[k], r[1])
				if !defined[r[1]] {
					count[r[1]] += 2 // not defined earlier in this file: never exact
				}
			}
			defined[k] = true
		}
	}
	for k, v := range base {
		vars[k], count[k], refs[k] = v, count[k]+1, nil
	}
	var opaque func(k string, depth int) bool // transitive: a value built on an unprovable one is unprovable
	opaque = func(k string, depth int) bool {
		for _, r := range refs[k] {
			pv, set := os.LookupEnv(r)
			if depth > 8 || count[r] != 1 || set && pv != vars[r] || !set && opaque(r, depth+1) {
				return true
			}
		}
		return false
	}
	return func(k string) (v string, ok bool) {
		if v, ok = os.LookupEnv(k); ok {
			return v, true
		}
		if opaque(k, 0) {
			return "$", true
		}
		v, ok = vars[k]
		return v, ok
	}
}

// expandSocket expands $VAR, ${VAR} and ${VAR:-d} ($$ is a literal $); an unset
// variable without a default, an unbalanced brace and every other form are not ok.
func expandSocket(s string, env SocketEnv) (string, bool) {
	if strings.Count(s, "${") != strings.Count(s, "}") {
		return "", false
	}
	ok := true
	out := os.Expand(s, func(ref string) string {
		name, def, hasDef := strings.Cut(ref, ":-")
		switch v, set := env(name); {
		case ref == "$":
			return "\x00"
		case set && (v != "" || !hasDef):
			return v
		case hasDef:
			return def
		}
		ok = false
		return ""
	})
	return strings.ReplaceAll(out, "\x00", "$"), ok && !strings.Contains(out, "$")
}

// SocketUse is one way a service reaches the socket and which input said so;
// Unproven marks what the guard could not read through (no exception waives it).
type SocketUse struct {
	Origin, Detail string
	Unproven       bool
}

// SocketReport is the merged view of every compose input.
type SocketReport struct {
	Uses    map[string][]SocketUse // service -> exposures
	Owners  map[string][]string    // service -> plugin slugs that declare it
	Core    map[string]bool        // services declared by the generated core stack
	Volumes map[string]string      // named volume -> "" or why it is socket-backed
}

type socketDoc struct {
	origin, dir string
	top         map[string]any
}

// socketMap reads a mapping that may be absent or an empty `!reset []`.
func socketMap(v any, what string) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if l, isList := v.([]any); !ok && v != nil && !(isList && len(l) == 0) {
		return nil, fmt.Errorf("%s has an unsupported shape", what)
	}
	return m, nil
}

// AnalyzeSocket scans the generated stack, enabled plugin fragments and the override file.
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
	matches, _ := filepath.Glob(filepath.Join(pluginDir, "*", pluginComposeFilename))
	files := [][2]string{} // origin, file; enabled plugins in name order, then the override
	for _, m := range matches {
		if _, off := os.Stat(filepath.Join(filepath.Dir(m), ".disabled")); off != nil {
			files = append(files, [2]string{"plugin:" + filepath.Base(filepath.Dir(m)), m})
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
			r, rok := ResolveSocketPath(d.dir, x)
			rep.Volumes[name] += "" // defined; stays "" unless socket-backed
			if dev != "" && (!ok || !rok) {
				rep.Volumes[name] = fmt.Sprintf("has a backing that cannot be proven (device %q cannot be resolved)", dev)
			} else if dev != "" && (SocketPathExposed(x) || SocketPathExposed(r)) && !strings.HasPrefix(rep.Volumes[name], "has a backing") {
				rep.Volumes[name] = "is backed by " + x // never replaces an unprovable reason: no exception waives that
			}
			if m["external"] != nil && m["external"] != false || m["name"] != nil {
				rep.Volumes[name] = "has a backing that cannot be proven (external or renamed)"
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
		own := len(slugs) == 1 && !slices.ContainsFunc(uses, func(u SocketUse) bool { return u.Origin != "plugin:"+slugs[0] || u.Unproven })
		if ex, ok := ActiveSocketException(strings.Join(slugs, "")); ok && own && !SocketProxyService(svc) {
			warnOnce(fmt.Sprintf("plugin %q holds the Docker socket; allowed until nself %s (%s), then the build refuses it (E130)", ex.Slug, ex.RemovalAt, ex.Ticket))
			continue
		}
		owner := uses[0].Origin
		if len(slugs) > 0 {
			owner = fmt.Sprintf("plugin %q (%s)", strings.Join(slugs, `", "`), owner)
		}
		return errs.Newf("E130", "%s service %q %s", owner, svc, uses[0].Detail)
	}
	return nil
}
