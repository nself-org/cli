#!/usr/bin/env bash
# Sourced merge and verified auto-merge cleanup for admin-merge.sh.
# ── Merge PR ──────────────────────────────────────────────────────────────────
MERGE_SUCCESS=0
MERGE_RESULT=merged   # merged | failed | closed | auto_merge_only (decides the exit code)
# pr_state <n>: prints the PR state (OPEN, MERGED, CLOSED), or nothing when unreadable.
pr_state() { gh pr view "$1" --repo "${TARGET_REPO}" --json state --jq .state 2>/dev/null || true; }
# Disable even after a failed merge call: GitHub may have accepted it.
# A failed read is not evidence that auto-merge is off.
disable_auto_merge() {
  local armed
  gh pr merge "${PR_NUMBER}" --repo "${TARGET_REPO}" --disable-auto >/dev/null 2>&1 || true
  if armed=$(gh pr view "${PR_NUMBER}" --repo "${TARGET_REPO}" --json autoMergeRequest --jq '.autoMergeRequest' 2>/dev/null) && [ "${armed}" = null ]; then
    info "Auto-merge was disabled again."
  else
    error "Could not verify auto-merge is disabled; it may still be armed."
    audit "MERGE  pr=${PR_NUMBER}  result=auto_merge_still_armed"
  fi
}
if [ "${DRY_RUN}" -eq 0 ]; then
  info "Merging PR #${PR_NUMBER}..."
  if gh pr merge "${PR_NUMBER}" \
      --repo "${TARGET_REPO}" \
      --squash \
      --delete-branch \
      --auto 2>&1; then
    # `--auto` exits 0 when it only ENABLES auto-merge (non-required checks still
    # pending), so the exit code proves nothing: read the PR state back.
    MERGE_DEADLINE=$(( $(date '+%s') + MERGE_VERIFY_SECONDS ))
    while :; do
      PR_STATE=$(pr_state "${PR_NUMBER}")
      if [ "${PR_STATE}" = "MERGED" ]; then MERGE_SUCCESS=1; break; fi
      if [ "${PR_STATE}" = "CLOSED" ]; then break; fi
      if [ "$(date '+%s')" -ge "${MERGE_DEADLINE}" ]; then break; fi
      sleep "${MERGE_POLL_SECONDS}"
    done
    if [ "${MERGE_SUCCESS}" -eq 0 ]; then
      # Do not leave auto-merge armed behind a restored protection, then look once
      # more: it may have fired between the last read and now.
      disable_auto_merge
      PR_STATE=$(pr_state "${PR_NUMBER}")
      if [ "${PR_STATE}" = "MERGED" ]; then MERGE_SUCCESS=1; fi
    fi
    if [ "${MERGE_SUCCESS}" -eq 1 ]; then
      ok "PR #${PR_NUMBER} merged."
      audit "MERGE  pr=${PR_NUMBER}  result=success"
    elif [ "${PR_STATE}" = "CLOSED" ]; then
      MERGE_RESULT=closed
      error "PR #${PR_NUMBER} is CLOSED, not MERGED."
      audit "MERGE  pr=${PR_NUMBER}  result=closed"
    else
      MERGE_RESULT=auto_merge_only
      error "PR #${PR_NUMBER} is ${PR_STATE:-unreadable}, not MERGED, ${MERGE_VERIFY_SECONDS}s after the merge call: GitHub only set auto-merge (non-required checks still pending?)."
      error "Not waiting for CI. Protection is restored below."
      audit "MERGE  pr=${PR_NUMBER}  result=auto_merge_only  state=${PR_STATE:-unreadable}"
    fi
  else
    # The call can fail after the merge landed (network drop): read the state once.
    if [ "$(pr_state "${PR_NUMBER}")" = "MERGED" ]; then
      MERGE_SUCCESS=1
      ok "PR #${PR_NUMBER} merged (the merge call reported an error, the PR reads MERGED)."
      audit "MERGE  pr=${PR_NUMBER}  result=success_after_error"
    else
      disable_auto_merge
      MERGE_RESULT=failed
      error "PR merge failed."
      audit "MERGE  pr=${PR_NUMBER}  result=failed"
      # Still restore even on failure
    fi
  fi
else
  info "[DRY-RUN] Would merge PR #${PR_NUMBER} (squash + delete branch), then verify state MERGED."
  MERGE_SUCCESS=1
fi
