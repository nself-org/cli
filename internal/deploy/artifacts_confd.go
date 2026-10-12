package deploy

// Purpose: the one conf.d path a deploy may ship (D-0424). The generator writes
//          nginx/conf.d/default.conf (marker on line 1); a v1.5 deploy ships it
//          so the host's nginx has the same default server the build rendered.
//          Every other conf.d* path stays hand-managed and is never shipped,
//          pruned or overwritten.
//          api-docs.conf moved to nginx/sites (orchestrator_build_finish.go), so
//          only a legacy conf.d/api-docs.conf is removed by the build: there is
//          nothing extra to ship or clean for it here.
// Inputs:  a path relative to the remote path.
// Outputs: whether it is a shippable generated conf.d file; an ArtifactSet
//          without a list of paths.
// Constraints: exactly nginx/conf.d/<name>.conf, a single segment, no control
//              characters, no leading dot. Not conf.d-<env>, not nested.

import (
	"path"
	"strings"
	"unicode"
)

// isGeneratedConfD reports whether rel is nginx/conf.d/<name>.conf with a single
// safe segment for <name>.conf (no separators, no control characters, no leading
// dot, a non-empty stem).
func isGeneratedConfD(rel string) bool {
	name, ok := strings.CutPrefix(rel, "nginx/conf.d/")
	if !ok || rel != path.Clean(rel) || !strings.HasSuffix(name, ".conf") || len(name) == len(".conf") {
		return false
	}
	return !strings.HasPrefix(name, ".") && !strings.ContainsAny(name, `/\`) && !strings.ContainsFunc(name, unicode.IsControl)
}

// shippable reports whether rel may be shipped, restored or removed by a deploy:
// a pruneable path, or a generated conf.d file. PrunePrevious keeps using
// pruneable, so a conf.d file is never pruned.
func shippable(rel string) bool { return pruneable(rel) || isGeneratedConfD(rel) }

// Without returns a copy of the set that leaves out paths (relative to the
// remote path) from Files and EnvFiles. The receiver is not changed. The staged
// files stay in the staging dir; they are no longer listed, so nothing syncs
// them.
func (s ArtifactSet) Without(paths []string) ArtifactSet {
	if len(paths) == 0 {
		return s
	}
	drop := make(map[string]bool, len(paths))
	for _, p := range paths {
		drop[p] = true
	}
	keep := func(in []string) []string {
		var out []string
		for _, f := range in {
			if !drop[f] {
				out = append(out, f)
			}
		}
		return out
	}
	s.Files, s.EnvFiles = keep(s.Files), keep(s.EnvFiles)
	return s
}
