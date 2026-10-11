#!/usr/bin/env bash
# Purpose: run the sdk/go/remote mutation gate (D-0483) exactly as its
#          allowlist header records it, so CI and the allowlist cannot drift.
#          Line 1 of sdk/go/remote/testdata/mutation-allow.txt holds the
#          command; this script takes the --only set from it, checks that the
#          set names every non-test file the package compiles on linux (a new
#          file must join the gate) and that no file hides behind a custom
#          build tag, then runs scripts/mutation.sh.
# Inputs:  --dry-run: run the checks and print the gate command, do not run it.
# Outputs: exit 0 no mutant lived; 1 a mutant lived, the header is missing or
#          malformed, or the --only set differs from the package's files;
#          2 bad usage. scripts/mutation.sh prints the gremlins report.
# Env:     GREMLINS_WORKERS etc. pass through to scripts/mutation.sh.
# Constraints: bash 3.2 compatible; runs at the module root two levels above
#          this script. Tested by scripts/ci/sdk-remote-mutation_test.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "${ROOT}"

dry=0
case "${1:-}" in
  "") ;;
  --dry-run) dry=1 ;;
  *) echo "usage: bash scripts/ci/sdk-remote-mutation.sh [--dry-run]" >&2; exit 2 ;;
esac

ALLOW="sdk/go/remote/testdata/mutation-allow.txt"
PREFIX="# Equivalent mutants for: bash scripts/mutation.sh sdk/go/remote --only "
SUFFIX=" --allow remote/testdata/mutation-allow.txt"

if [ ! -f "${ALLOW}" ]; then
  echo "sdk-remote-mutation: ${ALLOW} is missing" >&2
  exit 1
fi
header="$(head -n 1 "${ALLOW}")"
case "${header}" in
  "${PREFIX}"*"${SUFFIX}") ;;
  *)
    echo "sdk-remote-mutation: line 1 of ${ALLOW} is not the gate command:" >&2
    echo "  want: ${PREFIX}<a.go,b.go,...>${SUFFIX}" >&2
    echo "  got:  ${header}" >&2
    exit 1
    ;;
esac
only="${header#"${PREFIX}"}"
only="${only%"${SUFFIX}"}"
case "${only}" in
  "" | *" "*)
    echo "sdk-remote-mutation: the --only set in ${ALLOW} is empty or holds a space: '${only}'" >&2
    exit 1
    ;;
esac

# golist GOOS TEMPLATE: go list over the remote package for one GOOS. GOARCH
# is pinned so the answer does not depend on the host (arm64 Mac vs runner).
golist() {
  (cd sdk/go && GOOS="$1" GOARCH=amd64 GOFLAGS=-mod=mod GOWORK=off go list -f "$2" ./remote) || {
    echo "sdk-remote-mutation: go list ./remote (GOOS=$1) failed" >&2
    exit 1
  }
}

# Non-test files the linux gate may skip: lock_other.go is the !unix no-op
# stub the allowlist header names. Any other file go skips on linux (a custom
# build tag, a windows- or darwin-only file) would hide code from every
# mutant, so it fails here until it is mutated or listed with a reason.
IGNORED_OK="lock_other.go"
ignored_all="$(golist linux '{{range .IgnoredGoFiles}}{{.}}{{"\n"}}{{end}}')"
for f in $(printf '%s\n' "${ignored_all}" | grep -v -e '_test\.go$' -e '^$' || true); do
  case " ${IGNORED_OK} " in
    *" ${f} "*) ;;
    *)
      echo "sdk-remote-mutation: ${f} is not compiled on linux (build tag?), so no mutant reaches it" >&2
      exit 1
      ;;
  esac
done

# The gate runs on a linux host, so the set must equal the files go compiles
# there (lock_other.go is !unix and drops out on its own).
pkg_files="$(golist linux '{{join .GoFiles ","}}')"
want="$(printf '%s\n' "${pkg_files}" | tr ',' '\n' | sort)"
got="$(printf '%s\n' "${only}" | tr ',' '\n' | sort)"
if [ "${want}" != "${got}" ]; then
  echo "sdk-remote-mutation: the --only set in ${ALLOW} differs from sdk/go/remote's linux files" >&2
  echo "  (< package file missing from --only, > --only entry the package lacks)" >&2
  diff <(printf '%s\n' "${want}") <(printf '%s\n' "${got}") >&2 || true
  exit 1
fi

cmd=(bash scripts/mutation.sh sdk/go/remote --only "${only}" --allow remote/testdata/mutation-allow.txt)
if [ "${dry}" -eq 1 ]; then
  echo "GOFLAGS=-mod=mod ${cmd[*]}"
  exit 0
fi
GOFLAGS=-mod=mod exec "${cmd[@]}"
