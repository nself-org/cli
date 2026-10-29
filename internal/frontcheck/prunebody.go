// Copyright © 2024 nself-org. All rights reserved.

package frontcheck

// PruneBody is the per-path prune step: the marker rule, then the same
// server_name set as ServerNameSet (comments dropped, sorted, unique) compared
// with the recorded one. A path with no recorded set is never removed.
const PruneBody = `if [ -f "$2" ] && head -n 5 -- "$2" 2>/dev/null | grep -q @PAT@; then
want=; case "$1" in @WANT@esac
got=$(head -c @MAX@ -- "$2" 2>/dev/null | sed 's/#.*//' | grep -oE '(^|[^[:alnum:]_])server_name[[:space:]]+[^;]+;' | sed -E 's/^.*server_name[[:space:]]+//; s/;$//' | tr -s '[:space:]' '\n' | grep -v '^$' | LC_ALL=C sort -u | tr '\n' ' ')
got=${got% }
if [ -n "$want" ] && [ "$got" = "$want" ]; then rm -f -- "$2" && printf '%s\n' "$1"; else printf 'KEPT %s\n' "$1"; fi
fi`
