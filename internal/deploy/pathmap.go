package deploy

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
		{"the local plugin dir", m.PluginDir}, {"the home dir", m.Home},
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

// replacePath replaces from with to wherever from stands as a whole path: not
// in the middle of a longer name before it, and followed by a separator or a
// non-name character.
func replacePath(s, from, to string) string {
	if from == "" {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, from)
		if i < 0 {
			break
		}
		end := i + len(from)
		if boundaryBefore(s, i) && boundaryAfter(s, end) {
			b.WriteString(s[:i])
			b.WriteString(to)
		} else {
			b.WriteString(s[:end])
		}
		s = s[end:]
	}
	b.WriteString(s)
	return b.String()
}

func hasPath(s, p string) bool {
	for off := 0; ; {
		i := strings.Index(s[off:], p)
		if i < 0 {
			return false
		}
		i += off
		if boundaryBefore(s, i) && boundaryAfter(s, i+len(p)) {
			return true
		}
		off = i + len(p)
	}
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
