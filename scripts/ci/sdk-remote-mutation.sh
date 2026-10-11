#!/usr/bin/env bash
# Purpose: run the sdk/go/remote mutation gate (D-0483) exactly as its
#          allowlist header records it, so CI and the allowlist cannot drift.
#          Line 1 of sdk/go/remote/testdata/mutation-allow.txt holds the
#          command; this script takes the --only set from it, checks that the
#          set names every non-test file the package compiles on linux (a new
#          file must join the gate), then runs scripts/mutation.sh.
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

# The gate runs on a linux host, so the set must equal the files go compiles
# there (lock_other.go is !unix and drops out on its own).
pkg_files="$(cd sdk/go && GOOS=linux GOFLAGS=-mod=mod go list -f '{{join .GoFiles ","}}' ./remote)"
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
