#!/usr/bin/env bash
# Purpose: authenticate served ci releases before archive inspection or execution.
# Usage: --min-version VERSION [--registry-url URL] [--skip-run] [--ci-args ARGS]
#        --self-test (loopback only; fixture trust; never executes downloaded code).
# Inputs: catalogue and four platform tarballs; compiled-in signature trust root.
# Outputs: check results; exit 0 success, 1 failed check, 2 usage/missing tool.
# Constraints: Bash 3.2; jq/curl/tar/go; python3 for self-test; git/docker for runs.
# Scratch: TMPDIR, removed on exit. Unpinned keys and unsigned platform hashes fail closed.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
FIX="${ROOT}/scripts/ci/testdata/ci-bootstrap"
PLATFORMS="linux-amd64 linux-arm64 darwin-amd64 darwin-arm64"
BIN="nself-ci"
REGISTRY_URL="https://plugins.nself.org"; MIN_VERSION=""; SELF_TEST=0; SKIP_RUN=0
CI_ARGS="--check --no-status --no-gitleaks"; WORK=""; SERVER_PID=""
CURL_PROTO="--proto =https --proto-redir =https"
die() { echo "probe-served-ci: $*" >&2; exit 2; }
cleanup() {
  if [ -n "${SERVER_PID}" ]; then kill "${SERVER_PID}" 2>/dev/null || true; wait "${SERVER_PID}" 2>/dev/null || true; fi
  if [ -n "${WORK}" ]; then rm -rf "${WORK}"; fi
}
trap cleanup EXIT
need() { command -v "$1" >/dev/null 2>&1 || die "needs $1 on PATH"; }
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
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}
# Build the shared verifier with a Go overlay: all generated files stay in WORK.
# Fixture trust is compiled only for --self-test, which never executes a tarball.
build_verifier() {
  need go
  printf 'package main\nconst fixtureTrust = %s\n' "$([ "${SELF_TEST}" -eq 1 ] && echo true || echo false)" > "${WORK}/trust.go"
  cat > "${WORK}/verify.go" <<'GO'
package main
import ("crypto/ed25519"; "crypto/sha256"; "encoding/base64"; "encoding/json"; "fmt"; "os"
 "github.com/nself-org/cli/internal/plugin/verify/signature")
type entry struct {
 Name string `json:"name"`
 Version string `json:"version"`
 Checksum string `json:"checksum"`
 Release *signature.Release `json:"release_signature"`
}
func main() {
 var e entry
 if err := json.NewDecoder(os.Stdin).Decode(&e); err != nil { panic(err) }
 if fixtureTrust {
  seed := sha256.Sum256([]byte("probe-served-ci self-test only"))
  private := ed25519.NewKeyFromSeed(seed[:])
  public := private.Public().(ed25519.PublicKey)
  idHash := sha256.Sum256(public)
  id := fmt.Sprintf("plugins-%x", idHash[:8])
  signature.Keys = []signature.Key{{ID:id, Tier:signature.TierFree, PublicKey:public}}
  if os.Args[1] == "sign" {
   e.Release = &signature.Release{KeyID:id, Alg:signature.Alg,
    Sig:base64.StdEncoding.EncodeToString(ed25519.Sign(private, signature.SigningInput(e.Name, e.Version, e.Checksum)))}
   if err := json.NewEncoder(os.Stdout).Encode(e.Release); err != nil { panic(err) }; return
  }
 }
 sum := e.Checksum
 if len(os.Args) > 2 { sum = os.Args[2] }
 if err := signature.Verify(signature.TierFree, e.Name, e.Version, sum, e.Release); err != nil {
  fmt.Fprintln(os.Stderr, "release_signature:", err); os.Exit(1)
 }
}
GO
  jq -n --arg root "${ROOT}/scripts/ci" --arg work "${WORK}" \
    '{Replace:{($root+"/probe_verify.go"):($work+"/verify.go"), ($root+"/probe_trust.go"):($work+"/trust.go")}}' > "${WORK}/overlay.json"
  gofmt -w "${WORK}/verify.go" "${WORK}/trust.go"
  (cd "${ROOT}" && go build -mod=vendor -overlay "${WORK}/overlay.json" -o "${WORK}/verify" \
    ./scripts/ci/probe_verify.go ./scripts/ci/probe_trust.go)
}
check_catalogue() {
  local base="$1" min="$2" file entry n bad=0 v
  file="${WORK}/registry.json"
  # shellcheck disable=SC2086 # CURL_PROTO is two options on purpose
  if ! curl -fsS ${CURL_PROTO} --max-time 60 -o "${file}" "${base}/registry.json"; then fail "cannot fetch ${base}/registry.json"; return 1; fi
  if ! jq -e '(.plugins // .) | type == "array"' "${file}" >/dev/null 2>&1; then fail "${base}/registry.json is not a plugin list"; return 1; fi
  n="$(jq '[(.plugins // .)[] | select(.name == "ci" and ((.tier // "free") == "free"))] | length' "${file}")"
  if [ "${n}" != "1" ]; then fail "want exactly one free ci entry, found ${n}"; return 1; fi
  entry="$(jq -c '[(.plugins // .)[] | select(.name == "ci" and ((.tier // "free") == "free"))][0]' "${file}")"
  CI_VERSION="$(printf '%s' "${entry}" | jq -r '.version // ""')"
  CI_REPO="$(printf '%s' "${entry}" | jq -r '.repository // ""')"
  CI_REPO="${CI_REPO%.git}"; CI_ENTRY="${entry}"; v="${CI_VERSION}"
  if [ -z "${v}" ] || ! ver_ge "${v}" "${min}"; then fail "ci version '${v}' is below --min-version ${min}"; bad=1; fi
  if ! printf '%s' "${entry}" | jq -e '.release_signature | type == "object" and ((.key_id // "") != "") and ((.alg // "") != "") and ((.sig // "") != "")' >/dev/null 2>&1; then fail "the ci entry has no complete release_signature (key_id, alg, sig)"; bad=1; fi
  if ! printf '%s' "${entry}" | jq -e '(.checksum // "") | test("^[0-9a-f]{64}$")' >/dev/null 2>&1; then fail "the ci entry has no 64-hex checksum"; bad=1; fi
  if ! printf '%s' "${entry}" | jq -e '.pluginType == "cli" and .binaryName == "nself-ci"' >/dev/null 2>&1; then fail "the ci entry is not pluginType cli with binaryName nself-ci"; bad=1; fi
  if [ -z "${CI_REPO}" ]; then fail "the ci entry has no repository"; bad=1; fi
  if ! printf '%s' "${entry}" | jq -e '((.dependencies // []) | [.. | scalars] | length) == 0' >/dev/null 2>&1; then fail "the ci entry lists dependencies, which the seam refuses"; bad=1; fi
  if ! printf '%s' "${entry}" | jq -e '(.requires_license // false) == false and (.isCommercial // false) == false and ((.tier // "free") == "free")' >/dev/null 2>&1; then fail "the ci entry carries a licence marker, which the seam refuses"; bad=1; fi
  if [ "${bad}" -ne 0 ]; then return 1; fi
  if ! printf '%s' "${entry}" | "${WORK}/verify" verify; then fail "the ci release_signature does not verify"; return 1; fi
  echo "ok   catalogue: ci ${CI_VERSION} is free, signed, checksummed (>= ${min})"
}
check_tarballs() {
  local p url f entries want bad=0
  for p in ${PLATFORMS}; do
    url="${CI_REPO}/releases/download/v${CI_VERSION}/ci-${CI_VERSION}-${p}.tar.gz"
    f="${WORK}/tar-${p}.tar.gz"
    # shellcheck disable=SC2086
    if ! curl -fsSL ${CURL_PROTO} --max-time 300 -o "${f}" "${url}"; then fail "cannot download the ${p} tarball (${url})"; bad=1; continue; fi
    want="$(printf '%s' "${CI_ENTRY}" | jq -r --arg p "${p}" '.checksums.platforms[$p] // ""')"
    if ! printf '%s' "${want}" | grep -Eq '^[0-9a-f]{64}$'; then fail "the ci entry has no checksums.platforms entry for ${p}: the tarball cannot be authenticated"; bad=1; continue; fi
    if [ "$(sha256_of "${f}")" != "${want}" ]; then fail "the ${p} tarball does not match its checksums.platforms entry"; bad=1; rm -f "${f}"; continue; fi
    if ! printf '%s' "${CI_ENTRY}" | "${WORK}/verify" verify "${want}"; then fail "the ${p} tarball is not covered by release_signature"; bad=1; continue; fi
    if ! entries="$(tar -tzf "${f}" 2>/dev/null)"; then fail "the ${p} tarball is not a readable tar.gz"; bad=1; continue; fi
    if ! printf '%s\n' "${entries}" | grep -Eq "(^|/)${BIN}\$"; then fail "the ${p} tarball holds no ${BIN} file"; bad=1; continue; fi
    if printf '%s\n' "${entries}" | grep -Eq '(^/|(^|/)\.\.(/|$))'; then fail "the ${p} tarball has an absolute or .. entry"; bad=1; continue; fi
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
unpack() {
  local p="$1" dir="$2" found
  mkdir -p "${dir}"
  tar -xzf "${WORK}/tar-${p}.tar.gz" -C "${dir}"
  found="$(find "${dir}" -type f -name "${BIN}" | head -n 1)"
  if [ -z "${found}" ]; then return 1; fi
  chmod +x "${found}"
  echo "${found}"
}
normalize() { sed -e "s#${WORK}#<work>#g" -e 's#/work#<work>#g'; }
check_run() {
  local host_p linux_p hostbin linuxbin repo ref d rc out_h out_c bad=0
  need git; need docker
  host_p="$(host_platform)"
  linux_p="linux-$(host_platform | sed 's/^[a-z]*-//')"
  case " ${PLATFORMS} " in *" ${host_p} "*) ;; *) fail "no published tarball for this host (${host_p})"; return 1 ;; esac
  hostbin="$(unpack "${host_p}" "${WORK}/host-bin")" || { fail "no ${BIN} in the ${host_p} tarball"; return 1; }
  linuxbin="$(unpack "${linux_p}" "${WORK}/linux-bin")" || { fail "no ${BIN} in the ${linux_p} tarball"; return 1; }
  if ! docker image inspect "${PROBE_IMAGE:-debian:stable-slim}" >/dev/null 2>&1; then docker pull -q "${PROBE_IMAGE:-debian:stable-slim}" >/dev/null || { fail "cannot pull the container image"; return 1; }; fi
  repo="${WORK}/fixture-repo"
  mkdir -p "${repo}"; git -C "${repo}" init -q
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
    out_h="$(cd "${d}" && env -i PATH=/usr/bin:/bin HOME="${WORK}/home" "${hostbin}" ${CI_ARGS} 2>&1)" || rc=$?
    local rc_h="${rc}"
    rc=0
    # shellcheck disable=SC2086
    out_c="$(docker run --rm -v "$(dirname "${linuxbin}"):/probe:ro" -v "${d}:/work" -w /work -e HOME=/tmp \
      "${PROBE_IMAGE:-debian:stable-slim}" "/probe/${BIN}" ${CI_ARGS} 2>&1)" || rc=$?
    if [ "${rc_h}" != "${rc}" ]; then fail "${ref}: host exit ${rc_h} differs from container exit ${rc}"; bad=1; continue; fi
    if [ "$(printf '%s' "${out_h}" | normalize)" != "$(printf '%s' "${out_c}" | normalize)" ]; then fail "${ref}: host and container output differ"; bad=1; continue; fi
    echo "ok   run ${ref}: host and container agree (exit ${rc})"
  done
  if [ "${bad}" -ne 0 ]; then return 1; fi
}
probe() { # BASE MIN
  check_catalogue "$1" "$2" || return 1
  check_tarballs || return 1
  if [ "${SKIP_RUN}" -eq 0 ]; then check_run || return 1; fi
}
serve_case() {
  local root="$1" name="$2" ver tb p f sum sums stage sig
  ver="$(jq -r '.plugins[0].version' "${FIX}/catalogues/${name}/registry.json")"
  tb="with-binary"; if [ "${name}" = "no-binary" ]; then tb="without-binary"; fi
  mkdir -p "${root}/${name}/releases/download/v${ver}"
  sed "s#@BASE@#http://127.0.0.1:${PORT}/${name}#" "${FIX}/catalogues/${name}/registry.json" > "${root}/${name}/registry.json"
  stage="${root}/.stage-${name}"
  mkdir -p "${stage}"
  cp -R "${FIX}/tarballs/${tb}/." "${stage}/"
  sums='{}'
  for p in ${PLATFORMS}; do
    f="${root}/${name}/releases/download/v${ver}/ci-${ver}-${p}.tar.gz"
    if [ "${p}" = linux-amd64 ]; then tar -czf "${f}" -C "${stage}" .
    else cp "${root}/${name}/releases/download/v${ver}/ci-${ver}-linux-amd64.tar.gz" "${f}"; fi
    sum="$(sha256_of "${f}")"
    if [ "${name}" = "tampered" ]; then sum="0000000000000000000000000000000000000000000000000000000000000000"; fi
    sums="$(printf '%s' "${sums}" | jq -c --arg p "${p}" --arg s "${sum}" '. + {($p): $s}')"
  done
  jq --argjson s "${sums}" '.plugins[0].checksums = {sha256: .plugins[0].checksum, platforms: $s}' "${root}/${name}/registry.json" > "${root}/${name}/registry.tmp"
  mv "${root}/${name}/registry.tmp" "${root}/${name}/registry.json"
  if [ "${name}" != "no-signature" ]; then
    sum="$(sha256_of "${f}")"
    jq --arg sum "${sum}" '.plugins[0].checksum = $sum' "${root}/${name}/registry.json" > "${root}/${name}/registry.tmp"
    sig="$(jq -c '.plugins[0]' "${root}/${name}/registry.tmp" | "${WORK}/verify" sign)"
    jq --argjson sig "${sig}" '.plugins[0].release_signature = $sig' "${root}/${name}/registry.tmp" > "${root}/${name}/registry.json"
  fi
  if [ "${name}" = "good" ]; then
    mkdir -p "${root}/forged-signature"
    cp -R "${root}/good/releases" "${root}/forged-signature/"
    jq '.plugins[0].repository |= sub("/good$"; "/forged-signature") |
      .plugins[0].release_signature = {key_id:"made-up", alg:"not-ed25519", sig:"forged"}' \
      "${root}/good/registry.json" > "${root}/forged-signature/registry.json"
    for variant in unknown-alg unknown-key invalid-signature unsigned-platform; do
      mkdir -p "${root}/${variant}"
      jq --arg variant "${variant}" '
        if $variant == "unknown-alg" then .plugins[0].release_signature.alg = "not-ed25519"
        elif $variant == "unknown-key" then .plugins[0].release_signature.key_id = "made-up"
        elif $variant == "invalid-signature" then .plugins[0].release_signature.sig = ("A" * 86 + "==")
        else .plugins[0].checksums.platforms["linux-amd64"] = ("0" * 64) end' \
        "${root}/good/registry.json" > "${root}/${variant}/registry.json"
    done
    mkdir -p "${root}/unsigned-platform/releases/download/v${ver}"
    cp "${stage}/nself-ci" "${root}/unsigned-platform/nself-ci"
    printf '\n# attacker bytes\n' >> "${root}/unsigned-platform/nself-ci"
    f="${root}/unsigned-platform/releases/download/v${ver}/ci-${ver}-linux-amd64.tar.gz"
    tar -czf "${f}" -C "${root}/unsigned-platform" nself-ci
    jq --arg sum "$(sha256_of "${f}")" --arg base "http://127.0.0.1:${PORT}/unsigned-platform" \
      '.plugins[0].repository = $base | .plugins[0].checksums.platforms["linux-amd64"] = $sum' \
      "${root}/unsigned-platform/registry.json" > "${root}/unsigned-platform/registry.tmp"
    mv "${root}/unsigned-platform/registry.tmp" "${root}/unsigned-platform/registry.json"
  fi
}
self_test() {
  need python3
  local root="${WORK}/serve" name want out i status=0
  mkdir -p "${root}"
  PORT="$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')"
  for name in good no-signature no-binary low-version tampered licensed; do serve_case "${root}" "${name}"; done
  python3 -m http.server "${PORT}" --bind 127.0.0.1 --directory "${root}" >"${WORK}/server.log" 2>&1 &
  SERVER_PID=$!
  for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
    if curl -fsS --max-time 2 -o /dev/null "http://127.0.0.1:${PORT}/good/registry.json" 2>/dev/null; then break; fi
    sleep 0.25
  done
  curl -fsS --max-time 2 -o /dev/null "http://127.0.0.1:${PORT}/good/registry.json" || die "the fixture server did not start"
  SKIP_RUN=1
  CURL_PROTO="--proto =http,https --proto-redir =http,https"
  for name in good forged-signature unknown-alg unknown-key invalid-signature unsigned-platform no-signature no-binary low-version tampered licensed; do
    case "${name}" in
      good) want="" ;;
      forged-signature|unknown-alg|unknown-key|invalid-signature|unsigned-platform) want="release_signature" ;;
      no-signature) want="release_signature" ;;
      no-binary) want="holds no ${BIN} file" ;;
      low-version) want="below --min-version" ;;
      tampered) want="does not match its checksums.platforms" ;;
      licensed) want="licence marker" ;;
    esac
    status=0
    out="$(probe "http://127.0.0.1:${PORT}/${name}" "1.0.0" 2>&1)" || status=$?
    if [ -z "${want}" ]; then
      if [ "${status}" -ne 0 ]; then echo "SELF-TEST FAIL: ${name} should pass:" >&2; echo "${out}" >&2; return 1; fi
    else
      if [ "${status}" -eq 0 ]; then echo "SELF-TEST FAIL: ${name} should fail the probe but passed" >&2; return 1; fi
      if ! printf '%s\n' "${out}" | grep -Fq "${want}"; then echo "SELF-TEST FAIL: ${name} failed, but not for its own reason (want '${want}'):" >&2; echo "${out}" >&2; return 1; fi
    fi
    echo "self-test ok: ${name} $([ -z "${want}" ] && echo passes || echo "fails as expected")"
  done
  if "$0" --registry-url "http://127.0.0.1:${PORT}/good" >/dev/null 2>&1; then echo "SELF-TEST FAIL: the probe ran without --min-version" >&2; return 1; fi
  echo "self-test ok: --min-version is required"
  if "$0" --registry-url "http://127.0.0.1:${PORT}/good" --min-version 1.0.0 --skip-run >/dev/null 2>&1; then echo "SELF-TEST FAIL: the live probe accepted an http catalogue" >&2; return 1; fi
  echo "self-test ok: http catalogue refused"
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
build_verifier
if [ "${SELF_TEST}" -eq 1 ]; then self_test; exit $?; fi
[ -n "${MIN_VERSION}" ] || die "--min-version is required for the live probe"
if probe "${REGISTRY_URL}" "${MIN_VERSION}"; then
  echo "probe-served-ci: PASS"
else
  echo "probe-served-ci: FAIL" >&2
  exit 1
fi
