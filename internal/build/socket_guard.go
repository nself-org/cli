package build

// ADR 0027: inspect merged Compose inputs; refuse sockets on proxy/plugin services.

import (
	"fmt"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"gopkg.in/yaml.v3"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

type SocketException struct{ Slug, Ticket, RemovalAt string } // RemovalAt: first CLI version (X.Y.Z) that refuses it
var socketExceptions = map[string]SocketException{
	"forgejo":    {Slug: "forgejo", RemovalAt: "1.5.0", Ticket: "P7-TRUST-21"},
	"monitoring": {Slug: "monitoring", RemovalAt: "1.5.0", Ticket: "P7-PLUG-58"},
}

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

type SocketEnv func(string) (string, bool)

var envRefRE = regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)`)

// NewSocketEnv uses process/base/file precedence and distrusts ambiguous dotenv dependencies.
func NewSocketEnv(base map[string]string, files ...string) SocketEnv {
	vars, refs, count := map[string]string{}, map[string][]string{}, map[string]int{}
	for _, f := range files {
		m, _ := config.ReadEnvFile(f)
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

type SocketUse struct {
	Origin, Detail string
	Unproven       bool
}
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
type socketParseError struct {
	origin string
	err    error
}

func (e *socketParseError) Error() string { return fmt.Sprintf("%s: %v", e.origin, e.err) }
func socketMap(v any, what string) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if l, isList := v.([]any); !ok && v != nil && (!isList || len(l) != 0) {
		return nil, fmt.Errorf("%s has an unsupported shape", what)
	}
	return m, nil
}

// AnalyzeSocket scans all generated, plugin and override inputs; exceptions never waive uncertainty.
func AnalyzeSocket(workdir, pluginDir string, env SocketEnv, core []byte) (*SocketReport, error) {
	var docs []socketDoc
	add := func(origin string, data []byte) error {
		var top map[string]any
		if err := yaml.Unmarshal(data, &top); err != nil {
			return &socketParseError{origin, err}
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
			if m["external"] != nil && m["external"] != false {
				rep.Volumes[name] = "has a backing that cannot be proven (external)"
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

// socketFileEnv reads container values, distinct from Compose interpolation variables.
// Missing files cannot start a container; malformed present files fail without quoting contents.
func socketFileEnv(d socketDoc, value any, lookup SocketEnv) (map[string]any, error) {
	out := map[string]any{}
	files, list := value.([]any)
	if value == nil {
		return out, nil
	}
	if !list {
		files = []any{value}
	}
	for _, entry := range files {
		name, raw := "", false
		switch e := entry.(type) {
		case string:
			name = e
		case map[string]any:
			name, _ = e["path"].(string)
			raw = e["format"] == "raw"
			if e["format"] != nil && e["format"] != "raw" {
				return nil, fmt.Errorf("%s env_file has unsupported format", d.origin)
			}
		default:
			return nil, fmt.Errorf("%s env_file has unsupported shape", d.origin)
		}
		p, ok := expandSocket(name, lookup)
		if !ok || p == "" {
			return nil, fmt.Errorf("%s env_file path cannot be resolved", d.origin)
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(d.dir, p)
		}
		data, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			continue
		} // cannot run or expose a socket; optional files also disappear
		if err != nil {
			return nil, fmt.Errorf("%s env_file: %w", d.origin, err)
		}
		vars := map[string]string{}
		if !raw {
			vars, err = config.ParseEnvBytes(p, data)
			if err != nil {
				return nil, fmt.Errorf("%s env_file: %w", d.origin, err)
			}
		}
		for _, line := range strings.Split(string(data), "\n") {
			k, v, has := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "export "), "=")
			if strings.TrimSpace(k) != "DOCKER_HOST" || !has {
				continue
			}
			if raw {
				vars["DOCKER_HOST"] = v
			} else if strings.Contains(v, "$") {
				x, resolved := expandSocket(strings.Trim(strings.TrimSpace(v), "\"'"), lookup)
				if !resolved {
					x = "$"
				}
				vars["DOCKER_HOST"] = x
			}
		}
		if v, present := vars["DOCKER_HOST"]; present {
			out["DOCKER_HOST"] = v
		}
	}
	return out, nil
}
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
