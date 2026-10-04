#!/usr/bin/env bash
# Purpose: make every ADR 0021 gated branch findable, countable and documented.
#          A gated branch is a call to compat.V15() in non-test Go code outside
#          internal/compat. Each call needs the marker comment
#            // compat.V15(<ticket-id>): <old behaviour> -> <new behaviour>
#          on the same line or the line directly above.
# Usage:   compat-markers.sh               list "<file>:<line> <ticket-id>" per
#                                          gated branch (stdout only those lines;
#                                          nothing at all when there are none)
#          compat-markers.sh --check       exit 1 when a call is unmarked or the
#                                          "Gated behaviours" table in
#                                          .github/wiki/Compat-V15.md is stale
#          compat-markers.sh --write-wiki  regenerate that table from the markers
# Outputs: exit 0 when clean; exit 1 (reasons on stderr) when a compat.V15() call
#          has no well-formed marker, or --check finds a stale table.
# Constraints: bash 3.2 compatible. Scans untracked files too, so a new
#          uncommitted file is checked. Tests (*_test.go) are neither checked nor
#          counted: they select modes with compattest. A line that is only a
#          comment is not a call and is skipped.
set -euo pipefail

MODE="list"
case "${1:-}" in
  "") ;;
  --check) MODE="check" ;;
  --write-wiki) MODE="write" ;;
  *) echo "usage: compat-markers.sh [--check|--write-wiki]" >&2; exit 2 ;;
esac

cd "$(git rev-parse --show-toplevel)"
WIKI=".github/wiki/Compat-V15.md"
BEGIN='<!-- BEGIN GENERATED:gated -->'
END='<!-- END GENERATED:gated -->'
MARK_RE='//[[:space:]]*compat\.V15\(([A-Za-z0-9._-]+)\):[[:space:]]*(.*)$'
PREV_RE='^[[:space:]]*'"$MARK_RE"
COMMENT_RE='^[[:space:]]*//'

TMP="$(mktemp -d "${TMPDIR:-/tmp}/compat-markers.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT
RECORDS="$TMP/records"   # file<TAB>line<TAB>ticket<TAB>old<TAB>new
: > "$RECORDS"
BAD=0

# scan fills RECORDS and sets BAD=1 (reasons on stderr) for any bad call.
scan() {
  local hit file rest line text prev spec old new ticket
  while IFS= read -r hit; do
    [ -n "$hit" ] || continue
    file="${hit%%:*}"; rest="${hit#*:}"; line="${rest%%:*}"; text="${rest#*:}"
    if [[ "$text" =~ $COMMENT_RE ]]; then continue; fi
    spec=""
    if [[ "$text" =~ $MARK_RE ]]; then
      ticket="${BASH_REMATCH[1]}"; spec="${BASH_REMATCH[2]}"
    elif [ "$line" -gt 1 ]; then
      prev="$(sed -n "$((line - 1))p" "$file")"
      if [[ "$prev" =~ $PREV_RE ]]; then
        ticket="${BASH_REMATCH[1]}"; spec="${BASH_REMATCH[2]}"
      fi
    fi
    if [ -z "$spec" ]; then
      echo "compat-markers: $file:$line compat.V15() call has no marker (// compat.V15(<ticket-id>): <old> -> <new>)" >&2
      BAD=1; continue
    fi
    case "$spec" in
      *" -> "*) old="${spec%% -> *}"; new="${spec#* -> }" ;;
      *)
        echo "compat-markers: $file:$line marker lacks '<old> -> <new>'" >&2
        BAD=1; continue ;;
    esac
    printf '%s\t%s\t%s\t%s\t%s\n' "$file" "$line" "$ticket" "$old" "$new" >> "$RECORDS"
  done < <(git grep --untracked -n -E 'compat\.V15\(\)' -- '*.go' ':!*_test.go' ':!internal/compat/' ':!vendor/*' || true)
}

# render prints the Gated behaviours table for the current RECORDS.
render() {
  local ticket old new file n=0
  echo '| Ticket | v1.4 behaviour (old) | v1.5 behaviour (new) | File |'
  echo '|---|---|---|---|'
  while IFS="$(printf '\t')" read -r ticket old new file; do
    [ -n "$ticket" ] || continue
    old="${old//|/\\|}"; new="${new//|/\\|}"
    echo "| $ticket | $old | $new | \`$file\` |"
    n=$((n + 1))
  done < <(awk -F '\t' '{ printf "%s\t%s\t%s\t%s\n", $3, $4, $5, $1 }' "$RECORDS" | LC_ALL=C sort -u)
  if [ "$n" -eq 0 ]; then echo; echo '_No gated behaviours yet._'; fi
}

# current_block prints what the wiki holds between the markers.
current_block() {
  awk -v b="$BEGIN" -v e="$END" '
    $0 == e { on = 0 }
    on { print }
    $0 == b { on = 1 }' "$WIKI"
}

scan

case "$MODE" in
  list)
    LC_ALL=C sort -t "$(printf '\t')" -k1,1 -k2,2n "$RECORDS" | awk -F '\t' '{ printf "%s:%s %s\n", $1, $2, $3 }'
    ;;
  write|check)
    [ -f "$WIKI" ] || { echo "compat-markers: $WIKI missing" >&2; exit 1; }
    if ! grep -qxF "$BEGIN" "$WIKI" || ! grep -qxF "$END" "$WIKI"; then
      echo "compat-markers: $WIKI lacks the GENERATED:gated markers" >&2; exit 1
    fi
    render > "$TMP/table"
    if [ "$MODE" = "write" ]; then
      awk -v b="$BEGIN" -v e="$END" -v t="$TMP/table" '
        $0 == b { print; while ((getline l < t) > 0) print l; skip = 1; next }
        $0 == e { skip = 0 }
        !skip { print }' "$WIKI" > "$TMP/wiki.new"
      cat "$TMP/wiki.new" > "$WIKI"
    else
      current_block > "$TMP/current"
      if ! diff -q "$TMP/table" "$TMP/current" >/dev/null; then
        echo "compat-markers: Gated behaviours table in $WIKI is stale; run: bash scripts/ci/compat-markers.sh --write-wiki" >&2
        BAD=1
      fi
    fi
    ;;
esac

[ "$BAD" -eq 0 ] || exit 1
