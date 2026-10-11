#!/usr/bin/env bash
# Purpose: guard the hand-written tables in the contract wiki pages against the
#          generated sources they describe (D-0254). The generated blocks in
#          Exit-Codes.md and error-codes.md are checked by tools/wikigen -check;
#          this script checks what wikigen does not own:
#            1. JSON-Output.md schemas table: every schema in schemas/index.json
#               has a row (commands/* via the pattern row) and every row names a
#               file that exists.
#            2. JSON-Output.md JSON support values: the backticked values in the
#               "JSON support value" sentence equal the set the command registry
#               (.github/command-registry.json) uses.
#            3. Every error code cited in JSON-Output.md or Exit-Codes.md is a
#               registered code (a row in the generated error-codes.md).
#            4. Exit-Codes.md "Changes in v1.5.0" table: every registered code a
#               row names (single codes and ranges) has that row's "Exit now" as
#               its Exit column in error-codes.md.
# Inputs:  --root DIR (default: the repo root). Reads .github/wiki/{JSON-Output,
#          Exit-Codes,error-codes}.md, schemas/index.json,
#          .github/command-registry.json under DIR.
# Outputs: one line per problem on stderr; "contract-check-wiki: OK" on stdout.
# Exit:    0 all checks pass, 1 a check failed, 2 usage or a missing input.
# Constraints: read-only; needs bash, awk, jq.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
while [ $# -gt 0 ]; do
  case "$1" in
    --root) root="${2:?--root needs a directory}"; shift 2 ;;
    *) echo "usage: $0 [--root DIR]" >&2; exit 2 ;;
  esac
done

wiki="$root/.github/wiki"
jsonout="$wiki/JSON-Output.md"
exitpage="$wiki/Exit-Codes.md"
errpage="$wiki/error-codes.md"
index="$root/schemas/index.json"
registry="$root/.github/command-registry.json"
for f in "$jsonout" "$exitpage" "$errpage" "$index" "$registry"; do
  [ -f "$f" ] || { echo "contract-check-wiki: missing input $f" >&2; exit 2; }
done

bad=0
problem() { echo "contract-check-wiki: $1" >&2; bad=1; }

# Registered codes and their Exit column: "| <a id=...></a>E001 | 2 | ...".
codes="$(awk -F'|' '/^\| <a id="e[0-9]+"><\/a>E[0-9]+ \|/ {
  c = $2; sub(/.*<\/a>/, "", c); gsub(/ /, "", c); e = $3; gsub(/ /, "", e); print c, e }' "$errpage")"
[ -n "$codes" ] || { echo "contract-check-wiki: no error-code rows parsed from $errpage" >&2; exit 2; }
exit_of() { printf '%s\n' "$codes" | awk -v c="$1" '$1 == c { print $2 }'; }

# 1. Schemas table.
rows="$(grep -oE '^\| `schemas/[^`]+` \|' "$jsonout" | sed -E 's/^\| `schemas\/([^`]+)` \|/\1/' || true)"
[ -n "$rows" ] || problem "JSON-Output.md: no schemas table rows found"
while IFS= read -r p; do
  [ -n "$p" ] || continue
  case "$p" in *'<'*) continue ;; esac
  [ -f "$root/schemas/$p" ] || problem "JSON-Output.md names schemas/$p, which does not exist"
done <<< "$rows"
pattern_row=0
printf '%s\n' "$rows" | grep -qx 'commands/<command>.v1.schema.json' && pattern_row=1
while IFS= read -r p; do
  case "$p" in
    commands/*) [ "$pattern_row" = 1 ] || problem "JSON-Output.md has no schemas/commands/<command>.v1.schema.json row for schemas/$p" ;;
    *) printf '%s\n' "$rows" | grep -qxF "$p" || problem "schemas/$p is in schemas/index.json but has no row in the JSON-Output.md schemas table" ;;
  esac
done < <(jq -r '.schemas[].path' "$index")

# 2. JSON support values.
documented="$(grep -m1 'JSON support value' "$jsonout" | grep -oE '`[a-z]+`' | tr -d '`' | sort -u || true)"
[ -n "$documented" ] || problem "JSON-Output.md: no \"JSON support value\" sentence with backticked values found"
used="$(jq -r '[.. | objects | select(has("json")) | .json | strings] | unique[]' "$registry")"
[ -n "$used" ] || problem "no json support values found in .github/command-registry.json"
[ "$documented" = "$used" ] || problem "JSON-Output.md documents JSON support values [$(echo $documented)] but the registry uses [$(echo $used)]"

# 3. Cited codes exist.
for page in "$jsonout" "$exitpage"; do
  for c in $(grep -oE '\bE[0-9]{3}\b' "$page" | sort -u); do
    [ -n "$(exit_of "$c")" ] || problem "$(basename "$page") cites $c, which is not a registered error code"
  done
done

# 4. The v1.5.0 changes table: | Failure | Code | Exit before | Exit now | Commands |.
changes="$(awk '/^## Changes in v1\.5\.0/ { on = 1; next } on && /^## / { on = 0 } on && /^\| / && $0 !~ /^\| (Failure|-)/' "$exitpage" | grep -E '\| E[0-9]{3}' || true)"
[ -n "$changes" ] || problem "Exit-Codes.md: no rows parsed from the Changes in v1.5.0 table"
while IFS= read -r row; do
  [ -n "$row" ] || continue
  cell="$(printf '%s' "$row" | awk -F'|' '{ print $3 }')"
  now="$(printf '%s' "$row" | awk -F'|' '{ gsub(/ /, "", $5); print $5 }')"
  checked=0
  for tok in $(printf '%s' "$cell" | tr ',' ' '); do
    if [[ "$tok" =~ ^E([0-9]{3})-E([0-9]{3})$ ]]; then
      lo=$((10#${BASH_REMATCH[1]})); hi=$((10#${BASH_REMATCH[2]}))
      for c in $(printf '%s\n' "$codes" | awk -v lo="$lo" -v hi="$hi" '{ n = substr($1, 2) + 0; if (n >= lo && n <= hi) print $1 }'); do
        e="$(exit_of "$c")"; checked=$((checked + 1))
        [ "$e" = "$now" ] || problem "Exit-Codes.md says $c (in $tok) now exits $now; error-codes.md says $e"
      done
    elif [[ "$tok" =~ ^E[0-9]{3}$ ]]; then
      e="$(exit_of "$tok")"; checked=$((checked + 1))
      [ "$e" = "$now" ] || problem "Exit-Codes.md says $tok now exits $now; error-codes.md says ${e:-unregistered}"
    fi
  done
  [ "$checked" -gt 0 ] || problem "Exit-Codes.md: no registered code checked in the v1.5.0 row: $(printf '%s' "$row" | cut -c1-80)"
done <<< "$changes"

[ "$bad" = 0 ] || exit 1
echo "contract-check-wiki: OK"
