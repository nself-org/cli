#!/usr/bin/env bash
# Purpose: name the Go packages a change can affect, so a Ticket verifies with
#          `go test -mod=vendor $(bash scripts/ci/affected-pkgs.sh origin/main)`
#          instead of the whole suite. The full suite still runs once per
#          merge-train batch and in CI.
# Usage:   affected-pkgs.sh <base-ref>
# Inputs:  <base-ref>: any git ref. The change set is every file that differs
#          between merge-base(<base-ref>, HEAD) and HEAD, plus uncommitted and
#          untracked files (so it also works before the commit).
# Outputs: stdout, one import path per line, sorted: every module package with a
#          changed .go file plus every module package that imports one of them,
#          directly or through other module packages (reverse walk over
#          `go list -mod=vendor -deps`, test imports included). Prints `./...`
#          alone when go.mod, go.sum, vendor/ or anything under internal/errs/
#          changed (everything depends on those). Prints nothing when no Go
#          file changed: callers must not pass an empty list to `go test`.
#          Exit 0 on success, 2 on a usage error or an unknown ref.
# Constraints: bash 3.2 compatible; read-only; needs git and go. A package that
#          was deleted has no import graph entry and is skipped; its importers
#          changed too, so they are listed.
set -euo pipefail

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
  echo "usage: affected-pkgs.sh <base-ref>" >&2
  exit 2
fi
BASE="$1"
cd "$(git rev-parse --show-toplevel)"
if ! git rev-parse --verify --quiet "$BASE^{commit}" >/dev/null; then
  echo "affected-pkgs: unknown ref '$BASE'" >&2
  exit 2
fi

CHANGED="$(
  {
    git diff --name-only "$BASE"...HEAD
    git ls-files --modified --others --exclude-standard
  } | sort -u
)"

# Changes everything depends on: rebuild the world.
if printf '%s\n' "$CHANGED" | grep -qE '^(go\.mod|go\.sum|vendor/|internal/errs/)'; then
  echo "./..."
  exit 0
fi

GOFILES="$(printf '%s\n' "$CHANGED" | grep -E '\.go$' || true)"
[ -n "$GOFILES" ] || exit 0

MODDIR="$(CGO_ENABLED=0 go list -mod=vendor -m -f '{{.Dir}}')"
MODPATH="$(CGO_ENABLED=0 go list -mod=vendor -m -f '{{.Path}}')"

# One line per package: <import path>|<dir>|<every import, test imports too>.
GRAPH="$(CGO_ENABLED=0 go list -mod=vendor -deps -test=false \
  -f '{{.ImportPath}}|{{.Dir}}|{{join .Imports " "}} {{join .TestImports " "}} {{join .XTestImports " "}}' ./...)"

# Directories (relative to the module root) of the changed files.
DIRS="$(printf '%s\n' "$GOFILES" | while IFS= read -r f; do
  d="$(dirname "$f")"
  [ "$d" = "." ] && printf '.\n' || printf '%s\n' "$d"
done | sort -u)"

# awk: seed with packages in a changed dir, then close over reverse imports.
printf '%s\n' "$GRAPH" | awk -F'|' -v modpath="$MODPATH" -v moddir="$MODDIR" -v dirs="$DIRS" '
  BEGIN {
    n = split(dirs, arr, "\n")
    for (i = 1; i <= n; i++) changed[arr[i]] = 1
  }
  {
    pkg = $1; dir = $2; imps = $3
    if (index(pkg, modpath) != 1) next
    rel = substr(dir, length(moddir) + 2)
    if (rel == "") rel = "."
    pkgs[pkg] = 1
    if (rel in changed) hit[pkg] = 1
    m = split(imps, list, " ")
    for (j = 1; j <= m; j++) {
      if (list[j] != "" && index(list[j], modpath) == 1) {
        importers[list[j]] = importers[list[j]] " " pkg
      }
    }
  }
  END {
    qn = 0
    for (p in hit) queue[++qn] = p
    for (qi = 1; qi <= qn; qi++) {
      k = split(importers[queue[qi]], who, " ")
      for (j = 1; j <= k; j++) {
        if (!(who[j] in hit)) { hit[who[j]] = 1; queue[++qn] = who[j] }
      }
    }
    for (p in hit) if (p in pkgs) print p
  }
' | sort
