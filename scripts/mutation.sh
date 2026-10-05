#!/usr/bin/env bash
# scripts/mutation.sh PKG_DIR [extra gremlins flags]
#
# Purpose: mutation-test one package with go-gremlins and fail on any surviving
# mutant. A surviving mutant means a code change no test notices; on a security
# seam (internal/license) that is a hole.
# Usage:   bash scripts/mutation.sh internal/license
# Inputs:  PKG_DIR relative to the repo root (a path under sdk/go runs inside
#          the sdk/go module). No global install: gremlins runs through
#          `go run` at the release pinned below, fetched from the module proxy.
# Output:  gremlins' report on stdout; the gremlins output is also kept in
#          $MUTATION_OUT (default: a temp file) and its path is printed.
# Exit:    0 no mutant lived; 1 at least one lived (or gremlins failed to run).
#          --only f.go,g.go  judge only those files (see below); --allow FILE.
# Env:     GREMLINS_WORKERS (default 4), MUTATION_TIMEOUT_COEFFICIENT (default 5),
#          MUTATION_MAX_TIMEOUTS (default 8; --only fails above it).
set -uo pipefail

GREMLINS_VERSION="v0.6.0"

if [ $# -lt 1 ]; then
  echo "usage: bash scripts/mutation.sh PKG_DIR" >&2
  exit 2
fi
pkg="$1"
shift

# Own flags (everything else goes to gremlins unchanged):
#   --only a.go,b.go   judge only mutants in these non-test files of PKG_DIR; every
#                      other file is excluded from the run. A LIVED or NOT COVERED
#                      mutant in a listed file fails the run (exit 1).
#   --allow FILE       equivalent-mutant allowlist replacing the built-in
#                      MUTATION_EQUIVALENT list: one `MUTATOR at file.go:line:col | reason`
#                      per line; an entry without a reason is an error. Used with --only.
only=""
allow=""
passthru=()
while [ $# -gt 0 ]; do
  case "$1" in
    --only) only="${2:-}"; shift 2 || break ;;
    --only=*) only="${1#--only=}"; shift ;;
    --allow) allow="${2:-}"; shift 2 || break ;;
    --allow=*) allow="${1#--allow=}"; shift ;;
    *) passthru+=("$1"); shift ;;
  esac
done

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root" || exit 1
srcdir="$pkg"

# Module-aware: sdk/go is its own module.
case "$pkg" in
  sdk/go/*) cd sdk/go && pkg="./${pkg#sdk/go/}"; srcdir="${srcdir#sdk/go/}" ;;
  *) pkg="./${pkg#./}"; srcdir="${srcdir#./}" ;;
esac

out="${MUTATION_OUT:-$(mktemp "${TMPDIR:-/tmp}/mutation.XXXXXX")}"
export CGO_ENABLED=0
# -count=1: a cached test run makes gremlins' coverage baseline near-instant, its
# per-mutant timeout then shrinks with it, and every mutant "times out" (a false kill).
export GOFLAGS="${GOFLAGS:--mod=vendor -count=1}"

# gremlins itself is not vendored: build the pinned release into a temp dir
# (no global install), outside the vendored module graph.
bindir="$(mktemp -d "${TMPDIR:-/tmp}/gremlins-bin.XXXXXX")"
trap 'rm -rf "$bindir"' EXIT
if ! (cd "$bindir" && GOFLAGS= GOBIN="$bindir" go install "github.com/go-gremlins/gremlins/cmd/gremlins@${GREMLINS_VERSION}"); then
  echo "mutation: could not fetch gremlins ${GREMLINS_VERSION} from the module proxy" >&2
  exit 1
fi
gremlins() { "$bindir/gremlins" "$@"; }

# Equivalent mutants for --only runs, by gremlins id, each with the reason no test
# can tell the mutant from the original. Line numbers are those of the current
# source; a moved line needs its entry moved. Keep this list short and honest.
MUTATION_EQUIVALENT='
ARITHMETIC_BASE at validate.go:23:32 | const initialiser: Go emits no coverage block for a const, so gremlins can only report NOT COVERED; TestDefaultCheckIntervalIsSixHours pins the value
CONDITIONALS_BOUNDARY at checker.go:224:15 | `remaining < 0` vs `<= 0` in emitGraceWarning: at remaining == 0 both branches leave 0, so the output is identical
CONDITIONALS_NEGATION at cache_entry.go:124:37 | key-id filter in verifyLegacy is only a fast path: the rotation-window loop after it retries every key, so skipping the matching key changes no verdict
'

# --only mode. Passes: a file whose first line is `//go:build nself_devkeys` only
# compiles with that tag, so it is mutated in a second pass run with
# `--tags nself_devkeys`; every other listed file runs in the default-tag pass.
# Verdict: LIVED or NOT COVERED in a listed file, minus the allowlist, exits 1.
# One gremlins pass. $1 = tags ("" for none), $2 = file listing the base names to
# keep, $3 = report path. Runs from the module dir, so paths are relative to it.
mutation_only_pass() {
  local tags="$1" keep="$2" report="$3" f base args=() n=0
  for f in "$srcdir"/*.go; do
    base="$(basename "$f")"
    case "$base" in *_test.go) continue ;; esac
    if ! grep -qx "$base" "$keep"; then
      args+=(--exclude-files "(^|/)$(printf '%s' "$base" | sed 's/\./\\./g')\$")
    else
      n=$((n + 1))
    fi
  done
  [ "$n" -eq 0 ] && return 0
  [ -n "$tags" ] && args+=(--tags "$tags")
  gremlins unleash \
    --workers "${GREMLINS_WORKERS:-4}" \
    --timeout-coefficient "${MUTATION_TIMEOUT_COEFFICIENT:-5}" \
    ${args[@]+"${args[@]}"} ${passthru[@]+"${passthru[@]}"} "$pkg" 2>&1 | tee "$report"
  local st=${PIPESTATUS[0]}
  if [ "$st" -ne 0 ] && ! grep -q "Mutation testing completed" "$report"; then
    echo "mutation: gremlins failed to run (exit $st)" >&2
    return 1
  fi
  return 0
}

mutation_only_main() {
  local f base keepdef keepdev missing=0 list tmp
  tmp="$(mktemp -d "${TMPDIR:-/tmp}/mutation-only.XXXXXX")"
  keepdef="$tmp/keep-default"; keepdev="$tmp/keep-dev"
  : > "$keepdef"; : > "$keepdev"
  list="$(printf '%s' "$only" | tr ',' '\n')"
  for base in $list; do
    f="$root/$srcdir/$base"
    if [ ! -f "$f" ]; then
      echo "mutation: --only file not found (skipped): $srcdir/$base" >&2
      continue
    fi
    case "$base" in *_test.go) echo "mutation: --only names a test file: $base" >&2; missing=1; continue ;; esac
    if head -1 "$f" | grep -q '^//go:build nself_devkeys'; then
      echo "$base" >> "$keepdev"
    else
      echo "$base" >> "$keepdef"
    fi
  done
  [ "$missing" -ne 0 ] && { rm -rf "$tmp"; return 1; }

  printf '%s\n' "$list" > "$tmp/listed"
  : > "$out"
  mutation_only_pass "" "$keepdef" "$tmp/r1" || { rm -rf "$tmp"; return 1; }
  [ -f "$tmp/r1" ] && cat "$tmp/r1" >> "$out"
  mutation_only_pass "nself_devkeys" "$keepdev" "$tmp/r2" || { rm -rf "$tmp"; return 1; }
  [ -f "$tmp/r2" ] && cat "$tmp/r2" >> "$out"
  echo "mutation output: $out"

  # Allowlist: `MUTATOR at file.go:line:col | reason`; a reason is mandatory.
  # Default: the built-in list below; --allow FILE replaces it.
  : > "$tmp/allow"
  if [ -n "$allow" ]; then
    cp "$allow" "$tmp/allowsrc" || { rm -rf "$tmp"; return 1; }
  else
    printf '%s\n' "$MUTATION_EQUIVALENT" > "$tmp/allowsrc"
  fi
  local line id why bad=0
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in ''|'#'*) continue ;; esac
    id="${line%% | *}"; why="${line#* | }"
    if [ "$id" = "$line" ] || [ -z "$(printf '%s' "$why" | tr -d '[:space:]')" ]; then
      echo "mutation: allowlist entry without a reason: $line" >&2; bad=1; continue
    fi
    echo "$id" >> "$tmp/allow"
  done < "$tmp/allowsrc"
  [ "$bad" -ne 0 ] && { rm -rf "$tmp"; return 1; }

  # Bad = LIVED or NOT COVERED in a listed file and not allowlisted. TIMED OUT
  # counts as detected (the tests hung on the mutant), as in gremlins' efficacy.
  : > "$tmp/bad"
  sed -nE 's/^[[:space:]]*(LIVED|NOT COVERED)[[:space:]]+([A-Z_]+ at ([^:]+):[0-9]+:[0-9]+).*/\2/p' "$out" |
    sort -u | while IFS= read -r id; do
      base="${id#* at }"; base="${base%%:*}"
      printf '%s\n' "$list" | grep -qx "$base" || continue
      grep -qxF "$id" "$tmp/allow" && continue
      echo "$id"
    done > "$tmp/bad"
  local nbad nallow nto maxto="${MUTATION_MAX_TIMEOUTS:-8}"
  nto="$(sed -nE 's/^[[:space:]]*TIMED OUT[[:space:]]+[A-Z_]+ at ([^:]+):.*/\1/p' "$out" | grep -cxF -f "$tmp/listed" || true)"
  if [ "${nto:-0}" -gt "$maxto" ]; then
    echo "mutation: $nto mutants timed out (max $maxto): the timeout baseline is wrong, so timeouts are not kills; re-run" >&2
    rm -rf "$tmp"; return 1
  fi
  nbad="$(wc -l < "$tmp/bad" | tr -d ' ')"
  nallow="$(wc -l < "$tmp/allow" | tr -d ' ')"
  if [ "$nbad" -gt 0 ]; then
    echo "mutation: $nbad LIVED or NOT COVERED mutant(s) in: $only" >&2
    sed 's/^/  /' "$tmp/bad" >&2
    rm -rf "$tmp"; return 1
  fi
  echo "mutation: no surviving or uncovered mutants in: $only (allowlisted equivalents: $nallow)"
  rm -rf "$tmp"
  return 0
}

if [ -n "$only" ]; then
  mutation_only_main
  exit $?
fi

gremlins unleash \
  --workers "${GREMLINS_WORKERS:-4}" \
  --timeout-coefficient "${MUTATION_TIMEOUT_COEFFICIENT:-5}" \
  ${passthru[@]+"${passthru[@]}"} "$pkg" 2>&1 | tee "$out"
status=${PIPESTATUS[0]}
echo "mutation output: $out"

if [ "$status" -ne 0 ] && ! grep -q "Mutation testing completed" "$out"; then
  echo "mutation: gremlins failed to run (exit $status)" >&2
  exit 1
fi

lived="$(grep -cE '^[[:space:]]*LIVED' "$out" || true)"
if [ "${lived:-0}" -gt 0 ]; then
  echo "mutation: $lived mutant(s) survived in $pkg" >&2
  exit 1
fi
echo "mutation: no surviving mutants in $pkg"
exit 0
