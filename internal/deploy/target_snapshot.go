package deploy

// Purpose: the read-only picture of a remote target a v1.5 deploy builds
//          against (D24 step 1): which plugins (name, version, disabled) are
//          installed in the target's plugin dir, and which certificate dirs
//          under <remote_path>/ssl/certificates hold which PEM files.
// Inputs:  a Remote and the remote project path.
// Outputs: TargetSnapshot, and the differences against the local plugin set
//          or against another snapshot (drift, E485).
// Constraints: plain file reads only (grep, find, cat, printf): nothing is
//              sourced, `nself` is never run on the target (it would fetch
//              the registry and write a cache), no network, no writes. The
//              ssl read lists file names and never opens a PEM.

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// PluginRef is one installed plugin.
type PluginRef struct {
	Name     string
	Version  string
	Disabled bool
}

// CertDir is one certificate directory and which PEM files it holds.
type CertDir struct {
	Name                      string
	Fullchain, Privkey, Chain bool
}

// TargetSnapshot is what a target looks like to the build.
type TargetSnapshot struct {
	PluginDir string
	Plugins   []PluginRef
	SSL       []CertDir
	// FrontSSL is the fronting stack's <front>/ssl/certificates layout for a
	// fronted project (TakeFrontSSL); nil for a project that fronts itself.
	FrontSSL []CertDir
}

// TakeTargetSnapshot reads the snapshot of the host behind r.
func TakeTargetSnapshot(ctx context.Context, r Remote, remotePath string) (TargetSnapshot, error) {
	var s TargetSnapshot
	home, err := r.Run(ctx, `printf '%s\n' "$HOME"`)
	if err != nil {
		return s, fmt.Errorf("target snapshot: reading home: %w", err)
	}
	envOut, err := r.Run(ctx, shScript(fmt.Sprintf(`grep -E '^[[:space:]]*(NSELF_PLUGIN_DIR|PLUGIN_DIR)=' %s 2>/dev/null || true`, q(path.Join(remotePath, ".env")))))
	if err != nil {
		return s, fmt.Errorf("target snapshot: reading .env: %w", err)
	}
	s.PluginDir = pluginDirFrom(string(envOut), strings.TrimSpace(string(home)), remotePath)
	dir := q(s.PluginDir)
	out, err := r.Run(ctx, shScript(fmt.Sprintf(
		`[ -d %s ] || exit 0; find %s -maxdepth 2 -name .disabled -print | sed 's/^/@D /'; find %s -maxdepth 2 -name plugin.json -exec sh -c 'for f; do printf "@F %%s\n" "$f"; cat "$f"; printf "\n"; done' sh {} +`,
		dir, dir, dir)))
	if err != nil {
		return s, fmt.Errorf("target snapshot: reading plugins in %s: %w", s.PluginDir, err)
	}
	if s.Plugins, err = parsePluginBlocks(string(out)); err != nil {
		return s, err
	}
	pems, err := r.Run(ctx, shScript(fmt.Sprintf(`find -L %s -maxdepth 2 -name '*.pem' -print 2>/dev/null || true`, q(path.Join(remotePath, "ssl", "certificates")))))
	if err != nil {
		return s, fmt.Errorf("target snapshot: listing certificates: %w", err)
	}
	if s.SSL, err = parsePEMNames(string(pems)); err != nil {
		return s, err
	}
	return s, nil
}

// pluginDirFrom picks the plugin dir: NSELF_PLUGIN_DIR over PLUGIN_DIR, the
// last assignment of each wins, then ~/.nself/plugins. ~ and $HOME prefixes are
// expanded with the host's home; a relative value is relative to remotePath.
func pluginDirFrom(envLines, home, remotePath string) string {
	vals := map[string]string{}
	for _, line := range strings.Split(envLines, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		vals[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	v := vals["NSELF_PLUGIN_DIR"]
	if v == "" {
		v = vals["PLUGIN_DIR"]
	}
	if v == "" {
		return path.Join(home, ".nself", "plugins")
	}
	for _, p := range []string{"${HOME}", "$HOME", "~"} {
		if v == p || strings.HasPrefix(v, p+"/") {
			return path.Join(home, strings.TrimPrefix(v, p))
		}
	}
	if !strings.HasPrefix(v, "/") {
		return path.Join(remotePath, v)
	}
	return path.Clean(v)
}

// parsePluginBlocks reads the "@D <path>" (.disabled markers) and "@F <path>"
// (plugin.json follows) stream the snapshot command prints.
func parsePluginBlocks(out string) ([]PluginRef, error) {
	disabled := map[string]bool{}
	type block struct {
		dir  string
		body []string
	}
	var blocks []*block
	var cur *block
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "@D "):
			disabled[path.Base(path.Dir(strings.TrimPrefix(line, "@D ")))] = true
			cur = nil
		case strings.HasPrefix(line, "@F "):
			cur = &block{dir: path.Base(path.Dir(strings.TrimPrefix(line, "@F ")))}
			blocks = append(blocks, cur)
		case cur != nil:
			cur.body = append(cur.body, line)
		}
	}
	var refs []PluginRef
	for _, b := range blocks {
		var m struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if json.Unmarshal([]byte(strings.Join(b.body, "\n")), &m) != nil || m.Name == "" {
			continue // an unreadable manifest is not an installed plugin, as the local loader treats it
		}
		if !hostNameOK.MatchString(m.Name) || (m.Version != "" && !hostVersionOK.MatchString(m.Version)) {
			return nil, hostEntryError("plugin", m.Name+" "+m.Version)
		}
		refs = append(refs, PluginRef{Name: m.Name, Version: m.Version, Disabled: disabled[b.dir]})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
	return refs, nil
}

// Names read from the host are untrusted: they reach error text and, for cert
// dirs, a local file path in the scratch build.
var (
	hostNameOK    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	hostVersionOK = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
)

func hostEntryError(kind, entry string) error {
	return errs.New("E485", fmt.Sprintf("target snapshot: the host reports a %s with an unusable name or version %q; allowed are letters, digits, '.', '_' and '-' (max 128); nothing was sent", kind, entry))
}

// parsePEMNames groups <dir>/<file>.pem paths by certificate dir. Hidden dirs
// (the .<dir>.gen-N generations behind a symlink) are not certificate dirs.
func parsePEMNames(out string) ([]CertDir, error) {
	byDir := map[string]*CertDir{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		dir, file := path.Base(path.Dir(line)), path.Base(line)
		if dir == "" || dir == "." || strings.HasPrefix(dir, ".") || dir == "certificates" {
			continue
		}
		if !hostNameOK.MatchString(dir) {
			return nil, hostEntryError("certificate dir", dir)
		}
		c := byDir[dir]
		if c == nil {
			c = &CertDir{Name: dir}
			byDir[dir] = c
		}
		switch file {
		case "fullchain.pem":
			c.Fullchain = true
		case "privkey.pem":
			c.Privkey = true
		case "chain.pem":
			c.Chain = true
		}
	}
	var certs []CertDir
	for _, c := range byDir {
		certs = append(certs, *c)
	}
	sort.Slice(certs, func(i, j int) bool { return certs[i].Name < certs[j].Name })
	return certs, nil
}

// activePlugins maps name to version for the enabled plugins.
func activePlugins(ps []PluginRef) map[string]string {
	m := map[string]string{}
	for _, p := range ps {
		if !p.Disabled {
			m[p.Name] = p.Version
		}
	}
	return m
}

// PluginDrift lists the differences between the local enabled plugin set and
// the target's: a plugin on one side only, or a version difference.
func PluginDrift(local []PluginRef, target TargetSnapshot) []string {
	l, t := activePlugins(local), activePlugins(target.Plugins)
	var diffs []string
	for _, n := range sortedKeys(l) {
		switch tv, ok := t[n]; {
		case !ok:
			diffs = append(diffs, fmt.Sprintf("plugin %q %q is installed locally but not on the target", n, l[n]))
		case tv != l[n]:
			diffs = append(diffs, fmt.Sprintf("plugin %q is %q locally but %q on the target", n, l[n], tv))
		}
	}
	for _, n := range sortedKeys(t) {
		if _, ok := l[n]; !ok {
			diffs = append(diffs, fmt.Sprintf("plugin %q %q is on the target but not installed locally", n, t[n]))
		}
	}
	return diffs
}

// LayoutDrift lists the differences between the snapshot the build used (want)
// and another read of a target (got): plugins, versions and ssl dirs.
func LayoutDrift(want, got TargetSnapshot) []string {
	diffs := PluginDrift(want.Plugins, got)
	w, g := map[string]CertDir{}, map[string]CertDir{}
	for _, c := range want.SSL {
		w[c.Name] = c
	}
	for _, c := range got.SSL {
		g[c.Name] = c
	}
	for _, n := range sortedCertKeys(w) {
		switch gc, ok := g[n]; {
		case !ok:
			diffs = append(diffs, fmt.Sprintf("ssl dir %q is missing on this server", n))
		case gc != w[n]:
			diffs = append(diffs, fmt.Sprintf("ssl dir %q has different files on this server", n))
		}
	}
	for _, n := range sortedCertKeys(g) {
		if _, ok := w[n]; !ok {
			diffs = append(diffs, fmt.Sprintf("ssl dir %q exists on this server but not on the one the build used", n))
		}
	}
	return diffs
}

// DriftError is E485 with the differences named, or nil when there are none.
func DriftError(diffs []string) error {
	if len(diffs) == 0 {
		return nil
	}
	return errs.New("E485", "target drift: "+strings.Join(diffs, "; ")).
		WithFix("install the same plugin versions on the target (or locally), then deploy again; nothing was sent")
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func sortedCertKeys(m map[string]CertDir) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
