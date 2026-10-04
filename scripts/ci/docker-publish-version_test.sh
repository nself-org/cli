#!/usr/bin/env bash
# Purpose: test scripts/ci/docker-publish-version.sh against a stub `gh` that
#          answers from fixture JSON. No network, no registry credential.
# Inputs:  none.
# Outputs: "ok <case>" per passing case; exit 0 when all pass, exit 1 on the
#          first mismatch.
# Constraints: bash 3.2 compatible; needs jq (the stub uses it to honour -q).
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="${HERE}/docker-publish-version.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

mkdir -p "${WORK}/bin"

# Fixture: every release the stub knows. LATEST_TAG is what a tag-less
# `gh release view` returns (GitHub's latest, non-prerelease release).
cat > "${WORK}/releases.json" <<'JSON'
[
  {"tagName": "v1.4.13", "isPrerelease": false, "isDraft": false},
  {"tagName": "v1.4.12", "isPrerelease": false, "isDraft": false},
  {"tagName": "v1.5.0-rc.1", "isPrerelease": true, "isDraft": false},
  {"tagName": "v1.4.14", "isPrerelease": false, "isDraft": true}
]
JSON

cat > "${WORK}/bin/gh" <<'STUB'
#!/usr/bin/env bash
# Stub for: gh release view [TAG] -R REPO --json FIELDS -q EXPR
set -euo pipefail
[ "$1" = "release" ] && [ "$2" = "view" ] || { echo "stub gh: unsupported: $*" >&2; exit 2; }
shift 2
TAG=""
EXPR=""
while [ $# -gt 0 ]; do
  case "$1" in
    -R|--json) shift 2 ;;
    -q) EXPR="$2"; shift 2 ;;
    *) TAG="$1"; shift ;;
  esac
done
[ -n "${TAG}" ] || TAG="${STUB_LATEST_TAG}"
OBJ=$(jq -c --arg t "${TAG}" '.[] | select(.tagName == $t)' "${STUB_RELEASES}")
[ -n "${OBJ}" ] || { echo "release not found" >&2; exit 1; }
printf '%s' "${OBJ}" | jq -r "${EXPR}"
STUB
chmod +x "${WORK}/bin/gh"

export STUB_RELEASES="${WORK}/releases.json"
export STUB_LATEST_TAG="v1.4.13"

# run_case NAME EXPECT_EXIT EXPECT_OUTPUT_LINES(space-separated k=v or "-") VAR=VALUE...
run_case() {
  local name="$1" want_rc="$2" want_out="$3"
  shift 3
  local out="${WORK}/out.txt" rc=0 kv
  : > "${out}"
  env -i PATH="${WORK}/bin:/usr/bin:/bin:/opt/homebrew/bin:/usr/local/bin" HOME="${WORK}" \
    STUB_RELEASES="${STUB_RELEASES}" STUB_LATEST_TAG="${STUB_LATEST_TAG}" \
    REPO="nself-org/cli" GITHUB_OUTPUT="${out}" "$@" \
    bash "${SCRIPT}" > "${WORK}/stdout.txt" 2> "${WORK}/stderr.txt" || rc=$?
  if [ "${rc}" != "${want_rc}" ]; then
    echo "FAIL ${name}: exit ${rc}, want ${want_rc}" >&2
    cat "${WORK}/stderr.txt" >&2
    exit 1
  fi
  if [ "${want_rc}" = "0" ]; then
    for kv in ${want_out}; do
      grep -qx "${kv}" "${out}" || { echo "FAIL ${name}: output lacks ${kv}" >&2; cat "${out}" >&2; exit 1; }
    done
  else
    [ ! -s "${out}" ] || { echo "FAIL ${name}: wrote outputs on failure" >&2; exit 1; }
    grep -q '::error::' "${WORK}/stderr.txt" || { echo "FAIL ${name}: no ::error:: line" >&2; exit 1; }
  fi
  echo "ok ${name}"
}

# --- workflow_run ---
run_case "run: latest non-prerelease" 0 "version=1.4.13 tag=v1.4.13 latest=true proceed=true" \
  EVENT_NAME=workflow_run HAVE_SECRETS=1 RUN_CONCLUSION=success RUN_HEAD_BRANCH=v1.4.13
run_case "run: older non-prerelease is not latest" 0 "version=1.4.12 latest=false proceed=true" \
  EVENT_NAME=workflow_run HAVE_SECRETS=1 RUN_CONCLUSION=success RUN_HEAD_BRANCH=v1.4.12
run_case "run: prerelease is not latest" 0 "version=1.5.0-rc.1 latest=false proceed=true" \
  EVENT_NAME=workflow_run HAVE_SECRETS=1 RUN_CONCLUSION=success RUN_HEAD_BRANCH=v1.5.0-rc.1
run_case "run: head_branch is not a tag -> exit 1" 1 - \
  EVENT_NAME=workflow_run HAVE_SECRETS=1 RUN_CONCLUSION=success RUN_HEAD_BRANCH=main
run_case "run: tag without v -> exit 1" 1 - \
  EVENT_NAME=workflow_run HAVE_SECRETS=1 RUN_CONCLUSION=success RUN_HEAD_BRANCH=1.4.13
run_case "run: tag with no GitHub release -> exit 1" 1 - \
  EVENT_NAME=workflow_run HAVE_SECRETS=1 RUN_CONCLUSION=success RUN_HEAD_BRANCH=v1.4.99
run_case "run: draft release -> exit 1" 1 - \
  EVENT_NAME=workflow_run HAVE_SECRETS=1 RUN_CONCLUSION=success RUN_HEAD_BRANCH=v1.4.14
run_case "run: Release run failed -> exit 1" 1 - \
  EVENT_NAME=workflow_run HAVE_SECRETS=1 RUN_CONCLUSION=failure RUN_HEAD_BRANCH=v1.4.13
run_case "run: secrets absent -> exit 1" 1 - \
  EVENT_NAME=workflow_run HAVE_SECRETS=0 RUN_CONCLUSION=success RUN_HEAD_BRANCH=v1.4.13

# --- workflow_dispatch ---
run_case "dispatch: 1.4.12 -> version=1.4.12" 0 "version=1.4.12 tag=v1.4.12 latest=false proceed=true" \
  EVENT_NAME=workflow_dispatch HAVE_SECRETS=1 INPUT_VERSION=1.4.12
run_case "dispatch: v1.4.13 (leading v) is latest" 0 "version=1.4.13 latest=true proceed=true" \
  EVENT_NAME=workflow_dispatch HAVE_SECRETS=1 INPUT_VERSION=v1.4.13
run_case "dispatch: blank input uses GitHub's latest" 0 "version=1.4.13 latest=true proceed=true" \
  EVENT_NAME=workflow_dispatch HAVE_SECRETS=1 INPUT_VERSION=
run_case "dispatch: no such release -> exit 1" 1 - \
  EVENT_NAME=workflow_dispatch HAVE_SECRETS=1 INPUT_VERSION=9.9.9
run_case "dispatch: garbage input -> exit 1" 1 - \
  EVENT_NAME=workflow_dispatch HAVE_SECRETS=1 'INPUT_VERSION=1.4;rm'
run_case "dispatch: secrets absent -> exit 1" 1 - \
  EVENT_NAME=workflow_dispatch HAVE_SECRETS=0 INPUT_VERSION=1.4.12

# --- other ---
run_case "other event -> exit 1" 1 - EVENT_NAME=push HAVE_SECRETS=1
run_case "HAVE_SECRETS unset -> exit 1" 1 - \
  EVENT_NAME=workflow_run RUN_CONCLUSION=success RUN_HEAD_BRANCH=v1.4.13

echo "docker-publish-version_test: all cases passed"
