#!/usr/bin/env bash
# Purpose: test scripts/ci/docker-publish-promote.sh with stub `gh` and `docker`.
#          Proves :latest moves only while the promoted tag is still GitHub's
#          latest release at promotion time (D-0204 ordering race).
# Inputs:  none.
# Outputs: "ok <case>" per passing case; exit 0 when all pass, exit 1 on mismatch.
# Constraints: bash 3.2 compatible; no network, no registry credential.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="${HERE}/docker-publish-promote.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
mkdir -p "${WORK}/bin"

cat > "${WORK}/bin/gh" <<'STUB'
#!/usr/bin/env bash
[ -n "${STUB_GH_FAIL:-}" ] && exit 1
printf '%s' "${STUB_LATEST_TAG}"
STUB
cat > "${WORK}/bin/docker" <<'STUB'
#!/usr/bin/env bash
[ -z "${STUB_DOCKER_FAIL:-}" ] || exit 1
echo "$*" >> "${STUB_DOCKER_LOG}"
STUB
chmod +x "${WORK}/bin/gh" "${WORK}/bin/docker"

# run_case NAME WANT_RC WANT_DOCKER_LOG("-" = no call) VAR=VALUE...
run_case() {
  local name="$1" want_rc="$2" want_log="$3" rc=0
  shift 3
  : > "${WORK}/docker.log"
  env -i PATH="${WORK}/bin:/usr/bin:/bin" HOME="${WORK}" REPO="nself-org/cli" \
    STUB_DOCKER_LOG="${WORK}/docker.log" "$@" \
    bash "${SCRIPT}" > "${WORK}/stdout.txt" 2> "${WORK}/stderr.txt" || rc=$?
  [ "${rc}" = "${want_rc}" ] || { echo "FAIL ${name}: exit ${rc}, want ${want_rc}" >&2; cat "${WORK}/stderr.txt" >&2; exit 1; }
  if [ "${want_log}" = "-" ]; then
    [ ! -s "${WORK}/docker.log" ] || { echo "FAIL ${name}: docker ran: $(cat "${WORK}/docker.log")" >&2; exit 1; }
  else
    grep -qx "${want_log}" "${WORK}/docker.log" || { echo "FAIL ${name}: docker log lacks '${want_log}'" >&2; cat "${WORK}/docker.log" >&2; exit 1; }
  fi
  echo "ok ${name}"
}

run_case "current latest is promoted" 0 "buildx imagetools create -t nself/nself:latest nself/nself:1.4.13" \
  TAG=v1.4.13 STUB_LATEST_TAG=v1.4.13
# The race: v1.4.12's run finishes after v1.4.13 became latest.
run_case "stale tag is not promoted over a newer latest" 0 - \
  TAG=v1.4.12 STUB_LATEST_TAG=v1.4.13
grep -q '::notice::' "${WORK}/stdout.txt" || { echo "FAIL: stale skip printed no ::notice::" >&2; exit 1; }
echo "ok stale skip is announced"
run_case "unreadable latest release -> exit 1, no promotion" 1 - \
  TAG=v1.4.13 STUB_LATEST_TAG=v1.4.13 STUB_GH_FAIL=1
run_case "imagetools failure -> exit 1" 1 - \
  TAG=v1.4.13 STUB_LATEST_TAG=v1.4.13 STUB_DOCKER_FAIL=1
run_case "TAG unset -> exit 1" 1 - STUB_LATEST_TAG=v1.4.13

echo "docker-publish-promote_test: all cases passed"
