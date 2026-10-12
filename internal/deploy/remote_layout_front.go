package deploy

// Purpose: the symlink rule for the fronting stack's directory (D-0425), the
//          front-only twin of ResolveDirs. It resolves exactly nginx and
//          nginx/sites and the front files the deploy ships or may prune, and
//          touches no other name in the front root: the fronting stack's .env*,
//          compose files, ssl/ and everything else are neither read nor
//          readlinked.
// Inputs:  a Remote, the front root, the relative front file paths.
// Outputs: dir -> real path for the dirs reached through a symlink that stays
//          inside the real front root; E485 for a dir or a front file whose real
//          path leaves it.
// Constraints: one quoted remote script per call. The same output grammar and
//              the same E485 as ResolveDirs (parseResolved).

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/nself-org/cli/internal/errs"
)

// frontDirs are the only directories of the front root the apply looks at.
var frontDirs = []string{"nginx", "nginx/sites"}

// ResolveFrontDirs resolves frontDirs and the files (relative to front) on the host.
func ResolveFrontDirs(ctx context.Context, r Remote, front string, files []string) (map[string]string, error) {
	script := fmt.Sprintf(`rp=%s; cd -P "$rp" 2>/dev/null || exit 0; base=$(pwd -P)
res() { readlink -f -- "$1" 2>/dev/null || realpath -- "$1" 2>/dev/null; }
safe() { case "$1" in *[[:cntrl:]]*) return 1;; esac; return 0; }
san() { printf '%%s' "$1" | tr -c '[:print:]' '?'; }
bad() { printf 'OUTSIDE %%s\t%%s\n' "$(san "$1")" "$(san "$2")"; }
inside() { case "$1/" in "$base/"*) return 0;; esac; return 1; }
for d in %s; do if [ -d "$d" ]; then x=$(cd -P "$d" 2>/dev/null && pwd -P && echo X) || x=; x=${x%%X}; x=${x%%?}; if ! safe "$x"; then bad "$d" "$x"; elif inside "$x"; then [ "$x" = "$base/$d" ] || printf 'LINK %%s\t%%s\n' "$d" "$x"; else bad "$d" "${x:-?}"; fi; fi; done
for l in %s; do if [ -L "$l" ]; then x=$(res "$l"); inside "$x" || bad "$l" "${x:-?}"; fi; done
exit 0`, q(front), strings.Join(quoteAll(frontDirs), " "), strings.Join(quoteAll(append([]string{}, files...)), " "))
	raw, err := r.Run(ctx, shScript(script))
	if err != nil {
		return nil, fmt.Errorf("resolving the fronting stack's nginx dirs on %s: %w", r.Label(), err)
	}
	return parseResolved(string(raw))
}

// parseResolved turns the OUTSIDE and LINK lines of a resolve script into the
// dir -> real path map, or the E485 for the paths that leave the root.
func parseResolved(raw string) (map[string]string, error) {
	out := map[string]string{}
	var outside []string
	for _, line := range strings.Split(raw, "\n") {
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
