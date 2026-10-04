#!/usr/bin/env bash
# protection_snapshot_test.sh
#
# Fixture tests for scripts/lib/protection_snapshot.sh and the restore path of
# scripts/admin-merge.sh. Uses a STUBBED `gh` (and `nohup`) first on PATH: the
# stub keeps a fake branch-protection state, turns PUT bodies into GET-shaped
# state like GitHub does, and records every call. The live GitHub API is never
# called and no real repository is touched.
#
# Usage: protection_snapshot_test.sh [--case round-trip|mismatch|review-fields|
#                                             dry-run|watchdog-timeout|merge-flow]
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
cat > "${TMP}/bin/gh" << 'STUB_EOF'
#!/usr/bin/env bash
# Stub gh. State: $STUB_DIR/state.json (GET shape). Env: STUB_DROP="field ..."
# (server forgets those fields on PUT), STUB_PUT_FAIL=1, STUB_KILL_PARENT=1.
D="${STUB_DIR:?}"
case "$1" in
  auth) echo "Token scopes: 'repo'"; exit 0 ;;
  run) echo "completed/success"; exit 0 ;;
  pr)
    echo "pr $2" >> "${D}/calls"
    if [ "$2" = merge ] && [ "${STUB_KILL_PARENT:-0}" = 1 ]; then kill -9 "${PPID}"; fi
    exit 0 ;;
  api) ;;
  *) exit 0 ;;
esac
shift
path="$1"; shift; method=GET
while [ $# -gt 0 ]; do
  case "$1" in -X) method="$2"; shift 2 ;; -H|--input) shift 2 ;; *) shift ;; esac
done
if [ "${method}" = PUT ]; then
  echo "PUT ${path}" >> "${D}/calls"
  body="$(cat)"
  if [ "${STUB_PUT_FAIL:-0}" = 1 ]; then echo "HTTP 422 stub" >&2; exit 1; fi
  printf '%s' "${body}" | jq '
    def pr: {users: [(.users // [])[] | {login: .}], teams: [(.teams // [])[] | {slug: .}], apps: [(.apps // [])[] | {slug: .}]};
    def en: {enabled: (. // false)};
    {url: "stub",
     required_status_checks: (if .required_status_checks then .required_status_checks as $r | {
        strict: $r.strict,
        contexts: (($r.contexts // []) + [($r.checks // [])[].context]),
        checks: ((($r.contexts // []) | map({context: ., app_id: null}))
          + (($r.checks // []) | map({context, app_id: (if .app_id == -1 then null else .app_id end)})))} else null end),
     enforce_admins: (.enforce_admins | en),
     required_pull_request_reviews: (if .required_pull_request_reviews then .required_pull_request_reviews as $r
        | ($r | {dismiss_stale_reviews, require_code_owner_reviews, required_approving_review_count, require_last_push_approval})
        + (if $r.dismissal_restrictions then {dismissal_restrictions: ($r.dismissal_restrictions | pr)} else {} end)
        + (if $r.bypass_pull_request_allowances then {bypass_pull_request_allowances: ($r.bypass_pull_request_allowances | pr)} else {} end)
        else null end),
     restrictions: (if .restrictions then (.restrictions | pr) else null end),
     required_linear_history: (.required_linear_history | en), allow_force_pushes: (.allow_force_pushes | en),
     allow_deletions: (.allow_deletions | en), block_creations: (.block_creations | en),
     required_conversation_resolution: (.required_conversation_resolution | en),
     lock_branch: (.lock_branch | en), allow_fork_syncing: (.allow_fork_syncing | en)}
    | with_entries(select(.value != null))' > "${D}/state.json"
  for f in ${STUB_DROP:-}; do jq "del(.${f})" "${D}/state.json" > "${D}/s.tmp" && mv "${D}/s.tmp" "${D}/state.json"; done
  exit 0
fi
echo "GET ${path}" >> "${D}/calls"
cat "${D}/state.json"
STUB_EOF
# nohup stub: run the command in the foreground of its own background job and
# record the exit code, so the test can assert the detached watchdog's status.
cat > "${TMP}/bin/nohup" << 'STUB_EOF'
#!/usr/bin/env bash
"$@"
echo $? > "${STUB_DIR}/watchdog.rc"
STUB_EOF
chmod +x "${TMP}/bin/gh" "${TMP}/bin/nohup"

# shellcheck source=protection_snapshot.sh
. "${LIB}"

# -- helpers ------------------------------------------------------------------
setup() { # <fixture-name>: fresh server state and logs
  unset STUB_DROP STUB_PUT_FAIL STUB_KILL_PARENT
  rm -rf "${STUB_DIR}/calls" "${STUB_DIR}/watchdog.rc" "${HOME}/.nself" "${TMP}"/work/*
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

# -- main ---------------------------------------------------------------------
want=""
if [ "${1:-}" = "--case" ]; then want="${2:-}"; fi
for c in round-trip mismatch review-fields dry-run watchdog-timeout merge-flow; do
  if [ -z "${want}" ] || [ "${want}" = "${c}" ]; then
    printf '== %s\n' "${c}"
    "case_$(printf '%s' "${c}" | tr '-' '_')"
  fi
done
if [ "${FAILS}" -ne 0 ]; then printf '%s check(s) FAILED\n' "${FAILS}"; exit 1; fi
printf 'all checks passed\n'
