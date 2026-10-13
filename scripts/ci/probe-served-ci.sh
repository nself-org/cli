#!/usr/bin/env bash
# Purpose: prove the served plugins catalogue carries a usable, signed `ci`
#          release before a CLI patch stops resolving nself-ci any other way
#          (P7-CANON-24 runs this live; P7-CANON-23 ships it with --self-test).
#          A 1.4.x patch must never replace a working bootstrap with a failing install.
# Usage:   probe-served-ci.sh --min-version <v> [--registry-url <url>] [--skip-run] [--ci-args "<args>"]
#          probe-served-ci.sh --self-test
# Inputs:  --min-version  required for the live probe: the oldest acceptable ci version.
#          --registry-url catalogue base (default https://plugins.nself.org); the probe
#                         reads <url>/registry.json.
#          --skip-run     stop after the catalogue and tarball checks (no host/container run).
#          --ci-args      arguments for the fixture run (default "--check --no-status --no-gitleaks").
# Outputs: one line per check on stdout; failures on stderr. Exit 0 only when every
#          check passed; 1 on any failed check; 2 on a usage error or a missing tool.
# Checks:  (1) the catalogue has exactly one free `ci` entry: version >= --min-version,
#          non-empty release_signature (key_id, alg, sig), 64-hex checksum, a repository,
#          pluginType cli, binaryName nself-ci;
#          (2) the four platform tarballs (linux/darwin x amd64/arm64) download, hold
#          an `nself-ci` file and no absolute or `..` entry;
#          (3) unless --skip-run: nself-ci from the host tarball (throwaway HOME) and from
#          the linux tarball (in a container) give the same exit code and output on the
#          base and the head commit of scripts/ci/testdata/ci-bootstrap/repo.
#          --self-test runs (1) and (2) against scripts/ci/testdata/ci-bootstrap/catalogues/
#          {good,no-signature,no-binary,low-version} served by python3 -m http.server and
#          exits 0 only if `good` passes and each bad one fails for its own reason.
#          It touches no network beyond 127.0.0.1 and never runs (3).
# Constraints: bash 3.2 compatible; needs jq, curl, tar (docker and git for check 3,
#          python3 for --self-test); scratch goes to $TMPDIR and is removed on exit.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
FIX="${ROOT}/scripts/ci/testdata/ci-bootstrap"
PLATFORMS="linux-amd64 linux-arm64 darwin-amd64 darwin-arm64"
BIN="nself-ci"

REGISTRY_URL="https://plugins.nself.org"
MIN_VERSION=""
SELF_TEST=0
SKIP_RUN=0
CI_ARGS="--check --no-status --no-gitleaks"
WORK=""
SERVER_PID=""

die() { echo "probe-served-ci: $*" >&2; exit 2; }
cleanup() {
  if [ -n "${SERVER_PID}" ]; then kill "${SERVER_PID}" 2>/dev/null || true; fi
  if [ -n "${WORK}" ]; then rm -rf "${WORK}"; fi
}
trap cleanup EXIT

need() { command -v "$1" >/dev/null 2>&1 || die "needs $1 on PATH"; }

# ver_ge A B: true when dotted version A >= B (numeric parts only; a -pre or +build suffix is dropped).
ver_ge() {
  local a="${1#v}" b="${2#v}" i x y
  a="${a%%[-+]*}"; b="${b%%[-+]*}"
  local IFS=.
  # shellcheck disable=SC2086 # word splitting on IFS=. is the point
  set -- $a; local -a pa=("$@")
  # shellcheck disable=SC2086
  set -- $b; local -a pb=("$@")
  for i in 0 1 2; do
    x="${pa[$i]:-0}"; y="${pb[$i]:-0}"
    case "${x}${y}" in *[!0-9]*) return 1 ;; esac
    if [ "$x" -gt "$y" ]; then return 0; fi
    if [ "$x" -lt "$y" ]; then return 1; fi
  done
  return 0
}

fail() { echo "FAIL: $*" >&2; }

# check_catalogue BASE MIN: sets CI_VERSION and CI_REPO; returns 1 on any failed assertion.
check_catalogue() {
  local base="$1" min="$2" file entry n bad=0 v
  file="${WORK}/registry.json"
  if ! curl -fsS --max-time 60 -o "${file}" "${base}/registry.json"; then
    fail "cannot fetch ${base}/registry.json"; return 1
  fi
  if ! jq -e '(.plugins // .) | type == "array"' "${file}" >/dev/null 2>&1; then
    fail "${base}/registry.json is not a plugin list"; return 1
  fi
  n="$(jq '[(.plugins // .)[] | select(.name == "ci" and ((.tier // "free") == "free"))] | length' "${file}")"
  if [ "${n}" != "1" ]; then fail "want exactly one free ci entry, found ${n}"; return 1; fi
  entry="$(jq -c '[(.plugins // .)[] | select(.name == "ci" and ((.tier // "free") == "free"))][0]' "${file}")"
  CI_VERSION="$(printf '%s' "${entry}" | jq -r '.version // ""')"
  CI_REPO="$(printf '%s' "${entry}" | jq -r '.repository // ""')"
  CI_REPO="${CI_REPO%.git}"
  v="${CI_VERSION}"
  if [ -z "${v}" ] || ! ver_ge "${v}" "${min}"; then
    fail "ci version '${v}' is below --min-version ${min}"; bad=1
  fi
  if ! printf '%s' "${entry}" | jq -e '.release_signature | type == "object" and ((.key_id // "") != "") and ((.alg // "") != "") and ((.sig // "") != "")' >/dev/null 2>&1; then
    fail "the ci entry has no complete release_signature (key_id, alg, sig)"; bad=1
  fi
  if ! printf '%s' "${entry}" | jq -e '(.checksum // "") | test("^[0-9a-f]{64}$")' >/dev/null 2>&1; then
    fail "the ci entry has no 64-hex checksum"; bad=1
  fi
  if ! printf '%s' "${entry}" | jq -e '.pluginType == "cli" and .binaryName == "nself-ci"' >/dev/null 2>&1; then
    fail "the ci entry is not pluginType cli with binaryName nself-ci"; bad=1
  fi
  if [ -z "${CI_REPO}" ]; then fail "the ci entry has no repository"; bad=1; fi
  if [ "${bad}" -ne 0 ]; then return 1; fi
  echo "ok   catalogue: ci ${CI_VERSION} is free, signed, checksummed (>= ${min})"
}

# check_tarballs: downloads the four platform tarballs of CI_VERSION into WORK/tar-<platform>.tar.gz.
check_tarballs() {
  local p url f entries bad=0
  for p in ${PLATFORMS}; do
    url="${CI_REPO}/releases/download/v${CI_VERSION}/ci-${CI_VERSION}-${p}.tar.gz"
    f="${WORK}/tar-${p}.tar.gz"
    if ! curl -fsSL --max-time 300 -o "${f}" "${url}"; then
      fail "cannot download the ${p} tarball (${url})"; bad=1; continue
    fi
    if ! entries="$(tar -tzf "${f}" 2>/dev/null)"; then
      fail "the ${p} tarball is not a readable tar.gz"; bad=1; continue
    fi
    if ! printf '%s\n' "${entries}" | grep -Eq "(^|/)${BIN}\$"; then
      fail "the ${p} tarball holds no ${BIN} file"; bad=1; continue
    fi
    if printf '%s\n' "${entries}" | grep -Eq '(^/|(^|/)\.\.(/|$))'; then
      fail "the ${p} tarball has an absolute or .. entry"; bad=1; continue
    fi
    echo "ok   tarball ${p}: holds ${BIN}"
  done
  if [ "${bad}" -ne 0 ]; then return 1; fi
}

host_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) arch=unknown ;; esac
  echo "${os}-${arch}"
}

# unpack PLATFORM DIR: extracts the tarball and prints the path of its nself-ci file.
unpack() {
  local p="$1" dir="$2" found
  mkdir -p "${dir}"
  tar -xzf "${WORK}/tar-${p}.tar.gz" -C "${dir}"
  found="$(find "${dir}" -type f -name "${BIN}" | head -n 1)"
  if [ -z "${found}" ]; then return 1; fi
  chmod +x "${found}"
  echo "${found}"
}

# normalize blanks the throwaway paths so a host run and a container run compare.
normalize() { sed -e "s#${WORK}#<work>#g" -e 's#/work#<work>#g'; }

check_run() {
  local host_p linux_p hostbin linuxbin repo ref d rc out_h out_c bad=0
  need git; need docker
  host_p="$(host_platform)"
  linux_p="linux-$(host_platform | sed 's/^[a-z]*-//')"
  case " ${PLATFORMS} " in *" ${host_p} "*) ;; *) fail "no published tarball for this host (${host_p})"; return 1 ;; esac
  hostbin="$(unpack "${host_p}" "${WORK}/host-bin")" || { fail "no ${BIN} in the ${host_p} tarball"; return 1; }
  linuxbin="$(unpack "${linux_p}" "${WORK}/linux-bin")" || { fail "no ${BIN} in the ${linux_p} tarball"; return 1; }
  # Pull first and quietly: pull progress must not land in the compared output.
  if ! docker image inspect "${PROBE_IMAGE:-debian:stable-slim}" >/dev/null 2>&1; then
    docker pull -q "${PROBE_IMAGE:-debian:stable-slim}" >/dev/null || { fail "cannot pull the container image"; return 1; }
  fi
  repo="${WORK}/fixture-repo"
  mkdir -p "${repo}"
  git -C "${repo}" init -q
  git -C "${repo}" config user.email probe@example.invalid
  git -C "${repo}" config user.name probe
  cp -R "${FIX}/repo/base/." "${repo}/"
  git -C "${repo}" add -A && git -C "${repo}" commit -q -m base
  cp -R "${FIX}/repo/head/." "${repo}/"
  git -C "${repo}" add -A && git -C "${repo}" commit -q -m head
  for ref in base head; do
    d="${WORK}/checkout-${ref}"
    git clone -q "${repo}" "${d}"
    if [ "${ref}" = base ]; then git -C "${d}" checkout -q HEAD~1; fi
    mkdir -p "${WORK}/home"
    rc=0
    # shellcheck disable=SC2086
    out_h="$(cd "${d}" && HOME="${WORK}/home" "${hostbin}" ${CI_ARGS} 2>&1)" || rc=$?
    local rc_h="${rc}"
    rc=0
    # shellcheck disable=SC2086
    out_c="$(docker run --rm -v "$(dirname "${linuxbin}"):/probe:ro" -v "${d}:/work" -w /work -e HOME=/tmp \
      "${PROBE_IMAGE:-debian:stable-slim}" "/probe/${BIN}" ${CI_ARGS} 2>&1)" || rc=$?
    if [ "${rc_h}" != "${rc}" ]; then
      fail "${ref}: host exit ${rc_h} differs from container exit ${rc}"; bad=1; continue
    fi
    if [ "$(printf '%s' "${out_h}" | normalize)" != "$(printf '%s' "${out_c}" | normalize)" ]; then
      fail "${ref}: host and container output differ"; bad=1; continue
    fi
    echo "ok   run ${ref}: host and container agree (exit ${rc})"
  done
  if [ "${bad}" -ne 0 ]; then return 1; fi
}

probe() { # BASE MIN
  check_catalogue "$1" "$2" || return 1
  check_tarballs || return 1
  if [ "${SKIP_RUN}" -eq 0 ]; then check_run || return 1; fi
}

# serve_case ROOT NAME: builds ROOT/NAME from a fixture catalogue and the right tarball set.
serve_case() {
  local root="$1" name="$2" ver tb p stage
  ver="$(jq -r '.plugins[0].version' "${FIX}/catalogues/${name}/registry.json")"
  tb="with-binary"; if [ "${name}" = "no-binary" ]; then tb="without-binary"; fi
  mkdir -p "${root}/${name}/releases/download/v${ver}"
  sed "s#@BASE@#http://127.0.0.1:${PORT}/${name}#" "${FIX}/catalogues/${name}/registry.json" > "${root}/${name}/registry.json"
  stage="${root}/.stage-${name}"
  mkdir -p "${stage}"
  cp -R "${FIX}/tarballs/${tb}/." "${stage}/"
  for p in ${PLATFORMS}; do
    tar -czf "${root}/${name}/releases/download/v${ver}/ci-${ver}-${p}.tar.gz" -C "${stage}" .
  done
}

self_test() {
  need python3
  local root="${WORK}/serve" name want out i status=0
  mkdir -p "${root}"
  PORT="$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')"
  for name in good no-signature no-binary low-version; do serve_case "${root}" "${name}"; done
  python3 -m http.server "${PORT}" --bind 127.0.0.1 --directory "${root}" >"${WORK}/server.log" 2>&1 &
  SERVER_PID=$!
  for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
    if curl -fsS --max-time 2 -o /dev/null "http://127.0.0.1:${PORT}/good/registry.json" 2>/dev/null; then break; fi
    sleep 0.25
  done
  curl -fsS --max-time 2 -o /dev/null "http://127.0.0.1:${PORT}/good/registry.json" || die "the fixture server did not start"
  SKIP_RUN=1
  for name in good no-signature no-binary low-version; do
    case "${name}" in
      good) want="" ;;
      no-signature) want="release_signature" ;;
      no-binary) want="holds no ${BIN} file" ;;
      low-version) want="below --min-version" ;;
    esac
    status=0
    out="$(probe "http://127.0.0.1:${PORT}/${name}" "1.0.0" 2>&1)" || status=$?
    if [ -z "${want}" ]; then
      if [ "${status}" -ne 0 ]; then echo "SELF-TEST FAIL: ${name} should pass:" >&2; echo "${out}" >&2; return 1; fi
    else
      if [ "${status}" -eq 0 ]; then echo "SELF-TEST FAIL: ${name} should fail the probe but passed" >&2; return 1; fi
      if ! printf '%s\n' "${out}" | grep -Fq "${want}"; then
        echo "SELF-TEST FAIL: ${name} failed, but not for its own reason (want '${want}'):" >&2; echo "${out}" >&2; return 1
      fi
    fi
    echo "self-test ok: ${name} $([ -z "${want}" ] && echo passes || echo "fails as expected")"
  done
  # A live probe without --min-version is a usage error, not a pass.
  if "$0" --registry-url "http://127.0.0.1:${PORT}/good" >/dev/null 2>&1; then
    echo "SELF-TEST FAIL: the probe ran without --min-version" >&2; return 1
  fi
  echo "self-test ok: --min-version is required"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --min-version) [ "$#" -ge 2 ] || die "--min-version needs a value"; MIN_VERSION="$2"; shift 2 ;;
    --registry-url) [ "$#" -ge 2 ] || die "--registry-url needs a value"; REGISTRY_URL="${2%/}"; shift 2 ;;
    --ci-args) [ "$#" -ge 2 ] || die "--ci-args needs a value"; CI_ARGS="$2"; shift 2 ;;
    --skip-run) SKIP_RUN=1; shift ;;
    --self-test) SELF_TEST=1; shift ;;
    -h|--help) sed -n '2,/^set -euo/p' "$0" | sed '$d' | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

need jq; need curl; need tar
WORK="$(mktemp -d "${TMPDIR:-/tmp}/probe-served-ci.XXXXXX")"

if [ "${SELF_TEST}" -eq 1 ]; then
  self_test
  exit $?
fi

[ -n "${MIN_VERSION}" ] || die "--min-version is required for the live probe"
if probe "${REGISTRY_URL}" "${MIN_VERSION}"; then
  echo "probe-served-ci: PASS"
else
  echo "probe-served-ci: FAIL" >&2
  exit 1
fi
