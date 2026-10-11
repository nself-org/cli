package deploy

// Purpose: the symlink rule for the host's project dir (lead ruling, fix round 3).
//          A directory under remote_path may be a symlink only when its real path
//          is inside the real remote_path (trailing "/" boundary). Sync, restore,
//          prune and remove then go through the real path, re-resolved right
//          before each write or delete. A link that leaves remote_path is
//          refused with E485 before the lock and the tar, nothing sent.
// Inputs:  a Remote, the remote path, relative paths or directories.
// Outputs: dir -> real path for links that stay inside; an E485 error otherwise.
// Constraints: one quoted remote script per call. Links inside the dirs the tar
//              follows (nginx/, hasura/, .nself/plugins/, top-level files) are
//              checked too, because the tar dereferences them (tar -h).

import (
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/nself-org/cli/internal/errs"
)

// layoutRoots are directories always checked, with the tar's follow-links sources.
var layoutRoots = []string{"nginx", "hasura", ".nself", ".nself/plugins"}

// layoutDirs are the distinct directories (relative, not ".") that hold rels,
// plus layoutRoots.
func layoutDirs(rels []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range append(append([]string{}, layoutRoots...), dirsOf(rels)...) {
		if d != "." && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

func dirsOf(rels []string) []string {
	out := make([]string, 0, len(rels))
	for _, rel := range rels {
		out = append(out, path.Dir(rel))
	}
	return out
}

// ResolveDirs resolves every dir on the host (cd -P). It returns dir -> real path
// for the dirs reached through a symlink that stays inside the real remote_path,
// and fails with E485 (`symlink outside remote_path`) when a dir, or a link inside
// the tar's sources, resolves elsewhere or cannot be resolved.
func ResolveDirs(ctx context.Context, r Remote, remotePath string, dirs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(dirs) == 0 {
		return out, nil
	}
	script := fmt.Sprintf(`rp=%s; cd -P "$rp" 2>/dev/null || exit 0; base=$(pwd -P)
res() { readlink -f -- "$1" 2>/dev/null || realpath -- "$1" 2>/dev/null; }
safe() { case "$1" in *[[:cntrl:]]*) return 1;; esac; return 0; }
san() { printf '%%s' "$1" | tr -c '[:print:]' '?'; }
bad() { printf 'OUTSIDE %%s\t%%s\n' "$(san "$1")" "$(san "$2")"; }
inside() { case "$1/" in "$base/"*) return 0;; esac; return 1; }
for d in %s; do if [ -d "$d" ]; then x=$(cd -P "$d" 2>/dev/null && pwd -P && echo X) || x=; x=${x%%X}; x=${x%%?}; if ! safe "$x"; then bad "$d" "$x"; elif inside "$x"; then [ "$x" = "$base/$d" ] || printf 'LINK %%s\t%%s\n' "$d" "$x"; else bad "$d" "${x:-?}"; fi; fi; done
for l in %s .env.*; do if [ -L "$l" ]; then x=$(res "$l"); inside "$x" || bad "$l" "${x:-?}"; fi; done
for s in nginx hasura .nself/plugins; do if [ -e "$s" ]; then find -H "$s" -type l 2>/dev/null | while IFS= read -r l; do x=$(res "$l"); inside "$x" || bad "$l" "${x:-?}"; done; fi; done
exit 0`, q(remotePath), strings.Join(quoteAll(dirs), " "), strings.Join(quoteAll(tarSources), " "))
	raw, err := r.Run(ctx, shScript(script))
	if err != nil {
		return nil, fmt.Errorf("resolving symlinked directories on %s: %w", r.Label(), err)
	}
	var outside []string
	for _, line := range strings.Split(string(raw), "\n") {
		if rest, ok := strings.CutPrefix(line, "OUTSIDE "); ok {
			if p, real, ok := strings.Cut(rest, "\t"); ok {
				outside = append(outside, fmt.Sprintf("%q -> %q", p, real))
			}
		} else if rest, ok := strings.CutPrefix(line, "LINK "); ok {
			if d, real, ok := strings.Cut(rest, "\t"); ok && strings.HasPrefix(real, "/") {
				if strings.ContainsFunc(real, unicode.IsControl) { // never trust a host path with control characters
					outside = append(outside, fmt.Sprintf("%q -> %q", d, real))
					continue
				}
				out[d] = real
			}
		}
	}
	if len(outside) > 0 {
		return nil, errs.New("E485", "symlink outside remote_path: "+strings.Join(outside, ", ")).
			WithFix("replace the link with a real directory or point it inside the project directory; nothing was sent")
	}
	return out, nil
}

// guardedScript runs body once per path as the function act (arguments: the
// relative path, then its absolute path under the path's real directory), from
// remotePath. Every path is resolved first; one whose directory leaves the real
// remote_path (or that is itself a symlink) prints "REFUSE <path>" and nothing
// is run. The directory is resolved again just before each act. A path whose
// directory does not exist is skipped.
func guardedScript(remotePath string, paths []string, body string) string {
	return fmt.Sprintf(`rp=%s; cd -P "$rp" || exit 1; base=$(pwd -P); set -- %s
act() { %s; }
real_of() { [ -d "$base/$(dirname "$1")" ] || return 2; x=$(cd -P "$base/$(dirname "$1")" 2>/dev/null && pwd -P) || return 1; case "$x/" in "$base/"*) printf '%%s' "$x";; *) return 1;; esac; }
refuse=0
for f in "$@"; do if [ -L "$base/$f" ]; then printf 'REFUSE %%s\n' "$f"; refuse=1; continue; fi; real_of "$f" >/dev/null; [ $? -eq 1 ] && { printf 'REFUSE %%s\n' "$f"; refuse=1; }; done
if [ "$refuse" = 1 ]; then exit 0; fi
for f in "$@"; do d=$(real_of "$f") || continue; act "$f" "$d/$(basename "$f")"; done`, q(remotePath), strings.Join(quoteAll(paths), " "), body)
}

// refusedPaths turns the REFUSE lines of a guardedScript into an error.
func refusedPaths(out []byte) error {
	var bad []string
	for _, l := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(l, "REFUSE "); ok {
			bad = append(bad, fmt.Sprintf("%q", p))
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to delete %s: a symlink on the path leaves remote_path (or the file is a symlink); nothing was deleted", strings.Join(bad, ", "))
}

// noticeLinks tells the operator, once per directory that holds a shipped file,
// that the write goes through a symlink to another place inside remote_path.
func noticeLinks(w io.Writer, linked map[string]string, shipped []string) {
	for _, d := range layoutDirsOnly(shipped) {
		if real, ok := linked[d]; ok {
			emit(w, "  [notice] %q is a symlink to %q, writing through it", d, real)
		}
	}
}
