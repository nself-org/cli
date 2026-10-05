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
# Env:     GREMLINS_WORKERS (default 4), MUTATION_TIMEOUT_COEFFICIENT (default 5).
set -uo pipefail

GREMLINS_VERSION="v0.6.0"

if [ $# -lt 1 ]; then
  echo "usage: bash scripts/mutation.sh PKG_DIR" >&2
  exit 2
fi
pkg="$1"
shift

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root" || exit 1

# Module-aware: sdk/go is its own module.
case "$pkg" in
  sdk/go/*) cd sdk/go && pkg="./${pkg#sdk/go/}" ;;
  *) pkg="./${pkg#./}" ;;
esac

out="${MUTATION_OUT:-$(mktemp "${TMPDIR:-/tmp}/mutation.XXXXXX")}"
export CGO_ENABLED=0
export GOFLAGS="${GOFLAGS:--mod=vendor}"

# gremlins itself is not vendored: build the pinned release into a temp dir
# (no global install), outside the vendored module graph.
bindir="$(mktemp -d "${TMPDIR:-/tmp}/gremlins-bin.XXXXXX")"
trap 'rm -rf "$bindir"' EXIT
if ! (cd "$bindir" && GOFLAGS= GOBIN="$bindir" go install "github.com/go-gremlins/gremlins/cmd/gremlins@${GREMLINS_VERSION}"); then
  echo "mutation: could not fetch gremlins ${GREMLINS_VERSION} from the module proxy" >&2
  exit 1
fi
gremlins() { "$bindir/gremlins" "$@"; }

gremlins unleash \
  --workers "${GREMLINS_WORKERS:-4}" \
  --timeout-coefficient "${MUTATION_TIMEOUT_COEFFICIENT:-5}" \
  "$@" "$pkg" 2>&1 | tee "$out"
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
