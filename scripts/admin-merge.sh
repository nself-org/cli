#!/usr/bin/env bash
# admin-merge.sh
#
# Solo-operator merge helper for nself-org repos.
#
# Problem: nself-org has branch protection on main (1 required reviewer). A
# solo operator cannot self-merge a PR under those rules. This script:
#   1. Pre-flight: verifies git state, checks for in-flight operations, reads
#      the target branch from the first argument or interactive prompt.
#   2. Saves the FULL protection JSON it GETs before relaxing (the snapshot) to
#      a 0600 file, then launches an independent watchdog process (nohup,
#      separate PID) that will RESTORE that snapshot even if the parent shell
#      is killed.
#   3. Relaxes branch protection on the target repo/branch via GitHub API.
#   4. Merges the PR (by number), then reads its state back: it must be MERGED
#      within 60 s. `gh pr merge --auto` can exit 0 having only enabled
#      auto-merge; that, a CLOSED PR or a failed merge disables auto-merge
#      again, skips the CI wait, restores protection and exits 1.
#   5. Waits for CI to pass (up to WAIT_MINUTES, default 20).
#   6. Restores branch protection from the snapshot: every field the PUT API
#      accepts (scripts/lib/protection_snapshot.sh), then GETs it again and
#      compares. Any difference is printed, audited as `restore_mismatch` and
#      the script exits 3; it never reports success on an unverified restore.
#   7. Writes an audit log entry to ~/.nself/admin-merge-audit.log.
#
# The watchdog is an independent bash process (not a subshell) so it cannot
# be killed by SIGKILL to the parent. It sources the library by absolute path,
# reads the snapshot file, polls for the "done" marker and, if the marker never
# appears within WATCHDOG_DEADLINE, runs the same verified restore (exit 3 on
# mismatch). The done marker is written only after a verified restore.
#
# Usage:
#   scripts/admin-merge.sh [--repo <owner/repo>] [--branch <branch>]
#                          [--pr <number>] [--dry-run] [--no-wait-ci]
#                          [--wait-minutes <N>]
#
# Flags:
#   --repo <owner/repo>    Target repo (default: auto-detected from git remote)
#   --branch <branch>      Branch to merge (default: current branch)
#   --pr <number>          PR number to merge (auto-detected from branch if omitted)
#   --dry-run              Print what would happen (incl. the restore PUT body); change nothing
#   --no-wait-ci           Skip CI wait; restore immediately after merge (risky)
#   --wait-minutes <N>     CI wait budget in minutes (default: 20)
#   --force-restore        Restore branch protection only (recovery mode)
#   --snapshot <file>      With --force-restore: snapshot file to restore (its
#                          path is in the RESTORE_FAILED / restore_mismatch audit
#                          line). Without it the LIVE protection is re-applied,
#                          which is only meaningful when it is not relaxed now.
#
# Exit codes: 0 merged and restored; 1 usage or pre-flight error, or the PR did
#   not reach MERGED (protection is restored and verified first); 3 restore failed or not
#   verified (protection may be weaker than before: run --force-restore).
#
# Requirements:
#   gh (GitHub CLI, authenticated with admin scope)
#   jq
#   git
#
# Audit log: ~/.nself/admin-merge-audit.log
# Watchdog state: ${TMPDIR:-/tmp}/nself-admin-merge-<ts>.{done,relaxed,relax_ts,snapshot.json}
#
# Security note: relaxing branch protection for <60 seconds is a carefully
# bounded window. The watchdog ensures protection is ALWAYS restored even on
# SIGKILL. Do NOT expand RELAX_WINDOW_SECONDS beyond 120. WATCHDOG_DEADLINE may
# be lowered through the environment (tests) but never raised above its default.
#
# Authority: P98 S98-02 T09; admin-merge-workflow.md; P7-HYG-40

set -euo pipefail

# ── Constants ─────────────────────────────────────────────────────────────────
# RELAX_WINDOW_SECONDS: hard cap on how long protection stays off.
# WATCHDOG_DEADLINE >= 3x RELAX_WINDOW_SECONDS to allow CI startup overhead.
RELAX_WINDOW_SECONDS=60
WATCHDOG_MAX=$(( RELAX_WINDOW_SECONDS * 3 ))  # 180s
case "${WATCHDOG_DEADLINE:-}" in
  ''|*[!0-9]*) WATCHDOG_DEADLINE="${WATCHDOG_MAX}" ;;
  *) [ "${WATCHDOG_DEADLINE}" -le "${WATCHDOG_MAX}" ] || WATCHDOG_DEADLINE="${WATCHDOG_MAX}" ;;
esac
DEFAULT_WAIT_MINUTES=20
CI_POLL_SECONDS="${CI_POLL_SECONDS:-30}"
# After `gh pr merge` the PR must read state MERGED within MERGE_VERIFY_SECONDS
# (polled every MERGE_POLL_SECONDS); both may be lowered through the environment.
# Bash 3.2 exits 0 when arithmetic on a non-number fails with an EXIT trap armed,
# so both are checked here (digits only, poll >= 1, verify capped at the relax window).
case "${MERGE_VERIFY_SECONDS:-}" in
  ''|*[!0-9]*) MERGE_VERIFY_SECONDS="${RELAX_WINDOW_SECONDS}" ;;
  *) [ "${MERGE_VERIFY_SECONDS}" -le "${RELAX_WINDOW_SECONDS}" ] || MERGE_VERIFY_SECONDS="${RELAX_WINDOW_SECONDS}" ;;
esac
MERGE_VERIFY_SECONDS=$((10#${MERGE_VERIFY_SECONDS}))
case "${MERGE_POLL_SECONDS:-}" in
  ''|*[!0-9]*|0|00*) MERGE_POLL_SECONDS=5 ;;
esac
MERGE_POLL_SECONDS=$((10#${MERGE_POLL_SECONDS}))
AUDIT_LOG="${HOME}/.nself/admin-merge-audit.log"
export AUDIT_LOG
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PS_LIB="${SCRIPT_DIR}/lib/protection_snapshot.sh"
WORK_DIR="${TMPDIR:-/tmp}"

source "${SCRIPT_DIR}/lib/admin_merge_preflight.sh"

# ── Spawn watchdog sidecar ────────────────────────────────────────────────────
# The watchdog is an INDEPENDENT process (nohup + detached).
# It cannot be killed by SIGKILL to this shell.
# It polls for DONE_MARKER and runs the verified restore if it never appears.
# The script body is a quoted heredoc: no parent state is interpolated; every
# value arrives as an argument and the restore logic is sourced from PS_LIB.

WATCHDOG_SCRIPT="${WORK_DIR}/nself-watchdog-${MERGE_TS}.sh"

if [ "${DRY_RUN}" -eq 0 ]; then
  cat > "${WATCHDOG_SCRIPT}" << 'WATCHDOG_EOF'
#!/usr/bin/env bash
# Watchdog for admin-merge.sh. Restores branch protection from the snapshot
# file if the parent dies before it writes the done marker.
# Args: lib repo snapshot done relaxed relax_ts deadline audit_log dry_run self parent_pid
set -u
LIB="$1"; REPO="$2"; SNAP="$3"; DONE_MARKER="$4"; RELAX_MARKER="$5"
RELAX_TS_MARKER="$6"; DEADLINE="$7"; AUDIT_LOG="$8"; DRY_RUN="$9"; SELF="${10}"; PARENT="${11}"
export AUDIT_LOG
# The parent verifies its own restore from the same snapshot file, so it is
# removed only once the parent is gone (a slow CI must not lose it).
drop_snapshot() { kill -0 "${PARENT}" 2>/dev/null || rm -f "${SNAP}"; }
drop_markers() { rm -f "${SELF}" "${RELAX_MARKER}" "${RELAX_TS_MARKER}"; }
# shellcheck source=/dev/null
. "${LIB}" || { printf '[WATCHDOG] cannot source %s\n' "${LIB}"; exit 2; }

deadline=$(( $(date '+%s') + DEADLINE ))

# Wait until relax marker exists (protection has been relaxed) or deadline
while [ ! -f "${RELAX_MARKER}" ]; do
  if [ "$(date '+%s')" -ge "${deadline}" ]; then
    printf '[WATCHDOG] Deadline reached waiting for relax marker. Exiting (nothing to restore).\n'
    drop_snapshot; drop_markers
    exit 0
  fi
  sleep 2
done

# FIX3: reset the deadline from the actual relax completion time.
if [ -f "${RELAX_TS_MARKER}" ]; then
  relax_epoch=$(cat "${RELAX_TS_MARKER}" 2>/dev/null || date '+%s')
  deadline=$(( relax_epoch + DEADLINE ))
  printf '[WATCHDOG] Deadline reset from relax time: epoch=%s deadline=%s\n' "${relax_epoch}" "${deadline}"
fi
printf '[WATCHDOG] Protection was relaxed. Monitoring for done marker...\n'

while [ ! -f "${DONE_MARKER}" ]; do
  if [ "$(date '+%s')" -ge "${deadline}" ]; then
    printf '[WATCHDOG] Deadline exceeded without done marker. Forcing restore.\n'
    rc=0
    restore_protection "${REPO}" "${SNAP}" "${DRY_RUN}" watchdog_timeout || rc=$?
    if [ "${rc}" -eq 0 ]; then drop_snapshot; fi
    if ! kill -0 "${PARENT}" 2>/dev/null; then drop_markers; fi
    printf '[WATCHDOG] exit %s\n' "${rc}"
    exit "${rc}"
  fi
  sleep 3
done

printf '[WATCHDOG] Done marker found. Parent completed cleanly — no restore needed.\n'
drop_markers
exit 0
WATCHDOG_EOF
  chmod +x "${WATCHDOG_SCRIPT}"
  nohup bash "${WATCHDOG_SCRIPT}" "${PS_LIB}" "${TARGET_REPO}" "${SNAPSHOT_FILE}" \
    "${DONE_MARKER}" "${RELAX_MARKER}" "${RELAX_TS_MARKER}" "${WATCHDOG_DEADLINE}" \
    "${AUDIT_LOG}" "${DRY_RUN}" "${WATCHDOG_SCRIPT}" "$$" > "${WORK_DIR}/nself-watchdog-${MERGE_TS}.log" 2>&1 &
  WATCHDOG_PID=$!
  info "Watchdog spawned: PID ${WATCHDOG_PID}, deadline ${WATCHDOG_DEADLINE}s"
  audit "WATCHDOG_SPAWN  pid=${WATCHDOG_PID}  deadline=${WATCHDOG_DEADLINE}  snapshot=${SNAPSHOT_FILE}"
else
  info "[DRY-RUN] Would spawn watchdog (skipped)"
fi

# FIX5: EXIT trap — on any exit (clean, error, SIGINT, SIGTERM) a relaxed branch
# is restored from the snapshot. The done marker is written only after a
# VERIFIED restore (or when nothing was relaxed); a failed restore leaves it
# absent so the watchdog still runs its own restore, and exits 3 for the owner.
_exit_trap_fired=0
_cleanup_on_exit() {
  local rc=$?   # bash 3.2 reports 0 here after a fatal set -u/arithmetic error; keep what we got
  if [ "${_exit_trap_fired}" -eq 1 ]; then return; fi
  _exit_trap_fired=1
  if [ -f "${RELAX_MARKER}" ] && [ ! -f "${DONE_MARKER}" ]; then
    if restore_protection "${TARGET_REPO}" "${SNAPSHOT_FILE}" "${DRY_RUN}" "exit_trap"; then
      touch "${DONE_MARKER}" 2>/dev/null || true
      rm -f "${SNAPSHOT_FILE}"
    else
      error "Branch protection NOT verified restored. Snapshot: ${SNAPSHOT_FILE}. Run: $0 --repo ${TARGET_REPO} --force-restore --snapshot ${SNAPSHOT_FILE}"
      exit 3
    fi
  elif [ ! -f "${RELAX_MARKER}" ]; then
    touch "${DONE_MARKER}" 2>/dev/null || true
  fi
  exit "${rc}"
}
trap '_cleanup_on_exit' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# ── Relax branch protection ───────────────────────────────────────────────────
info "Relaxing branch protection on ${TARGET_REPO}/main..."

RELAX_PAYLOAD=$(build_relax_body "${PROTECTION_JSON}")

if [ "${DRY_RUN}" -eq 0 ]; then
  # Arm the restore BEFORE the PUT: if the PUT lands but gh reports failure
  # (timeout, interrupt), the trap and watchdog still restore. Restore is
  # idempotent, so arming a PUT that never landed is harmless.
  touch "${RELAX_MARKER}"
  date '+%s' > "${RELAX_TS_MARKER}"  # watchdog deadline runs from here (FIX3)
  # gh api does not support -w or --timeout flags; use exit code to determine success
  if gh api "repos/${TARGET_REPO}/branches/main/protection" \
    -X PUT \
    --input - \
    -H "Accept: application/vnd.github.v3+json" \
    <<< "${RELAX_PAYLOAD}" > /dev/null 2>&1; then
    ok "Branch protection relaxed."
    audit "RELAX  repo=${TARGET_REPO}"
  else
    warn "Relax PUT did not confirm; restoring from the snapshot in case it landed."
    restore_protection "${TARGET_REPO}" "${SNAPSHOT_FILE}" 0 "relax_failed" || exit 3
    touch "${DONE_MARKER}"
    rm -f "${SNAPSHOT_FILE}"
    die "Failed to relax branch protection. Protection verified identical to the snapshot."
  fi
else
  info "[DRY-RUN] Would relax branch protection via PUT."
  info "[DRY-RUN] Payload: ${RELAX_PAYLOAD}"
fi

source "${SCRIPT_DIR}/lib/admin_merge_pr.sh"

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
