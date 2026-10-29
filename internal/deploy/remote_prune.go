package deploy

// Purpose: remote prune helpers extracted from remote_sync.go to keep that
//          file under the 300-line cap (TestFileSizeBudgetNotExceeded) and
//          shrink internal/deploy below its package-size ceiling. The prune
//          removes only files that carry the generated marker and were shipped
//          by a previous deploy; nothing under conf.d* and no unmarked file
//          is ever a candidate.

import (
	"context"
	"fmt"
	"path"
	"strings"
)

// PrunePrevious removes the files the previous deploy shipped that the new set
// dropped and that carry the generated marker (a plugin fragment under
// .nself/plugins/<slug>/ needs no marker: it is a copy of the operator's file).
// What may be removed is decided by recorded, the local record of that deploy
// (deploy.json); the host's own shipped list (informational) can only narrow it.
// With no record nothing is removed. It returns the files it removed.
func PrunePrevious(ctx context.Context, r Remote, remotePath string, keep map[string]bool, recorded []string) ([]string, error) {
	if len(recorded) == 0 {
		return nil, nil
	}
	known := map[string]bool{}
	for _, f := range recorded {
		known[f] = true
	}
	prev, err := remoteLines(ctx, r, path.Join(remotePath, ShippedFile))
	if err != nil {
		return nil, err
	}
	var cand, plugins []string
	for _, f := range prev {
		switch {
		case !known[f] || keep[f]:
		case isPluginFragment(f):
			plugins = append(plugins, f)
		default:
			cand = append(cand, f)
		}
	}
	removed, err := PruneMarked(ctx, r, remotePath, cand)
	if err != nil {
		return removed, err
	}
	gone, err := pruneShippedPlugins(ctx, r, remotePath, plugins)
	return append(removed, gone...), err
}

// PruneMarked removes, from the candidates (paths relative to remotePath), the
// files that carry the generated marker in their first lines. Entries that are
// not safe project-relative paths (see pruneable) are skipped. A batch in which
// any path resolves outside the project (or is itself a symlink) is refused
// whole: nothing in it is deleted and the error names the path. It returns the
// files it removed.
func PruneMarked(ctx context.Context, r Remote, remotePath string, candidates []string) ([]string, error) {
	body := fmt.Sprintf(`if [ -f "$2" ] && head -n 5 -- "$2" 2>/dev/null | grep -qi %s; then rm -f -- "$2" && printf '%%s\n' "$1"; fi`, q(GeneratedMarker))
	return pruneGuarded(ctx, r, remotePath, candidates, body)
}

// pruneShippedPlugins removes the plugin compose fragments (.nself/plugins/<slug>/...)
// the previous deploy shipped and this one dropped. A fragment is copied from the
// operator's plugin dir and carries no generated marker, so the marker is not
// asked for: the candidates come from the local deploy record intersected with
// the host's shipped list (PrunePrevious), and the symlink and realpath guard is
// the one PruneMarked uses. Candidates that are not plugin fragments are skipped.
func pruneShippedPlugins(ctx context.Context, r Remote, remotePath string, candidates []string) ([]string, error) {
	var cand []string
	for _, f := range candidates {
		if isPluginFragment(f) {
			cand = append(cand, f)
		}
	}
	return pruneGuarded(ctx, r, remotePath, cand, `if [ -f "$2" ]; then rm -f -- "$2" && printf '%s\n' "$1"; fi`)
}

// isPluginFragment reports whether rel is a file at least one directory below
// .nself/plugins/<slug>/ that pruneable accepts.
func isPluginFragment(rel string) bool {
	rest, ok := strings.CutPrefix(rel, ".nself/plugins/")
	if !ok || !pruneable(rel) {
		return false
	}
	slug, file, ok := strings.Cut(rest, "/")
	return ok && slug != "" && file != ""
}

// pruneGuarded runs body (see guardedScript) over the pruneable candidates in
// batches and returns what it printed as removed.
func pruneGuarded(ctx context.Context, r Remote, remotePath string, candidates []string, body string) ([]string, error) {
	var cand []string
	for _, f := range candidates {
		if pruneable(f) {
			cand = append(cand, f)
		}
	}
	var removed []string
	for len(cand) > 0 {
		n := min(len(cand), 50)
		out, err := r.Run(ctx, shScript(guardedScript(remotePath, cand[:n], body)))
		if err != nil {
			return removed, fmt.Errorf("pruning generated files on %s: %w", r.Label(), err)
		}
		if err := refusedPaths(out); err != nil {
			return removed, err
		}
		for _, f := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if f != "" {
				removed = append(removed, f)
			}
		}
		cand = cand[n:]
	}
	return removed, nil
}

// pruneable reports whether rel is a safe project-relative file path: not empty,
// clean, no ".", ".." or empty component, not absolute, not the host's own .env,
// no newline, and not under a conf.d* dir.
func pruneable(rel string) bool {
	if rel == "" || rel != path.Clean(rel) || rel == ".env" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\n") {
		return false
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.HasPrefix(seg, "conf.d") {
			return false
		}
	}
	return true
}
