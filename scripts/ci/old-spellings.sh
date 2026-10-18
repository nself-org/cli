#!/usr/bin/env bash
# Purpose: report every place that still names an old (shim) command spelling,
#          so the v1.6.0 removal (ADR 0016, EPIC P7-CANON D13) starts from a
#          list instead of a search. A report, not a gate: it always exits 0
#          when it could read the canon table.
# Usage:   old-spellings.sh            (run from anywhere inside the repo)
# Inputs:  .github/wiki/Command-Canon.md, the generated old -> new table
#          (tools/cmdinventory -format canon). Rows of kind "breakout" are
#          skipped: the command keeps its spelling, only its home moves.
# Outputs: stdout, one line per hit:
#            <file>:<line> <old path> -> <new path>
#          for every Go string literal in cmd/ and internal/ and every line in
#          .github/wiki/ and README.md that names an old path. The canon page
#          and the Changelog are excluded (they list old paths on purpose). A
#          line naming several old paths is reported once, with the longest.
#          stderr: a one-line count. Attach the stdout to P7-SHIP-09 evidence.
# Exit:    0 report written, 1 canon table unreadable or empty.
set -euo pipefail

root=$(git rev-parse --show-toplevel)
cd "$root"
canon=.github/wiki/Command-Canon.md
[ -r "$canon" ] || { echo "old-spellings: cannot read $canon" >&2; exit 1; }

# rows: "<old path>\t<new path>", longest old path first so a line that names
# "nself migrate run" is reported for that row and not for "nself migrate".
rows=$(awk -F'|' '
  $2 ~ /`nself / && $4 !~ /breakout/ {
    old = $2; new = $3
    gsub(/`/, "", old); gsub(/`/, "", new)
    gsub(/^ +| +$/, "", old); gsub(/^ +| +$/, "", new)
    sub(/ \(plugin .*$/, "", new)
    print length(old) "\t" old "\t" new
  }' "$canon" | sort -t "$(printf '\t')" -k1,1nr | cut -f2-)
[ -n "$rows" ] || { echo "old-spellings: no rows in $canon" >&2; exit 1; }

hits=$(mktemp "${TMPDIR:-/tmp}/old-spellings.XXXXXX")
trap 'rm -f "$hits"' EXIT
tab=$(printf '\t')
# git grep exits 1 for "no match", which is fine; any other failure is real
grep_ok() { git grep "$@" || [ $? -eq 1 ]; }
# a path is complete when the next character cannot continue a command word
end='($|[^A-Za-z0-9_-])'

while IFS="$tab" read -r old new; do
  [ -n "$old" ] || continue
  esc=$(printf '%s' "$old" | sed 's/[][\.*^$()+?{}|]/\\&/g')
  {
    grep_ok -nE "[\"\`][^\"\`]*${esc}${end}" -- 'cmd/*.go' 'internal/*.go'
    grep_ok -nE "${esc}${end}" -- '.github/wiki' README.md \
      ':!.github/wiki/Command-Canon.md' ':!.github/wiki/Changelog.md'
  } | awk -F: -v old="$old" -v new="$new" '{ print $1 ":" $2 "\t" old " -> " new }' >>"$hits"
done <<<"$rows"

# first occurrence wins, and rows were walked longest-first
awk -F'\t' '!seen[$1]++ { print $1 " " $2 }' "$hits" | sort -t: -k1,1 -k2,2n
echo "old-spellings: $(awk -F'\t' '!seen[$1]++' "$hits" | wc -l | tr -d ' ') line(s) name an old spelling" >&2
