package deploy

// Purpose: recheck front-file ownership immediately before an action.
// Inputs: recorded server_name sets keyed by safe front-relative paths.
// Outputs: shell checks that fail when a sibling replaced a file.
// Constraints: absent files are safe; existing files need a recorded set.

import (
	"fmt"
	"sort"
	"strings"
)

// stagedFrontNames collects ownership sets of files this apply ships.
func stagedFrontNames(stage string, paths []string) (map[string][]string, error) {
	names := map[string][]string{}
	for _, p := range paths {
		n, err := localServerNames(stage, p)
		if err != nil {
			return nil, err
		}
		names[p] = n
	}
	return names, nil
}

// frontOwnershipShell defines front_owned(relative, absolute). The awk parser
// mirrors serverNameSet: strip comments, collect every directive, sort uniquely.
func frontOwnershipShell(names map[string][]string) string {
	var cases strings.Builder
	paths := make([]string, 0, len(names))
	for p := range names {
		if frontPathOK(p) {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(&cases, "%s) expected=%s;;\n", q(p), q(strings.Join(names[p], "\n")))
	}
	return fmt.Sprintf(`front_owned() {
if [ ! -e "$2" ] && [ ! -L "$2" ]; then return 0; fi
[ -f "$2" ] && [ ! -L "$2" ] || return 1
head -n 5 -- "$2" | grep -q %s || return 1
case "$1" in
%s*) return 1;;
esac
names=$(LC_ALL=C awk '{ sub(/#.*/, ""); line=$0; while (match(line, /(^|[^[:alnum:]_])server_name[ \t]+[^;]+;/)) { value=substr(line,RSTART,RLENGTH); sub(/^.*server_name[ \t]+/, "", value); sub(/;$/, "", value); n=split(value, fields, /[[:space:]]+/); for (i=1;i<=n;i++) if (fields[i]!="") print fields[i]; line=substr(line,RSTART+RLENGTH); } }' "$2") || return 1
actual=$(printf '%%s\n' "$names" | LC_ALL=C sort -u) || return 1
[ "$actual" = "$expected" ] || { printf 'front server_name changed: %%s\n' "$1" >&2; return 1; }
}
`, q("^"+strings.ReplaceAll(BuildNginxMarker, " ", "[ ]")), cases.String())
}

// frontOwnershipCheck checks the entire change set before a restore starts.
func frontOwnershipCheck(root string, paths []string, names map[string][]string) string {
	if names == nil || len(paths) == 0 {
		return ""
	}
	return frontOwnershipShell(names) + fmt.Sprintf(`for f in %s; do front_owned "$f" %s/"$f" || exit 1; done; `,
		strings.Join(quoteAll(paths), " "), q(root))
}
