#!/usr/bin/env bash
# Purpose: positive and negative controls for scripts/ci/vendor-sync-check.sh
#          on a synthetic module shaped like the cli: a main module that
#          replaces a first-party sdk module with ./sdk and vendors it.
# Inputs:  none.
# Outputs: "ok <case>" per passing case; exit 0 when all pass, exit 1 on the
#          first mismatch.
# Constraints: bash 3.2 compatible; offline (GOPROXY=off, no third-party
#          modules); needs only the go toolchain.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

export GOPROXY=off GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=mod
export GOCACHE="${GOCACHE:-$(go env GOCACHE)}"

# fixture builds a fresh, in-sync module at $1.
fixture() {
  local m="$1"
  mkdir -p "${m}/scripts/ci" "${m}/sdk/lib"
  cp "${HERE}/vendor-sync-check.sh" "${m}/scripts/ci/"
  cat > "${m}/go.mod" <<'EOF'
module github.com/nself-org/vsc-fixture

go 1.21

require github.com/nself-org/vsc-fixture/sdk v0.0.0

replace github.com/nself-org/vsc-fixture/sdk => ./sdk
EOF
  cat > "${m}/main.go" <<'EOF'
package main

import "github.com/nself-org/vsc-fixture/sdk/lib"

func main() { lib.Hello() }
EOF
  printf 'module github.com/nself-org/vsc-fixture/sdk\n\ngo 1.21\n' > "${m}/sdk/go.mod"
  printf 'package lib\n\n// Hello is the vendored function.\nfunc Hello() {}\n' > "${m}/sdk/lib/lib.go"
  printf 'package lib\n\nimport "testing"\n\nfunc TestHello(t *testing.T) { Hello() }\n' > "${m}/sdk/lib/lib_test.go"
  (cd "${m}" && go mod vendor)
}

# expect CASE WANT_EXIT DIR [NEEDLE]: run the check in DIR and compare.
expect() {
  local name="$1" want="$2" dir="$3" needle="${4:-}" got=0
  bash "${dir}/scripts/ci/vendor-sync-check.sh" > "${WORK}/${name}.out" 2>&1 || got=$?
  if [ "${got}" -ne "${want}" ]; then
    echo "FAIL ${name}: exit ${got}, want ${want}" >&2
    cat "${WORK}/${name}.out" >&2
    exit 1
  fi
  if [ -n "${needle}" ] && ! grep -qF -- "${needle}" "${WORK}/${name}.out"; then
    echo "FAIL ${name}: output lacks ${needle}" >&2
    cat "${WORK}/${name}.out" >&2
    exit 1
  fi
  echo "ok ${name}"
}

# case_dir NAME: a fresh in-sync fixture per case, so no case sees another's plant.
case_dir() { fixture "${WORK}/$1" >&2; echo "${WORK}/$1"; }

d="$(case_dir in-sync)"
expect in-sync 0 "${d}" "matches go mod vendor"

# The D-0482 hazard: an sdk fix that was never re-vendored.
d="$(case_dir sdk-edit-not-vendored)"
printf '\n// planted drift\n' >> "${d}/sdk/lib/lib.go"
expect sdk-edit-not-vendored 1 "${d}" "+// planted drift"

d="$(case_dir sdk-new-file-not-vendored)"
printf 'package lib\n\nfunc Extra() {}\n' > "${d}/sdk/lib/extra.go"
expect sdk-new-file-not-vendored 1 "${d}" "extra.go"

d="$(case_dir vendored-copy-hand-edited)"
printf '\n// hand edit\n' >> "${d}/vendor/github.com/nself-org/vsc-fixture/sdk/lib/lib.go"
expect vendored-copy-hand-edited 1 "${d}" "lib.go"

d="$(case_dir vendored-file-missing)"
rm "${d}/vendor/github.com/nself-org/vsc-fixture/sdk/lib/lib.go"
expect vendored-file-missing 1 "${d}" "Only in <go mod vendor>"

# Test files are not vendored, so editing one is not drift.
d="$(case_dir sdk-test-edit-is-not-drift)"
printf '\n// test-only change\n' >> "${d}/sdk/lib/lib_test.go"
expect sdk-test-edit-is-not-drift 0 "${d}"

d="$(case_dir go-mod-vendor-fails)"
printf 'package main\n\nimport _ "github.com/nself-org/vsc-fixture/sdk/missing"\n' > "${d}/broken.go"
expect go-mod-vendor-fails 2 "${d}" "go mod vendor failed"

d="$(case_dir no-vendor-dir)"
rm -rf "${d}/vendor"
expect no-vendor-dir 1 "${d}" "no vendor/ directory"

echo "vendor-sync-check_test: all cases passed"
