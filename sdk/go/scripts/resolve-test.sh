#!/usr/bin/env bash
# resolve-test.sh - prove, with no network, that the Go SDK tag form resolves.
#
# The module path is github.com/nself-org/cli/sdk/go/v2, so Go resolves only
# tags `sdk/go/v2.M.P`. This script clones the current checkout into a temp
# dir, redirects https://github.com/nself-org/cli to that clone with a private
# git config (url.<file>.insteadOf), and checks two cases with GOPROXY=direct:
#   1. tag `sdk/go/v2.99.0`  -> a scratch module can `go get ...@v2.99.0` and
#      `go build` a file importing .../sdk/go/v2/plugin.
#   2. only `sdk/go/v2/v1.99.0` (the old, broken form) -> `go get ...@v1.99.0`
#      and `go get ...@v2.99.0` both fail.
# Exits non-zero if either expectation breaks. Creates no tag outside the temp
# clones; never pushes. HTTP(S)_PROXY point at a closed local port so any real
# network use fails instead of silently succeeding.
#
# Dependencies of the SDK (chi, pgx, ...) are served from a copy of the local
# module cache's download area, so nothing is fetched. Set NSELF_SDK_RESOLVE_SEED
# to another cache/download directory if GOMODCACHE is not the seed you want.
set -euo pipefail

MOD="github.com/nself-org/cli/sdk/go/v2"
ROOT="$(git rev-parse --show-toplevel)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/sdk-resolve.XXXXXX")"
trap 'chmod -R u+w "$WORK" 2>/dev/null || true; rm -rf "$WORK"' EXIT

SEED="${NSELF_SDK_RESOLVE_SEED:-$(go env GOMODCACHE)/cache/download}"

# make_clone <name> <tag>: clone HEAD, drop inherited sdk/* tags, add one tag.
make_clone() {
  git clone --quiet "$ROOT" "$WORK/$1"
  git -C "$WORK/$1" tag -l 'sdk/*' | while read -r t; do git -C "$WORK/$1" tag -d "$t" >/dev/null; done
  git -C "$WORK/$1" tag "$2"
}

# scratch <name> <clone-name>: write a scratch module and an isolated env file.
scratch() {
  local dir="$WORK/$1"
  mkdir -p "$dir/mod/cache" "$dir/cache"
  [ -d "$SEED" ] && cp -R "$SEED" "$dir/mod/cache/download"
  printf '[url "file://%s"]\n\tinsteadOf = https://github.com/nself-org/cli\n' "$WORK/$2" >"$dir/gitconfig"
  mkdir -p "$dir/src"
  printf 'module example.com/scratch\n\ngo 1.25.0\n' >"$dir/src/go.mod"
  printf 'package main\n\nimport "%s/plugin"\n\nfunc main() { _ = plugin.Info{} }\n' "$MOD" >"$dir/src/main.go"
}

# run_go <name> <args...>: go in the scratch module with the isolated env.
run_go() {
  local dir="$WORK/$1"
  shift
  (cd "$dir/src" && env \
    GIT_CONFIG_GLOBAL="$dir/gitconfig" GIT_CONFIG_NOSYSTEM=1 GIT_TERMINAL_PROMPT=0 \
    GOPROXY=direct GOPRIVATE='github.com/nself-org/*' GONOSUMDB='github.com/nself-org/*' \
    GOSUMDB=off GOFLAGS=-mod=mod GOTOOLCHAIN=local GOWORK=off \
    GOMODCACHE="$dir/mod" GOCACHE="$dir/cache" \
    HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 ALL_PROXY=http://127.0.0.1:9 \
    go "$@")
}

fail=0

echo "== case 1: new tag form sdk/go/v2.99.0 must resolve and build"
make_clone good sdk/go/v2.99.0
scratch s1 good
if run_go s1 get "$MOD@v2.99.0" && run_go s1 build ./...; then
  echo "PASS: $MOD@v2.99.0 resolved and built"
else
  echo "FAIL: $MOD@v2.99.0 did not resolve or build" >&2
  fail=1
fi

echo "== case 2: old tag form sdk/go/v2/v1.99.0 must NOT resolve"
make_clone old sdk/go/v2/v1.99.0
scratch s2 old
if run_go s2 get "$MOD@v1.99.0" >/dev/null 2>&1; then
  echo "FAIL: old-form tag resolved as $MOD@v1.99.0" >&2
  fail=1
elif run_go s2 get "$MOD@v2.99.0" >/dev/null 2>&1; then
  echo "FAIL: $MOD@v2.99.0 resolved without a sdk/go/v2.99.0 tag" >&2
  fail=1
else
  echo "PASS: old-form tag does not resolve"
fi

if [ "$fail" -ne 0 ]; then
  echo "resolve-test: FAILED" >&2
  exit 1
fi
echo "resolve-test: OK"
