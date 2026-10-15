#!/usr/bin/env bats
# admin-merge.bats  (D-0829)
#
# Offline tests for the merge-verification tail of scripts/admin-merge.sh. `gh`
# is a stateful stub on PATH: protection reads/writes succeed, `pr view` reports
# an OPEN PR, and `pr merge` / `--disable-auto` behave as the test configures.
# Every call is logged to GH_LOG. No test touches the network or GitHub.

SCRIPT="${BATS_TEST_DIRNAME}/../admin-merge.sh"
# macOS system bash 3.2 is the shell where the 08/09 octal bug exits 0.
if [ -x /bin/bash ]; then BASH_BIN=/bin/bash; else BASH_BIN=bash; fi

setup() {
  export HOME="${BATS_TEST_TMPDIR}/home"
  export TMPDIR="${BATS_TEST_TMPDIR}/tmp"
  mkdir -p "${HOME}" "${TMPDIR}" "${BATS_TEST_TMPDIR}/bin"
  export GH_LOG="${BATS_TEST_TMPDIR}/gh.log"
  export GH_ARMED="${BATS_TEST_TMPDIR}/armed"
  : > "${GH_LOG}"
  cat > "${BATS_TEST_TMPDIR}/protection.json" <<'JSON'
{
  "required_status_checks": {"strict": false, "contexts": ["ci"], "checks": [{"context": "ci", "app_id": 15368}]},
  "required_pull_request_reviews": {"dismiss_stale_reviews": false, "require_code_owner_reviews": false, "require_last_push_approval": false, "required_approving_review_count": 1},
  "required_signatures": {"enabled": false},
  "enforce_admins": {"enabled": true},
  "required_linear_history": {"enabled": false},
  "allow_force_pushes": {"enabled": false},
  "allow_deletions": {"enabled": false},
  "block_creations": {"enabled": false},
  "required_conversation_resolution": {"enabled": false},
  "lock_branch": {"enabled": false},
  "allow_fork_syncing": {"enabled": false}
}
JSON
  export GH_PROTECTION="${BATS_TEST_TMPDIR}/protection.json"
  cat > "${BATS_TEST_TMPDIR}/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${GH_LOG}"
case "$*" in
  "auth status"*) echo "Token scopes: 'repo'"; exit 0 ;;
  "api repos/"*"/protection"*) [ "$*" != "${*/-X PUT/}" ] && exit 0; cat "${GH_PROTECTION}"; exit 0 ;;
  "pr merge"*--disable-auto*)
    [ "${GH_DISABLE_RC:-0}" -eq 0 ] && rm -f "${GH_ARMED}"
    exit "${GH_DISABLE_RC:-0}" ;;
  "pr merge"*) : > "${GH_ARMED}"; exit "${GH_MERGE_RC:-0}" ;;  # a failed call may still have armed it
  "pr view"*autoMergeRequest*) if [ -e "${GH_ARMED}" ]; then echo false; else echo true; fi; exit 0 ;;
  "pr view"*) echo OPEN; exit 0 ;;
esac
exit 98
STUB
  chmod +x "${BATS_TEST_TMPDIR}/bin/gh"
  export PATH="${BATS_TEST_TMPDIR}/bin:${PATH}"
  export MERGE_VERIFY_SECONDS=1 MERGE_POLL_SECONDS=1 WATCHDOG_DEADLINE=30
}

# run_merge: answers the prompt with y; output and status land in bats' run vars.
run_merge() {
  run "${BASH_BIN}" -c '"$0" "$1" --repo nself-org/cli --branch fix/x --pr 7 --no-wait-ci <<< y' "${BASH_BIN}" "${SCRIPT}"
}

@test "disable-auto confirmed: the script says auto-merge was disabled and exits 1" {
  run_merge
  [ "${status}" -eq 1 ]
  [[ "${output}" == *"Auto-merge was disabled again."* ]]
  [[ "${output}" != *"auto_merge_still_armed"* ]]
}

@test "disable-auto fails: never claims it disabled auto-merge, audits auto_merge_still_armed" {
  GH_DISABLE_RC=1 run_merge
  [ "${status}" -eq 1 ]
  [[ "${output}" != *"disabled again"* ]]
  [[ "${output}" == *"still be armed"* ]]
  grep -q 'auto_merge_still_armed' "${HOME}/.nself/admin-merge-audit.log"
  grep -q 'protection' "${GH_LOG}"
}

@test "failed merge call: auto-merge is disabled before protection is restored" {
  GH_MERGE_RC=1 run_merge
  [ "${status}" -eq 1 ]
  [[ "${output}" == *"PR merge failed."* ]]
  grep -q -- '--disable-auto' "${GH_LOG}"
}

@test "failed merge call and disable-auto fails: audits auto_merge_still_armed" {
  GH_MERGE_RC=1 GH_DISABLE_RC=1 run_merge
  [ "${status}" -eq 1 ]
  grep -q 'auto_merge_still_armed' "${HOME}/.nself/admin-merge-audit.log"
}

@test "MERGE_VERIFY_SECONDS=08 is decimal 8, not an octal error: exits 1 for an unmerged PR" {
  MERGE_VERIFY_SECONDS=08 MERGE_POLL_SECONDS=2 run_merge
  [ "${status}" -eq 1 ]
  [[ "${output}" == *"was NOT merged"* ]]
  [[ "${output}" == *"8s after the merge call"* ]]
}

@test "MERGE_POLL_SECONDS=09 and WATCHDOG_DEADLINE=09 normalise to decimal" {
  MERGE_POLL_SECONDS=09 WATCHDOG_DEADLINE=09 run "${BASH_BIN}" "${SCRIPT}" --repo nself-org/cli --branch fix/x --pr 7 --dry-run
  [ "${status}" -eq 0 ]
  [[ "${output}" == *"Watchdog:     9s deadline"* ]]
}
