#!/usr/bin/env bash
# self_test.sh: fixture tests for scripts/ci/nself-ci-protect.sh (P7-CI-60).
# Recorded live GET fixtures (cli, plugins, packages; read 2026-10-04 in
# P7-HYG-14) and a stub gh first on PATH. The stub keeps a fake protection
# state and records every call; the live GitHub API is never called and no real
# repository is touched.
# Usage: self_test.sh   (exit 0 when every case passes, 1 otherwise)

# shellcheck disable=SC2015  # `check` helpers use a && b || c on purpose
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROTECT="${HERE}/../../nself-ci-protect.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT
mkdir -p "${TMP}/bin" "${TMP}/stub" "${TMP}/home" "${TMP}/work"
cp "${HERE}/stub-gh.sh" "${TMP}/bin/gh"
chmod +x "${TMP}/bin/gh"
export STUB_DIR="${TMP}/stub" HOME="${TMP}/home" TMPDIR="${TMP}/work"
export PATH="${TMP}/bin:${PATH}"
export AUDIT_LOG="${HOME}/audit.log" NSELF_CI_PROTECT_DIR="${TMP}/snap"
FAILS=0

pass() { printf 'PASS  %s\n' "$1"; }
fail() { printf 'FAIL  %s\n' "$1"; FAILS=$((FAILS + 1)); }
check() { # <description> <command...>
  local d="$1"; shift
  if "$@" > /dev/null 2>&1; then pass "${d}"; else fail "${d}"; fi
}
setup() { # <fixture>
  unset STUB_POST_STATE STUB_PUT_FAIL STUB_GET_FAIL
  rm -rf "${STUB_DIR:?}"/* "${NSELF_CI_PROTECT_DIR}"
  cp "${HERE}/$1.json" "${STUB_DIR}/state.json"
}
run() { # <args...>: output in ${TMP}/out, status in ${TMP}/rc
  local rc=0
  bash "${PROTECT}" "$@" > "${TMP}/out" 2>&1 || rc=$?
  echo "${rc}" > "${TMP}/rc"
}
rc_is() { [ "$(cat "${TMP}/rc")" = "$1" ]; }
puts() { grep -c '^PUT ' "${STUB_DIR}/calls" 2>/dev/null || true; }
adds() { grep -c '^ADD ' "${TMP}/out" || true; }
state_checks() { jq -c '[.required_status_checks.checks[] | {context, app_id: (.app_id // -1)}] | sort_by(.context)' "${STUB_DIR}/state.json"; }
body_checks() { # <file>
  jq -c '[.required_status_checks.checks[] | {context, app_id}] | sort_by(.context)' "$1"
}
put_body_json() { sed -n '/^PUT body for /,$p' "${TMP}/out" | sed '1d'; }
pre_file() { sed -n 's/^\[info\]  pre-state saved: \(.*\.json\) (restore.*$/\1/p' "${TMP}/out"; }

# 1. dry-run on cli: exactly one added check, every existing pin kept, no PUT.
setup cli-live
run --repo nself-org/cli --dry-run
check "cli dry-run exits 0" rc_is 0
check "cli dry-run adds exactly one check" test "$(adds)" = 1
check "cli dry-run adds {nself-ci, 15368}" grep -qx 'ADD nself-ci app_id 15368' "${TMP}/out"
put_body_json > "${TMP}/body.json"
check "cli dry-run body keeps both app-pinned checks and adds one" \
  test "$(body_checks "${TMP}/body.json")" = '[{"context":"Vet, Build & Test (ubuntu-latest)","app_id":15368},{"context":"golangci-lint","app_id":15368},{"context":"nself-ci","app_id":15368}]'
check "cli dry-run body holds no bare contexts" jq -e '.required_status_checks.contexts == []' "${TMP}/body.json"
check "cli dry-run sent no PUT" test "$(puts)" = 0
check "cli dry-run saved the pre-state" jq -e '.required_status_checks.checks | length == 2' "$(pre_file)"

# 2. plugins adds two, packages adds one.
setup plugins-live
run --repo nself-org/plugins --dry-run
check "plugins dry-run adds exactly two checks" test "$(adds)" = 2
check "plugins dry-run adds nself-ci/source" grep -qx 'ADD nself-ci/source app_id 15368' "${TMP}/out"
setup packages-live
run --repo nself-org/packages --dry-run
check "packages dry-run adds exactly one check" test "$(adds)" = 1

# 3. apply on cli: PUT the target body, GET, compare passes; second apply is a no-op.
setup cli-live
run --repo nself-org/cli --apply
FIRST_PRE="$(pre_file)"
check "cli apply exits 0 and verifies" rc_is 0
check "cli apply says verified" grep -q 'applied and verified' "${TMP}/out"
check "cli apply sent exactly one PUT" test "$(puts)" = 1
check "cli apply PUT body carries the three pinned checks" \
  test "$(body_checks "${STUB_DIR}/put-last.json")" = '[{"context":"Vet, Build & Test (ubuntu-latest)","app_id":15368},{"context":"golangci-lint","app_id":15368},{"context":"nself-ci","app_id":15368}]'
check "cli live state now has the pinned nself-ci" test "$(state_checks)" = '[{"context":"Vet, Build & Test (ubuntu-latest)","app_id":15368},{"context":"golangci-lint","app_id":15368},{"context":"nself-ci","app_id":15368}]'
run --repo nself-org/cli --apply
check "second apply is a no-op (no second PUT)" test "$(puts)" = 1
check "second apply says nothing to add" grep -q 'nothing to add' "${TMP}/out"

# 4. restore: PUTs the saved pre-state and verifies it.
check "the second run kept the first pre-state file" test "$(find "${NSELF_CI_PROTECT_DIR}" -type f | wc -l | tr -d ' ')" = 2
run --repo nself-org/cli --restore "${FIRST_PRE}"
check "restore exits 0 and verifies" rc_is 0
check "restore PUT body is the pre-state checks" test "$(body_checks "${STUB_DIR}/put-last.json")" = '[{"context":"Vet, Build & Test (ubuntu-latest)","app_id":15368},{"context":"golangci-lint","app_id":15368}]'
check "restore left no nself-ci behind" test "$(state_checks)" = '[{"context":"Vet, Build & Test (ubuntu-latest)","app_id":15368},{"context":"golangci-lint","app_id":15368}]'
check "restore verified identical to the pre-state" grep -q 'verified identical' "${TMP}/out"

# 5. a server that lands a different state fails the compare with the diff.
setup cli-live
export STUB_POST_STATE="${HERE}/cli-drift.json"
run --repo nself-org/cli --apply
check "apply against a different post-state exits 3" rc_is 3
check "apply prints the differing field" grep -q 'required_status_checks' "${TMP}/out"
check "apply names the restore command" grep -q -- '--restore' "${TMP}/out"
unset STUB_POST_STATE

# 6. refusals change nothing.
setup cli-live
jq '.required_status_checks.checks += [{"context":"nself-ci","app_id":777}] | .required_status_checks.contexts += ["nself-ci"]' "${HERE}/cli-live.json" > "${STUB_DIR}/state.json"
run --repo nself-org/cli --apply
check "pin conflict exits 2" rc_is 2
check "pin conflict names the check" grep -q 'pin conflict: nself-ci' "${TMP}/out"
check "pin conflict sent no PUT" test "$(puts)" = 0
setup cli-live
export STUB_GET_FAIL=1
run --repo nself-org/cli --apply
check "unreadable protection exits 2" rc_is 2
check "unreadable protection sent no PUT" test "$(puts)" = 0
unset STUB_GET_FAIL
setup cli-live
export STUB_PUT_FAIL=1
run --repo nself-org/cli --apply
check "rejected PUT exits 3" rc_is 3
unset STUB_PUT_FAIL
setup cli-live
run --repo nself-org/admin --dry-run
check "a repo with no pending entry has nothing to add" grep -q 'nothing to add' "${TMP}/out"
run --repo nself-org/cli --dry-run --apply
check "two modes are a usage error" rc_is 1
run --repo nself-org/cli
check "no mode is a usage error" rc_is 1

if [ "${FAILS}" -gt 0 ]; then printf '\n%d case(s) failed\n' "${FAILS}"; exit 1; fi
printf '\nall nself-ci-protect self-tests passed\n'
