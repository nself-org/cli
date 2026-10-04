#!/usr/bin/env bash
# protection_snapshot_test.sh
#
# Fixture tests for scripts/lib/protection_snapshot.sh and the restore path of
# scripts/admin-merge.sh. Uses a STUBBED `gh` (and `nohup`) first on PATH: the
# stub keeps a fake branch-protection state, turns PUT bodies into GET-shaped
# state like GitHub does, and records every call. The live GitHub API is never
# called and no real repository is touched.
#
# The PATH `bash` is /bin/bash when it exists (3.2 on macOS), so admin-merge.sh
# and its watchdog run under the oldest supported bash.
#
# Usage: protection_snapshot_test.sh [--case round-trip|mismatch|review-fields|
#                                             dry-run|watchdog-timeout|merge-flow|
#                                             slow-ci|relax-ambiguous|force-restore]
# With no --case every case runs. Exit 0 when all pass, 1 otherwise.

# shellcheck disable=SC2016,SC2015,SC2119,SC2120,SC1091  # bash -c bodies are single-quoted on purpose
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="${HERE}/protection_snapshot.sh"
AM="${HERE}/../admin-merge.sh"
FX="${HERE}/testdata/protection"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT
mkdir -p "${TMP}/bin" "${TMP}/stub" "${TMP}/work" "${TMP}/home"

export STUB_DIR="${TMP}/stub" HOME="${TMP}/home" TMPDIR="${TMP}/work"
export PATH="${TMP}/bin:${PATH}"
export AUDIT_LOG="${HOME}/.nself/admin-merge-audit.log"
REPO="nself-org/fixture"
API="repos/${REPO}/branches/main/protection"
FAILS=0

# -- stubs --------------------------------------------------------------------
# Stubs live in testdata/protection/ (stub-gh.sh: fake protection state, schema-
# checking PUT, call log; stub-nohup.sh: records the watchdog's exit code).
cp "${FX}/stub-gh.sh" "${TMP}/bin/gh"
cp "${FX}/stub-nohup.sh" "${TMP}/bin/nohup"
chmod +x "${TMP}/bin/gh" "${TMP}/bin/nohup"
if [ -x /bin/bash ]; then ln -s /bin/bash "${TMP}/bin/bash"; fi

# shellcheck source=protection_snapshot.sh
. "${LIB}"

# -- helpers ------------------------------------------------------------------
setup() { # <fixture-name>: fresh server state and logs
  unset STUB_DROP STUB_PUT_FAIL STUB_KILL_PARENT STUB_SLOW_CI STUB_FAIL_AFTER_LAND_ONCE STUB_GET_STDERR
  rm -rf "${STUB_DIR}/calls" "${STUB_DIR}/watchdog.rc" "${STUB_DIR}/runcount" "${STUB_DIR}/landed" "${HOME}/.nself" "${TMP}"/work/*
  cp "${FX}/$1.json" "${STUB_DIR}/state.json"
  cp "${FX}/$1.json" "${STUB_DIR}/snapshot.json"
}
server() { cat "${STUB_DIR}/state.json"; }
relax() { build_relax_body "$(server)" | gh api "${API}" -X PUT --input - > /dev/null; }
pass() { printf 'PASS  %s\n' "$1"; }
fail() { printf 'FAIL  %s\n' "$1"; FAILS=$((FAILS + 1)); }
check() { # <description> <command...>
  local d="$1"; shift
  if "$@" > /dev/null 2>&1; then pass "${d}"; else fail "${d}"; fi
}
audit_has() { grep -q -- "$1" "${AUDIT_LOG}" 2>/dev/null; }
no_puts() { ! grep -q '^PUT' "${STUB_DIR}/calls"; }
wait_watchdog() {
  local i=0
  while [ ! -f "${STUB_DIR}/watchdog.rc" ] && [ "${i}" -lt 60 ]; do sleep 0.5; i=$((i + 1)); done
  [ -f "${STUB_DIR}/watchdog.rc" ]
}
run_am() { # extra admin-merge args; answers the [y/N] prompt with y
  printf 'y\n' | bash "${AM}" --repo "${REPO}" --branch feat --pr 7 --no-wait-ci "$@"
}

# -- cases --------------------------------------------------------------------
case_round_trip() {
  local f snap body
  for f in cli-like plugins-like checks-app-ids review-fields; do
    setup "${f}"
    snap="$(cat "${FX}/${f}.json")"
    body="$(snapshot_to_put_body "${snap}")"
    check "${f}: PUT body has unwrapped booleans, no url keys" \
      bash -c 'jq -e "(.enforce_admins|type==\"boolean\") and (.required_linear_history|type==\"boolean\") and ([..|objects|has(\"url\")]|any|not)" <<< "$1"' _ "${body}"
    relax
    check "${f}: relax body really loosens the server" \
      bash -c '! jq -e ".enforce_admins.enabled" "$1"' _ "${STUB_DIR}/state.json"
    check "${f}: restore_protection returns 0" restore_protection "${REPO}" "${STUB_DIR}/snapshot.json" 0 test
    check "${f}: normalise(GET after PUT) == normalise(snapshot)" compare "${snap}" "$(server)"
  done
  setup cli-like
  body="$(snapshot_to_put_body "$(cat "${FX}/cli-like.json")")"
  check "cli-like: enforce_admins, conversation resolution, linear history all true" \
    bash -c 'jq -e ".enforce_admins and .required_conversation_resolution and .required_linear_history" <<< "$1"' _ "${body}"
  check "cli-like: unpinned checks sent as checks with app_id -1 and contexts []" \
    bash -c 'jq -e "(.required_status_checks.contexts==[]) and ([.required_status_checks.checks[].app_id]|unique==[-1])" <<< "$1"' _ "${body}"
  check "stub rejects a body without contexts (schema guard works)" \
    bash -c '! printf "%s" "{\"required_status_checks\":{\"strict\":true,\"checks\":[]},\"enforce_admins\":true,\"required_pull_request_reviews\":null,\"restrictions\":null}" | gh api "$1" -X PUT --input -' _ "${API}"
  setup plugins-like
  body="$(snapshot_to_put_body "$(cat "${FX}/plugins-like.json")")"
  check "plugins-like: enforce_admins stays false, no forced stale-review dismissal" \
    bash -c 'jq -e "(.enforce_admins|not) and (.required_pull_request_reviews.dismiss_stale_reviews|not)" <<< "$1"' _ "${body}"
  setup checks-app-ids
  body="$(snapshot_to_put_body "$(cat "${FX}/checks-app-ids.json")")"
  check "checks-app-ids: checks carry app ids (15368, -1), contexts empty" \
    bash -c 'jq -e "(.required_status_checks.contexts==[]) and ([.required_status_checks.checks[].app_id]|sort==[-1,15368,15368])" <<< "$1"' _ "${body}"
}

case_mismatch() {
  local out
  setup cli-like; relax
  export STUB_DROP=enforce_admins
  out="$(restore_protection "${REPO}" "${STUB_DIR}/snapshot.json" 0 test 2>&1)" && fail "dropped field: restore must fail" || pass "dropped field: restore exits non-zero"
  check "dropped field: output names enforce_admins" grep -q 'enforce_admins' <<< "${out}"
  check "dropped field: restore_mismatch audit line" audit_has 'restore_mismatch'
  setup cli-like; relax
  export STUB_PUT_FAIL=1
  out="$(restore_protection "${REPO}" "${STUB_DIR}/snapshot.json" 0 test 2>&1)" && fail "rejected PUT: restore must fail" || pass "rejected PUT: restore exits non-zero"
  check "rejected PUT: RESTORE_FAILED audit line" audit_has 'RESTORE_FAILED'
  setup cli-like; relax
  jq '.required_signatures.enabled = true' "${FX}/cli-like.json" > "${TMP}/sig.json"
  out="$(restore_protection "${REPO}" "${TMP}/sig.json" 0 test 2>&1)" && fail "required_signatures: restore must fail" || pass "required_signatures: restore refuses"
  check "required_signatures: named in the message" grep -q 'required_signatures' <<< "${out}"
  jq '.some_new_field = {"enabled": true}' "${FX}/cli-like.json" > "${TMP}/new.json"
  check "unknown field: snapshot_to_put_body fails" bash -c '! { . "$1"; snapshot_to_put_body "$(cat "$2")"; } 2>/dev/null' _ "${LIB}" "${TMP}/new.json"
  setup cli-like; relax
  export STUB_GET_STDERR=1
  check "stderr noise on the verify GET does not break the restore" restore_protection "${REPO}" "${STUB_DIR}/snapshot.json" 0 test
  setup cli-like
  check "compare: equal snapshots pass" compare "$(cat "${FX}/cli-like.json")" "$(cat "${FX}/cli-like.json")"
  check "compare: differing snapshots fail" bash -c '. "$1"; ! compare "$(cat "$2")" "$(cat "$3")"' _ "${LIB}" "${FX}/cli-like.json" "${FX}/plugins-like.json"
}

case_review_fields() {
  local n body
  n="$(normalise "$(cat "${FX}/review-fields.json")")"
  body="$(snapshot_to_put_body "$(cat "${FX}/review-fields.json")")"
  check "normalise carries dismissal_restrictions (sorted logins, slugs)" \
    bash -c 'jq -e ".required_pull_request_reviews.dismissal_restrictions == {users:[\"alice\",\"zed\"],teams:[\"core\"],apps:[\"release-bot\"]}" <<< "$1"' _ "${n}"
  check "normalise carries bypass_pull_request_allowances" \
    bash -c 'jq -e ".required_pull_request_reviews.bypass_pull_request_allowances == {users:[\"bob\"],teams:[\"maintainers\"],apps:[\"merge-queue\"]}" <<< "$1"' _ "${n}"
  check "PUT body carries both, plus code-owner and last-push approval" \
    bash -c 'jq -e ".required_pull_request_reviews | (.dismissal_restrictions.users|length==2) and (.bypass_pull_request_allowances.apps==[\"merge-queue\"]) and .require_code_owner_reviews and .require_last_push_approval and (.required_approving_review_count==2)" <<< "$1"' _ "${body}"
  check "PUT body carries restrictions and block_creations" \
    bash -c 'jq -e ".restrictions == {users:[\"carol\"],teams:[\"release\"],apps:[\"deployer\"]} and .block_creations" <<< "$1"' _ "${body}"
  setup review-fields; relax
  check "review-fields survive a relax + restore" restore_protection "${REPO}" "${STUB_DIR}/snapshot.json" 0 test
  check "review-fields: server state equals snapshot" compare "$(cat "${FX}/review-fields.json")" "$(server)"
}

case_dry_run() {
  local out rc=0
  setup cli-like
  out="$(bash "${AM}" --repo "${REPO}" --branch feat --pr 7 --dry-run 2>&1)" || rc=$?
  check "dry-run exits 0" test "${rc}" -eq 0
  check "dry-run prints the PUT body it would send" grep -q '"enforce_admins": true' <<< "${out}"
  check "dry-run says Would PUT" grep -q 'Would PUT' <<< "${out}"
  check "dry-run: stub gh recorded no PUT" no_puts
  check "dry-run: stub gh recorded no merge" bash -c '! grep -q "pr merge" "$1"' _ "${STUB_DIR}/calls"
  check "dry-run leaves server state untouched" compare "$(cat "${FX}/cli-like.json")" "$(server)"
}

case_watchdog_timeout() {
  # The stub gh SIGKILLs admin-merge.sh at the merge step: no trap, no done
  # marker. The detached watchdog (deadline 2s) must restore from the snapshot.
  export WATCHDOG_DEADLINE=2
  setup cli-like
  export STUB_KILL_PARENT=1
  run_am > "${TMP}/am.out" 2>&1 || true
  check "watchdog: finishes" wait_watchdog
  check "watchdog: exits 0" test "$(cat "${STUB_DIR}/watchdog.rc" 2>/dev/null)" = 0
  check "watchdog: PUT relax then PUT restore, then GET verify" \
    bash -c 'p=$(grep -c "^PUT" "$1"); [ "$p" -eq 2 ] && [ "$(tail -1 "$1")" = "GET '"${API}"'" ]' _ "${STUB_DIR}/calls"
  check "watchdog: restored state equals the snapshot" compare "$(cat "${FX}/cli-like.json")" "$(server)"
  check "watchdog: RESTORE audit line with watchdog_timeout" audit_has 'RESTORE .*watchdog_timeout'

  setup cli-like
  export STUB_KILL_PARENT=1 STUB_DROP=enforce_admins
  run_am > "${TMP}/am.out" 2>&1 || true
  check "watchdog (dropped field): finishes" wait_watchdog
  check "watchdog (dropped field): exits 3" test "$(cat "${STUB_DIR}/watchdog.rc" 2>/dev/null)" = 3
  check "watchdog (dropped field): restore_mismatch audit line" audit_has 'restore_mismatch'
  unset WATCHDOG_DEADLINE
}

case_merge_flow() {
  local rc=0
  export WATCHDOG_DEADLINE=2
  setup cli-like
  run_am > "${TMP}/am.out" 2>&1 || rc=$?
  check "merge: exits 0" test "${rc}" -eq 0
  check "merge: protection equals the pre-merge snapshot" compare "$(cat "${FX}/cli-like.json")" "$(server)"
  check "merge: done marker written, DONE audited" bash -c 'ls "$1"/nself-admin-merge-*.done && grep -q "DONE" "$2"' _ "${TMPDIR}" "${AUDIT_LOG}"
  wait_watchdog || true

  setup checks-app-ids
  export STUB_DROP=enforce_admins; rc=0
  run_am > "${TMP}/am.out" 2>&1 || rc=$?
  check "merge (dropped field): exits 3" test "${rc}" -eq 3
  check "merge (dropped field): restore_mismatch audited" audit_has 'restore_mismatch'
  check "merge (dropped field): no done marker" bash -c '! ls "$1"/nself-admin-merge-*.done' _ "${TMPDIR}"
  wait_watchdog || true
  unset WATCHDOG_DEADLINE
}

case_slow_ci() {
  # CI outlasts the watchdog deadline: the watchdog restores first, the parent
  # must still find the snapshot, verify, and exit 0 (no false exit 3).
  local rc=0
  export WATCHDOG_DEADLINE=2 CI_POLL_SECONDS=4
  setup cli-like
  export STUB_SLOW_CI=1
  printf 'y\n' | bash "${AM}" --repo "${REPO}" --branch feat --pr 7 --wait-minutes 1 > "${TMP}/am.out" 2>&1 || rc=$?
  check "slow CI: parent exits 0" test "${rc}" -eq 0
  check "slow CI: watchdog restored first" audit_has 'RESTORE .*watchdog_timeout'
  check "slow CI: parent restore verified afterwards" audit_has 'RESTORE .*post_merge'
  check "slow CI: no RESTORE_FAILED / restore_mismatch" bash -c '! grep -qE "RESTORE_FAILED|restore_mismatch" "$1"' _ "${AUDIT_LOG}"
  check "slow CI: protection equals the snapshot" compare "$(cat "${FX}/cli-like.json")" "$(server)"
  check "slow CI: snapshot file removed by the parent" bash -c '! ls "$1"/nself-admin-merge-*.snapshot.json' _ "${TMPDIR}"
  wait_watchdog || true
  unset WATCHDOG_DEADLINE CI_POLL_SECONDS
}

case_relax_ambiguous() {
  # The relax PUT lands but gh reports failure: protection must be restored.
  local rc=0
  export WATCHDOG_DEADLINE=2
  setup cli-like
  export STUB_FAIL_AFTER_LAND_ONCE=1
  run_am > "${TMP}/am.out" 2>&1 || rc=$?
  check "ambiguous relax: exits non-zero (1)" test "${rc}" -eq 1
  check "ambiguous relax: restored from the snapshot" audit_has 'RESTORE .*relax_failed'
  check "ambiguous relax: protection equals the snapshot" compare "$(cat "${FX}/cli-like.json")" "$(server)"
  check "ambiguous relax: no merge attempted" bash -c '! grep -q "pr merge" "$1"' _ "${STUB_DIR}/calls"
  wait_watchdog || true
  unset WATCHDOG_DEADLINE
}

case_force_restore() {
  # Recovery must work from a checkout on main (the printed hint has no --branch).
  local rc=0
  setup cli-like; relax
  git init -q "${TMP}/mainrepo"
  git -C "${TMP}/mainrepo" checkout -q -b main
  git -C "${TMP}/mainrepo" -c user.name=t -c user.email=t@t.invalid commit -q --allow-empty -m init
  (cd "${TMP}/mainrepo" && bash "${AM}" --repo "${REPO}" --force-restore --snapshot "${STUB_DIR}/snapshot.json") > "${TMP}/am.out" 2>&1 || rc=$?
  check "force-restore from main: exits 0" test "${rc}" -eq 0
  check "force-restore from main: protection equals the snapshot" compare "$(cat "${FX}/cli-like.json")" "$(server)"
  check "force-restore refuses a merge from main" bash -c '! (cd "$1" && printf "y\n" | bash "$2" --repo "$3" --pr 7 --dry-run) > /dev/null 2>&1' _ "${TMP}/mainrepo" "${AM}" "${REPO}"
}

# -- main ---------------------------------------------------------------------
want=""
if [ "${1:-}" = "--case" ]; then want="${2:-}"; fi
for c in round-trip mismatch review-fields dry-run watchdog-timeout merge-flow slow-ci relax-ambiguous force-restore; do
  if [ -z "${want}" ] || [ "${want}" = "${c}" ]; then
    printf '== %s\n' "${c}"
    "case_$(printf '%s' "${c}" | tr '-' '_')"
  fi
done
if [ "${FAILS}" -ne 0 ]; then printf '%s check(s) FAILED\n' "${FAILS}"; exit 1; fi
printf 'all checks passed\n'
