#!/usr/bin/env bash
# Purpose: positive and negative controls for scripts/ci/panic-ratchet.sh on a
#          synthetic tree.
# Inputs:  none.
# Outputs: "ok <case>" per passing case; exit 0 when all pass, exit 1 on the
#          first mismatch.
# Constraints: bash 3.2 compatible; offline; no go toolchain needed.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
T="${WORK}/tree"
mkdir -p "${T}/internal/a" "${T}/vendor/x" "${T}/internal/a/testdata" "${T}/cmd"
printf 'package a\n\nfunc F() { panic("boom") }\n' > "${T}/internal/a/a.go"
printf 'package a\n\n// panic("in a comment") is ignored\nfunc G() error { return nil }\n' > "${T}/internal/a/b.go"
printf 'package a\n\nfunc H() { panic("in a test") }\n' > "${T}/internal/a/a_test.go"
printf 'package x\n\nfunc V() { panic("vendored") }\n' > "${T}/vendor/x/x.go"
printf 'package a\n\nfunc T() { panic("fixture") }\n' > "${T}/internal/a/testdata/t.go"
printf 'package main\n\nfunc main() { panic("main may panic") }\n' > "${T}/cmd/main.go"
printf '1 internal/a/a.go\n' > "${WORK}/base.txt"

# expect CASE WANT_EXIT [NEEDLE]: run the ratchet on the synthetic tree.
expect() {
  local name="$1" want="$2" needle="${3:-}" got=0
  PANIC_BASELINE="${WORK}/base.txt" bash "${HERE}/panic-ratchet.sh" "${T}" > "${WORK}/${name}.out" 2>&1 || got=$?
  if [ "${got}" -ne "${want}" ]; then
    echo "FAIL ${name}: exit ${got}, want ${want}" >&2; cat "${WORK}/${name}.out" >&2; exit 1
  fi
  if [ -n "${needle}" ] && ! grep -q -- "${needle}" "${WORK}/${name}.out"; then
    echo "FAIL ${name}: output lacks '${needle}'" >&2; cat "${WORK}/${name}.out" >&2; exit 1
  fi
  echo "ok ${name}"
}

# Comments, tests, vendor, testdata and package main do not count.
expect within-baseline 0 "ok (1 site(s))"

# A second site in a baselined file fails.
printf 'package a\n\nfunc F2() { panic("two") }\n' >> "${T}/internal/a/a.go"
expect file-over-baseline 1 "internal/a/a.go has 2"

# A new file with a site fails.
printf 'package a\n\nfunc F() { panic("boom") }\n' > "${T}/internal/a/a.go"
printf 'package a\n\nfunc N() { panic("new") }\n' > "${T}/internal/a/new.go"
expect new-file 1 "internal/a/new.go has 1"

# Removing a site only notes that the baseline can drop.
rm "${T}/internal/a/new.go"
printf 'package a\n\nfunc F() error { return nil }\n' > "${T}/internal/a/a.go"
expect below-baseline 0 "has no raw panic() left"

# A missing baseline is a usage error.
got=0; PANIC_BASELINE="${WORK}/missing.txt" bash "${HERE}/panic-ratchet.sh" "${T}" >/dev/null 2>&1 || got=$?
[ "${got}" -eq 2 ] || { echo "FAIL missing-baseline: exit ${got}, want 2" >&2; exit 1; }
echo "ok missing-baseline"
