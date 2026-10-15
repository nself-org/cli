package deploy

// Purpose: the symlink rule for the host's project dir (lead ruling, fix round 3).
//          A directory under remote_path may be a symlink only when its real path
//          is inside the real remote_path (trailing "/" boundary). Sync, restore,
//          prune and remove then go through the real path, re-resolved right
//          before each write or delete. A link that leaves remote_path is
//          refused with E485 before the lock and the tar, nothing sent.
//          Also here: the host guards that run before the lock. A nginx/ file
//          the host holds without the marker is never overwritten
//          (CheckNotOverwritingForeign, E485), and a hand-written
//          nginx/conf.d/<name>.conf is kept and dropped from the set
//          (KeptHostConfD, D-0424).
// Inputs:  a Remote, the remote path, relative paths or directories.
// Outputs: dir -> real path for links that stay inside; an E485 error otherwise.
// Constraints: one quoted remote script per call. Everything reachable through
//              the dirs the tar follows (nginx/, hasura/, .nself/plugins/, top-level
//              files), link chains included (find -L), must resolve inside
//              remote_path, because the tar dereferences them (tar -h). The backup
//              dir is a root too: the tar is written there.

import (
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// layoutRoots are directories always checked, with the tar's follow-links sources.
var layoutRoots = []string{"nginx", "hasura", ".nself", ".nself/plugins", BackupDir}

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
for s in nginx hasura .nself/plugins; do if [ -e "$s" ]; then find -L "$s" -exec sh -c 'for f; do r=$(readlink -f -- "$f" 2>/dev/null || realpath -- "$f" 2>/dev/null); case "$r/" in "$0/"*) ;; *) printf "OUTSIDE %%s\t%%s\n" "$(printf %%s "$f" | tr -c "[:print:]" "?")" "$(printf %%s "${r:-?}" | tr -c "[:print:]" "?")";; esac; done' "$base" {} + 2>/dev/null; fi; done
exit 0`, q(remotePath), strings.Join(quoteAll(dirs), " "), strings.Join(quoteAll(tarSources), " "))
	raw, err := r.Run(ctx, shScript(script))
	if err != nil {
		return nil, fmt.Errorf("resolving symlinked directories on %s: %w", r.Label(), err)
	}
	return parseResolved(string(raw))
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
rc=0
for f in "$@"; do d=$(real_of "$f") || continue; act "$f" "$d/$(basename "$f")" || rc=1; done
exit "$rc"`, q(remotePath), strings.Join(quoteAll(paths), " "), body)
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

// CheckNotOverwritingForeign refuses (E485 `unmarked file would be overwritten`,
// nothing sent) when a nginx/ file of the set already exists on the host without
// the nself generated marker in its first lines and is not in the previous deploy's
// local record (previous) or in the host's own shipped list (whoever can edit that
// list can edit the nginx files directly, so it grants nothing; deletes stay driven by
// the local record): such a file is hand written, or another tool's. The
// printed paths are always taken from the set, never from the host's output.
func CheckNotOverwritingForeign(ctx context.Context, r Remote, remotePath string, files, previous []string) error {
	known := map[string]bool{}
	for _, f := range previous {
		known[f] = true
	}
	var cand []string
	for _, f := range files {
		if strings.HasPrefix(f, "nginx/") && !known[f] && pruneable(f) {
			cand = append(cand, f)
		}
	}
	if len(cand) == 0 {
		return nil
	}
	script := fmt.Sprintf(`cd %s 2>/dev/null || exit 0; for f in %s; do if [ -f "$f" ] && ! head -n 5 -- "$f" 2>/dev/null | grep -qi %s && ! grep -qxF -- "$f" %s 2>/dev/null; then printf 'UNMARKED %%s\n' "$f"; fi; done; exit 0`,
		q(remotePath), strings.Join(quoteAll(cand), " "), q(GeneratedMarker), q(ShippedFile))
	out, err := r.Run(ctx, shScript(script))
	if err != nil {
		return fmt.Errorf("checking existing nginx files on %s: %w", r.Label(), err)
	}
	inSet := map[string]bool{}
	for _, f := range cand {
		inSet[f] = true
	}
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "UNMARKED "); ok && inSet[p] {
			return errs.New("E485", fmt.Sprintf("unmarked file would be overwritten: %q", p)).
				WithFix("move or delete that hand-written file on the host (or add the nself generated marker to it), then deploy again; nothing was sent")
		}
	}
	return nil
}

// BuildNginxMarker is the first-line stamp of every nginx conf the build
// renders. A host conf.d file counts as marked only when one of its first 5
// lines starts with exactly this text (case-sensitive): looser matches take
// "# hand-written, NOT generated by nself build" or "# Generated by nself ssl
// add" for ours. A test in cmd/commands keeps it equal to the build's marker.
const BuildNginxMarker = "# Generated by nself build"

// defaultServerConf is the generated conf.d file that declares the default
// server; two default servers on one host fail nginx -t.
const defaultServerConf = "nginx/conf.d/default.conf"

// KeptConfD is a conf.d candidate a deploy leaves out of the set.
type KeptConfD struct {
	Path string
	// DefaultServerIn is set when the file was kept out because the host already
	// declares a default_server in that file (relative to the remote path);
	// empty when the host holds a hand-written file at Path.
	DefaultServerIn string
}

// KeptHostConfD returns the nginx/conf.d/<name>.conf candidates of files that a
// deploy must not ship (D-0424): the path exists as a regular file with no line
// in its first 5 starting with BuildNginxMarker, or is a symlink or anything that
// is not a regular file. Also, when nginx/conf.d/default.conf is absent on the
// host and any other regular file under nginx/conf.d*/ or nginx/sites/ has a
// listen line with default_server, our default.conf is kept out (reason
// DefaultServerIn), since two default servers fail nginx -t. The previous
// deploy's record and the host's shipped list are ignored for conf.d: the host
// file alone decides, so a hand-written default.conf survives even when an
// earlier deploy shipped one. A missing file or a marked one is otherwise not
// kept. Printed paths are always taken from files, never from the host's output
// (the default_server file name is shown with control characters replaced). The
// caller drops the paths from the set and goes on; this is not an error.
//
// The marker is sent as a pattern with each space written [ ], the same match, so the
// fake host of the tests (which refuses a script that runs "nself ") does not take
// the pattern text for a command.
func KeptHostConfD(ctx context.Context, r Remote, remotePath string, files []string) ([]KeptConfD, error) {
	var cand []string
	inSet := map[string]bool{}
	for _, f := range files {
		if isGeneratedConfD(f) {
			cand = append(cand, f)
			inSet[f] = true
		}
	}
	if len(cand) == 0 {
		return nil, nil
	}
	script := fmt.Sprintf(`cd %s 2>/dev/null || exit 0
san() { printf '%%s' "$1" | tr -c '[:print:]' '?'; }
for f in %s; do
if [ -L "$f" ]; then printf 'KEEP %%s\n' "$f"
elif [ -f "$f" ]; then if ! head -n 5 -- "$f" 2>/dev/null | grep -q %s; then printf 'KEEP %%s\n' "$f"; fi
elif [ -e "$f" ]; then printf 'KEEP %%s\n' "$f"
elif [ "$f" = %s ]; then for g in nginx/conf.d*/* nginx/sites/*; do if [ -f "$g" ] && grep -Eq '(^|[{;])[[:space:]]*listen[[:space:]][^#;]*default_server' -- "$g" 2>/dev/null; then printf 'DSRV %%s\t%%s\n' "$f" "$(san "$g")"; break; fi; done
fi
done; exit 0`, q(remotePath), strings.Join(quoteAll(cand), " "), q("^"+strings.ReplaceAll(BuildNginxMarker, " ", "[ ]")), q(defaultServerConf))
	out, err := r.Run(ctx, shScript(script))
	if err != nil {
		return nil, fmt.Errorf("checking existing nginx/conf.d files on %s: %w", r.Label(), err)
	}
	var kept []KeptConfD
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "KEEP "); ok && inSet[p] {
			kept = append(kept, KeptConfD{Path: p})
			inSet[p] = false // each path once
		} else if rest, ok := strings.CutPrefix(line, "DSRV "); ok {
			if p, in, ok := strings.Cut(rest, "\t"); ok && inSet[p] {
				kept = append(kept, KeptConfD{Path: p, DefaultServerIn: in})
				inSet[p] = false
			}
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Path < kept[j].Path })
	return kept, nil
}
