#!/usr/bin/env bash
# Purpose: controls for scripts/ci/wiki-link-audit.sh on a synthetic wiki whose
#          page names sort differently under C and en_US.UTF-8 collation
#          (mixed case, hyphen, underscore). D-1288: comm against a
#          locale-sorted page list reported every such link as broken.
# Inputs:  none.
# Outputs: "ok <case>" per passing case; exit 0 when all pass, exit 1 on the
#          first mismatch.
# Constraints: bash 3.2 compatible; offline. The locale cases need en_US.UTF-8
#          installed; where it is missing they still run and pass trivially.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

# fixture builds a wiki at $1 linking to every page it holds.
fixture() {
  local d="$1"
  mkdir -p "${d}/scripts/ci" "${d}/.github/wiki/sub"
  cp "${HERE}/wiki-link-audit.sh" "${d}/scripts/ci/"
  printf '# a\n' > "${d}/.github/wiki/a-b.md"
  printf '# a\n' > "${d}/.github/wiki/a_b.md"
  printf '# a\n' > "${d}/.github/wiki/AB.md"
  printf '# a\n' > "${d}/.github/wiki/ab-c.md"
  printf '# a\n' > "${d}/.github/wiki/Zed.md"
  printf '# a\n' > "${d}/.github/wiki/sub/bundle-x.md"
  printf '[[a-b]] [[a_b]] [[AB]] [[ab-c]] [[Zed]] [[bundle-x]] [[sub/bundle-x.md]]\n' \
    > "${d}/.github/wiki/Home.md"
}

# expect CASE WANT_EXIT LOCALE DIR [NEEDLE]
expect() {
  local name="$1" want="$2" loc="$3" dir="$4" needle="${5:-}" got=0
  LC_ALL="${loc}" bash "${dir}/scripts/ci/wiki-link-audit.sh" > "${WORK}/${name}.out" 2>&1 || got=$?
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

fixture "${WORK}/good"
expect all-resolve-c 0 C "${WORK}/good"
expect all-resolve-utf8-locale 0 en_US.UTF-8 "${WORK}/good"

fixture "${WORK}/bad"
printf '[[Missing-Page]]\n' >> "${WORK}/bad/.github/wiki/Home.md"
expect broken-link-c 1 C "${WORK}/bad" "[[Missing-Page]]"
expect broken-link-utf8-locale 1 en_US.UTF-8 "${WORK}/bad" "[[Missing-Page]]"

echo "wiki-link-audit_test: all cases passed"
