# shellcheck shell=bash
# ── Merge PR ──────────────────────────────────────────────────────────────────
MERGE_SUCCESS=0
MERGE_RESULT=merged   # merged | failed | closed | auto_merge_only (decides the exit code)
# pr_state <n>: prints the PR state (OPEN, MERGED, CLOSED), or nothing when unreadable.
pr_state() { gh pr view "$1" --repo "${TARGET_REPO}" --json state --jq .state 2>/dev/null || true; }
if [ "${DRY_RUN}" -eq 0 ]; then
  verify_pending_bypass
  audit "MERGE_ATTEMPT  pr=${PR_NUMBER}  method=${MERGE_METHOD}  backlog=${ADMIN_BYPASS_CASE:-none}"
  info "Merging PR #${PR_NUMBER}..."
  if gh pr merge "${PR_NUMBER}" \
      --repo "${TARGET_REPO}" \
      --squash \
      --delete-branch \
      "${MERGE_METHOD}" 2>&1; then
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
      gh pr merge "${PR_NUMBER}" --repo "${TARGET_REPO}" --disable-auto >/dev/null 2>&1 || true
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
      error "PR #${PR_NUMBER} is ${PR_STATE:-unreadable}, not MERGED, ${MERGE_VERIFY_SECONDS}s after the merge call: GitHub only set auto-merge (non-required checks still pending?). Auto-merge was disabled again."
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
      MERGE_RESULT=failed
      error "PR merge failed."
      audit "MERGE  pr=${PR_NUMBER}  result=failed"
      # Still restore even on failure
    fi
  fi
else
  info "[DRY-RUN] Would merge PR #${PR_NUMBER} (squash + delete branch, ${MERGE_METHOD}), then verify state MERGED."
  MERGE_SUCCESS=1
fi

# ── Wait for CI (optional) ────────────────────────────────────────────────────
if [ "${NO_WAIT_CI}" -eq 0 ] && [ "${MERGE_SUCCESS}" -eq 1 ] && [ "${DRY_RUN}" -eq 0 ]; then
  info "Waiting up to ${WAIT_MINUTES} minutes for post-merge CI to pass..."
  WAIT_DEADLINE=$(( $(date '+%s') + WAIT_MINUTES * 60 ))
  CI_PASSED=0

  while [ "$(date '+%s')" -lt "${WAIT_DEADLINE}" ]; do
    # Query CI for the merge commit on main
    RUN_STATUS=$(gh run list \
      --repo "${TARGET_REPO}" \
      --branch main \
      --limit 1 \
      --json status,conclusion \
      --jq '.[0] | "\(.status)/\(.conclusion)"' 2>/dev/null || printf 'unknown/unknown')

    STATUS_PART=$(printf '%s' "${RUN_STATUS}" | cut -d'/' -f1)
    CONCLUSION_PART=$(printf '%s' "${RUN_STATUS}" | cut -d'/' -f2)

    if [ "${STATUS_PART}" = "completed" ]; then
      if [ "${CONCLUSION_PART}" = "success" ]; then
        ok "CI passed."
        CI_PASSED=1
        audit "CI_PASS  repo=${TARGET_REPO}"
      else
        warn "CI completed with conclusion: ${CONCLUSION_PART}"
        audit "CI_FAIL  repo=${TARGET_REPO}  conclusion=${CONCLUSION_PART}"
      fi
      break
    fi

    printf '  CI status: %s/%s — waiting...\n' "${STATUS_PART}" "${CONCLUSION_PART}"
    sleep "${CI_POLL_SECONDS}"
  done

  if [ "${CI_PASSED}" -eq 0 ]; then
    warn "CI did not pass within ${WAIT_MINUTES} minutes. Restoring protection anyway."
    audit "CI_TIMEOUT  repo=${TARGET_REPO}"
  fi
fi

# ── Restore branch protection (verified; exit 3 on failure) ───────────────────
# On failure the EXIT trap retries once and the watchdog tries again at its
# deadline; the done marker stays absent until a restore is verified.
restore_protection "${TARGET_REPO}" "${SNAPSHOT_FILE}" "${DRY_RUN}" "post_merge" || exit 3

# ── Signal watchdog that we're done ──────────────────────────────────────────
touch "${DONE_MARKER}"
rm -f "${SNAPSHOT_FILE}"
audit "DONE  repo=${TARGET_REPO}  pr=${PR_NUMBER}  merge=${MERGE_RESULT}"

# Protection is back and verified; the PR itself did not land.
if [ "${MERGE_RESULT}" != "merged" ]; then
  error "PR #${PR_NUMBER} was NOT merged (${MERGE_RESULT}). Branch protection is restored and verified."
  exit 1
fi

ok "Admin merge complete. Branch protection restored and verified. Watchdog will self-exit."

printf '\n'
info "Audit log: ${AUDIT_LOG}"
if [ "${DRY_RUN}" -eq 0 ]; then
  info "Watchdog log: ${WORK_DIR}/nself-watchdog-${MERGE_TS}.log"
fi
