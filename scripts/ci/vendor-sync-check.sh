#!/usr/bin/env bash
# Purpose: fail when the tracked vendor/ tree differs from what `go mod vendor`
#          produces now. The cli builds with -mod=vendor and go.mod replaces
#          github.com/nself-org/cli/sdk/go/v2 with ./sdk/go, so an sdk/go fix
#          that is not re-vendored passes its own tests while the cli binary
#          keeps the stale copy (D-0482; cli#654 nearly merged that way).
# Inputs:  none. Runs at the module root two levels above this script.
# Outputs: exit 0 when vendor/ is in sync; exit 1 listing every drifted path
#          (with a diff for first-party github.com/nself-org paths) or any
#          symlink in a local replace target; exit 2
#          when `go mod vendor` itself fails.
# Constraints: bash 3.2 compatible; writes only to a temp dir, never to the
#          tree. Needs the module cache or the module proxy (third-party code
#          is vendored too). Tested by scripts/ci/vendor-sync-check_test.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "${ROOT}"

if [ ! -d vendor ]; then
  echo "vendor-sync-check: no vendor/ directory in ${ROOT}" >&2
  exit 1
fi

# A developer's go.work must not change what is checked.
export GOWORK=off

# Go builds through a symlinked .go file or directory but `go mod vendor`
# drops it, so a symlink in a local replace target (./sdk/go) would reach
# go test but not the binary. A local target is a ./, ../ or absolute path.
while IFS= read -r dir; do
  links="$(find "${dir}" -type l 2>/dev/null || true)"
  if [ -n "${links}" ]; then
    echo "vendor-sync-check: FAIL: symlinks in ${dir} (go mod vendor drops them):" >&2
    echo "${links}" >&2
    exit 1
  fi
done < <(go mod edit -json | grep -o '"Path": "\.\{0,2\}/[^"]*"' | sed 's/^"Path": "//; s/"$//')

WORK="$(mktemp -d "${TMPDIR:-/tmp}/vendor-sync.XXXXXX")"
trap 'rm -rf "${WORK}"' EXIT

# GOFLAGS=-mod=vendor (set by some CI jobs) makes `go mod vendor` refuse to run.
if ! GOFLAGS= go mod vendor -o "${WORK}/vendor"; then
  echo "vendor-sync-check: go mod vendor failed" >&2
  exit 2
fi

if diff -rq vendor "${WORK}/vendor" > "${WORK}/drift.txt"; then
  echo "vendor-sync-check: vendor/ matches go mod vendor"
  exit 0
fi

echo "vendor-sync-check: FAIL: vendor/ differs from go mod vendor" >&2
sed "s|${WORK}/vendor|<go mod vendor>|g" "${WORK}/drift.txt" >&2
# First-party code (the replaced sdk module) is where drift matters most:
# show what the cli binary would miss.
if grep -q 'nself-org' "${WORK}/drift.txt"; then
  diff -ru vendor/github.com/nself-org "${WORK}/vendor/github.com/nself-org" \
    | sed "s|${WORK}/vendor|<go mod vendor>|g" | head -200 >&2 || true
fi
echo "vendor-sync-check: run 'go mod vendor' and commit vendor/ with the change" >&2
exit 1
