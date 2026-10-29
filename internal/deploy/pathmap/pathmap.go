package pathmap

// Purpose: rewrite the absolute local paths a build leaves in compose files,
//          manifests and env files to the remote layout (D24 step 3), and
//          prove none survives. Local roots: the scratch project, the
//          operator's real project, the local plugin dir, the operator's home.
// Inputs:  the roots and the remote project path.
// Outputs: rewritten text, and the list of local roots a text still names.
// Constraints: a root matches only on path boundaries (/Users/a/p does not
//              match /Users/a/p2). The home dir cannot be mapped, so it is
//              only ever reported as a leftover. Roots under the remote path
//              are masked before the leftover scan, so a remote path that
//              starts with the local home is not a false hit.

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// PathMap maps the local roots to the remote layout.
type PathMap struct {
	ScratchRoot  string // the scratch project the build ran in
	OperatorRoot string // the operator's real project root
	PluginDir    string // the local plugin dir the build read
	Home         string // the operator's home dir (reported, never mapped)
	RemotePath   string // the project dir on the target
	// FrontRoot is, for a project fronted by another stack's nginx (D-0425), the
	// local scratch root of that stack: the parent of ScratchRoot. It maps to
	// path.Dir(RemotePath), after ScratchRoot -> RemotePath (longest first).
	FrontRoot string
}

// RemotePlugins is where plugin fragments live on the target.
func (m PathMap) RemotePlugins() string { return path.Join(m.RemotePath, ".nself", "plugins") }

type pathPair struct{ from, to string }

// pairs returns the from->to rewrites, longest source first, each root also
// under its symlink-resolved spelling.
func (m PathMap) pairs() []pathPair {
	var ps []pathPair
	add := func(from, to string) {
		for _, f := range rootSpellings(from) {
			ps = append(ps, pathPair{f, to})
		}
	}
	add(m.PluginDir, m.RemotePlugins())
	add(m.ScratchRoot, m.RemotePath)
	add(m.OperatorRoot, m.RemotePath)
	if m.FrontRoot != "" {
		add(m.FrontRoot, path.Dir(m.RemotePath))
	}
	sort.SliceStable(ps, func(i, j int) bool { return len(ps[i].from) > len(ps[j].from) })
	return ps
}

// rootSpellings returns root cleaned, plus its symlink-resolved form when that
// differs (macOS /var -> /private/var). Empty or "/" yields nothing.
func rootSpellings(root string) []string {
	root = strings.TrimSuffix(filepath.ToSlash(root), "/")
	if root == "" {
		return nil
	}
	out := []string{root}
	if real, err := filepath.EvalSymlinks(root); err == nil && real != root {
		out = append(out, filepath.ToSlash(real))
	}
	return out
}

// Rewrite maps every local root in s to the remote layout.
func (m PathMap) Rewrite(s string) string {
	for _, p := range m.pairs() {
		s = replacePath(s, p.from, p.to)
	}
	return s
}

// Leftovers names the local roots s still contains after a rewrite: scratch,
// operator, plugin dir and home.
func (m PathMap) Leftovers(s string) []string {
	for _, masked := range []string{m.RemotePlugins(), m.RemotePath} {
		s = replacePath(s, masked, "\x00")
	}
	var left []string
	for _, r := range []struct{ name, root string }{
		{"the scratch project", m.ScratchRoot}, {"the operator project", m.OperatorRoot},
		{"the fronting scratch root", m.FrontRoot}, {"the local plugin dir", m.PluginDir}, {"the home dir", m.Home},
	} {
		for _, sp := range rootSpellings(r.root) {
			if hasPath(s, sp) {
				left = append(left, r.name+" ("+sp+")")
				break
			}
		}
	}
	return left
}

// Remote returns the target path of a local file: under the scratch project it
// keeps its relative path, under the plugin dir it moves to .nself/plugins.
func (m PathMap) Remote(local string) (string, error) {
	if !filepath.IsAbs(local) {
		local = filepath.Join(m.ScratchRoot, local)
	}
	local = filepath.ToSlash(local)
	for _, src := range []struct{ root, to string }{{m.PluginDir, m.RemotePlugins()}, {m.ScratchRoot, m.RemotePath}} {
		for _, r := range rootSpellings(src.root) {
			if rel, ok := strings.CutPrefix(local, r+"/"); ok {
				return path.Join(src.to, rel), nil
			}
		}
	}
	return "", fmt.Errorf("%s is outside the scratch project and the plugin dir; it cannot be shipped", local)
}

// rootRE matches root as a path in any separator spelling a build can leave in a
// file: "/" in root matches "/" or "\" (runs of them), and a Windows drive path
// (C:\x) matches case-insensitively. Windows operators get C:\Users\a\p/nginx
// from a compose file the build wrote with filepath.Join.
func rootRE(root string) *regexp.Regexp {
	parts := strings.Split(root, "/")
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	pat := strings.Join(parts, `[\\/]+`)
	if len(root) >= 2 && root[1] == ':' {
		pat = "(?i)" + pat
	}
	return regexp.MustCompile(pat)
}

// replacePath replaces from with to wherever from stands as a whole path: not
// in the middle of a longer name before it, and followed by a separator or a
// non-name character. Backslashes in the rest of the path after a match become "/".
func replacePath(s, from, to string) string {
	if from == "" {
		return s
	}
	re := rootRE(from)
	var b strings.Builder
	last := 0
	for _, m := range re.FindAllStringIndex(s, -1) {
		if m[0] < last || !boundaryBefore(s, m[0]) || !boundaryAfter(s, m[1]) {
			continue
		}
		b.WriteString(s[last:m[0]])
		b.WriteString(to)
		end := m[1]
		for end < len(s) && !strings.ContainsRune(" \t\r\n\"',;:", rune(s[end])) {
			end++
		}
		b.WriteString(strings.ReplaceAll(s[m[1]:end], `\`, "/"))
		last = end
	}
	b.WriteString(s[last:])
	return b.String()
}

func hasPath(s, p string) bool {
	for _, m := range rootRE(p).FindAllStringIndex(s, -1) {
		if boundaryBefore(s, m[0]) && boundaryAfter(s, m[1]) {
			return true
		}
	}
	return false
}

func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-'
}

func boundaryBefore(s string, i int) bool { return i == 0 || !isNameByte(s[i-1]) }

func boundaryAfter(s string, end int) bool {
	return end >= len(s) || s[end] == '/' || !isNameByte(s[end])
}

// DefaultHome is the operator's home dir for a PathMap.
func DefaultHome() string {
	h, _ := os.UserHomeDir()
	return h
}
