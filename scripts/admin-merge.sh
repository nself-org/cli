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
#   4. Merges the PR (by number) or the current branch directly.
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
# Exit codes: 0 ok; 1 usage or pre-flight error; 3 restore failed or not
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
AUDIT_LOG="${HOME}/.nself/admin-merge-audit.log"
export AUDIT_LOG
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PS_LIB="${SCRIPT_DIR}/lib/protection_snapshot.sh"
WORK_DIR="${TMPDIR:-/tmp}"

# ── Arguments ─────────────────────────────────────────────────────────────────
DRY_RUN=0
NO_WAIT_CI=0
FORCE_RESTORE=0
WAIT_MINUTES="${DEFAULT_WAIT_MINUTES}"
TARGET_REPO=""
TARGET_BRANCH=""
PR_NUMBER=""
SNAPSHOT_FILE=""

while [ $# -gt 0 ]; do
  case "$1" in
    --repo)           TARGET_REPO="$2";    shift 2 ;;
    --branch)         TARGET_BRANCH="$2";  shift 2 ;;
    --pr)             PR_NUMBER="$2";      shift 2 ;;
    --dry-run)        DRY_RUN=1;           shift ;;
    --no-wait-ci)     NO_WAIT_CI=1;        shift ;;
    --force-restore)  FORCE_RESTORE=1;     shift ;;
    --snapshot)       SNAPSHOT_FILE="$2";  shift 2 ;;
    --wait-minutes)   WAIT_MINUTES="$2";   shift 2 ;;
    --help|-h)
      sed -n '2,/^set -euo pipefail/p' "$0" | grep '^#' | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      printf 'Unknown flag: %s\n' "$1" >&2
      exit 1
      ;;
  esac
done

# ── Utilities ─────────────────────────────────────────────────────────────────
TS() { date '+%Y-%m-%dT%H:%M:%S%z'; }
BOLD='\033[1m'
RED='\033[31m'
YELLOW='\033[33m'
GREEN='\033[32m'
RESET='\033[0m'

info()  { printf '%b[INFO]%b  %s\n' "${BOLD}" "${RESET}" "$*"; }
warn()  { printf '%b[WARN]%b  %s\n' "${YELLOW}" "${RESET}" "$*"; }
error() { printf '%b[ERROR]%b %s\n' "${RED}" "${RESET}" "$*" >&2; }
ok()    { printf '%b[OK]%b    %s\n' "${GREEN}" "${RESET}" "$*"; }

audit() {
  mkdir -p "$(dirname "${AUDIT_LOG}")"
  printf '%s  %s\n' "$(TS)" "$*" >> "${AUDIT_LOG}"
}

die() {
  error "$*"
  audit "ABORT: $*"
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "Required command not found: $1"
}

# ── Pre-flight checks ─────────────────────────────────────────────────────────
require_cmd gh
require_cmd jq
require_cmd git
[ -r "${PS_LIB}" ] || die "Missing library: ${PS_LIB}"
# shellcheck source=/dev/null
. "${PS_LIB}"

# Auto-detect repo from git remote
if [ -z "${TARGET_REPO}" ]; then
  REMOTE_URL=$(git remote get-url origin 2>/dev/null || true)
  if [ -z "${REMOTE_URL}" ]; then
    die "Cannot detect repo: no git remote 'origin'. Pass --repo owner/repo."
  fi
  # Extract owner/repo from https://github.com/owner/repo.git or git@github.com:owner/repo.git
  TARGET_REPO=$(printf '%s' "${REMOTE_URL}" \
    | sed 's|.*github\.com[:/]\(.*\)\.git|\1|' \
    | sed 's|.*github\.com[:/]\(.*\)|\1|')
fi

# Auto-detect branch and guard against merging from main. --force-restore only
# restores protection, so it needs neither (recovery must work from any checkout).
if [ "${FORCE_RESTORE}" -eq 0 ] && [ -z "${TARGET_BRANCH}" ]; then
  TARGET_BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)
  if [ -z "${TARGET_BRANCH}" ] || [ "${TARGET_BRANCH}" = "HEAD" ]; then
    die "Cannot detect current branch. Pass --branch <branch>."
  fi
fi

# Guard: never merge directly on main
if [ "${FORCE_RESTORE}" -eq 0 ] && { [ "${TARGET_BRANCH}" = "main" ] || [ "${TARGET_BRANCH}" = "master" ]; }; then
  die "TARGET_BRANCH is '${TARGET_BRANCH}' — this script merges TO main, not FROM main. Pass a feature/fix branch via --branch."
fi

# Verify gh auth
if ! gh auth status >/dev/null 2>&1; then
  die "GitHub CLI not authenticated. Run: gh auth login"
fi

# Verify admin scope (branch protection requires admin:repo or repo scope)
SCOPES=$(gh auth status 2>&1 | grep -i 'scopes\|token scopes' | head -1 || true)
if ! printf '%s' "${SCOPES}" | grep -qi 'repo'; then
  warn "Could not confirm 'repo' scope on GH token. Branch protection API may fail."
  warn "Re-authenticate with: gh auth login --scopes repo"
fi

# Auto-detect PR number
if [ -z "${PR_NUMBER}" ] && [ "${FORCE_RESTORE}" -eq 0 ]; then
  PR_NUMBER=$(gh pr list --repo "${TARGET_REPO}" --head "${TARGET_BRANCH}" --state open --json number --jq '.[0].number' 2>/dev/null || true)
  if [ -z "${PR_NUMBER}" ]; then
    die "No open PR found for branch '${TARGET_BRANCH}' in ${TARGET_REPO}. Create one first or pass --pr <number>."
  fi
  info "Auto-detected PR #${PR_NUMBER} for branch '${TARGET_BRANCH}'"
fi

# ── Read current branch protection (the snapshot restored after the merge) ────
info "Reading current branch protection for ${TARGET_REPO}..."

PROTECTION_JSON=$(gh api "repos/${TARGET_REPO}/branches/main/protection" 2>/dev/null || true)
if [ -z "${PROTECTION_JSON}" ]; then
  die "Cannot read branch protection for ${TARGET_REPO}/main. Ensure GH token has admin:repo scope."
fi
printf '%s' "${PROTECTION_JSON}" | jq -e 'type == "object"' >/dev/null 2>&1 \
  || die "Branch protection response for ${TARGET_REPO}/main is not a JSON object."
# Refuse before relaxing anything if the PUT API cannot reproduce the snapshot.
ps_unexpressible "${PROTECTION_JSON}" || die "Cannot restore this protection faithfully; owner decision needed. Nothing was changed."

info "  Snapshot: $(printf '%s' "${PROTECTION_JSON}" | jq -c '{reviews: .required_pull_request_reviews.required_approving_review_count, enforce_admins: .enforce_admins.enabled, strict: .required_status_checks.strict, checks: [.required_status_checks.contexts[]?]}')"

# ── Watchdog marker files ─────────────────────────────────────────────────────
MERGE_TS=$(date '+%Y%m%d%H%M%S')
DONE_MARKER="${WORK_DIR}/nself-admin-merge-${MERGE_TS}.done"
RELAX_MARKER="${WORK_DIR}/nself-admin-merge-${MERGE_TS}.relaxed"
# RELAX_TS_MARKER: written after relax API call succeeds; contains the epoch
# at which relax completed. Watchdog reads this to reset its deadline from the
# actual relax time rather than from spawn time (FIX3 — watchdog deadline timing).
RELAX_TS_MARKER="${WORK_DIR}/nself-admin-merge-${MERGE_TS}.relax_ts"

# Write the snapshot (0600) to the file restore_protection and the watchdog read.
write_snapshot() { ( umask 077; printf '%s' "${PROTECTION_JSON}" > "$1" ); }

# ── Force-restore early exit ──────────────────────────────────────────────────
if [ "${FORCE_RESTORE}" -eq 1 ]; then
  warn "Force-restore mode: restoring branch protection without merge."
  if [ -z "${SNAPSHOT_FILE}" ]; then
    warn "No --snapshot given: re-applying the LIVE protection (a no-op if it is currently relaxed)."
    SNAPSHOT_FILE="${WORK_DIR}/nself-admin-merge-${MERGE_TS}.snapshot.json"
    write_snapshot "${SNAPSHOT_FILE}"
  fi
  restore_protection "${TARGET_REPO}" "${SNAPSHOT_FILE}" "${DRY_RUN}" "force_restore" || exit 3
  exit 0
fi

# ── Pre-merge summary ─────────────────────────────────────────────────────────
printf '\n'
info "Admin merge plan:"
printf '  Repo:         %s\n' "${TARGET_REPO}"
printf '  Branch:       %s → main\n' "${TARGET_BRANCH}"
printf '  PR:           #%s\n' "${PR_NUMBER}"
printf '  CI wait:      %s minutes\n' "${WAIT_MINUTES}"
printf '  Dry run:      %s\n' "${DRY_RUN}"
printf '  Watchdog:     %ss deadline (%s marker)\n' "${WATCHDOG_DEADLINE}" "${DONE_MARKER}"
printf '\n'

if [ "${DRY_RUN}" -eq 0 ]; then
  warn "This will TEMPORARILY relax branch protection on ${TARGET_REPO}/main."
  warn "Protection is restored automatically within ${WATCHDOG_DEADLINE}s by the watchdog."
  printf '\nProceed? [y/N] '
  read -r CONFIRM
  if [ "${CONFIRM}" != "y" ] && [ "${CONFIRM}" != "Y" ]; then
    info "Aborted."
    exit 0
  fi
fi

audit "START  repo=${TARGET_REPO}  branch=${TARGET_BRANCH}  pr=${PR_NUMBER}  dry_run=${DRY_RUN}"
SNAPSHOT_FILE="${WORK_DIR}/nself-admin-merge-${MERGE_TS}.snapshot.json"
write_snapshot "${SNAPSHOT_FILE}"

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

# ── Merge PR ──────────────────────────────────────────────────────────────────
MERGE_SUCCESS=0
if [ "${DRY_RUN}" -eq 0 ]; then
  info "Merging PR #${PR_NUMBER}..."
  if gh pr merge "${PR_NUMBER}" \
      --repo "${TARGET_REPO}" \
      --squash \
      --delete-branch \
      --auto 2>&1; then
    MERGE_SUCCESS=1
    ok "PR #${PR_NUMBER} merged."
    audit "MERGE  pr=${PR_NUMBER}  result=success"
  else
    error "PR merge failed."
    audit "MERGE  pr=${PR_NUMBER}  result=failed"
    # Still restore even on failure
  fi
else
  info "[DRY-RUN] Would merge PR #${PR_NUMBER} (squash + delete branch)."
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
audit "DONE  repo=${TARGET_REPO}  pr=${PR_NUMBER}"

ok "Admin merge complete. Branch protection restored and verified. Watchdog will self-exit."

printf '\n'
info "Audit log: ${AUDIT_LOG}"
if [ "${DRY_RUN}" -eq 0 ]; then
  info "Watchdog log: ${WORK_DIR}/nself-watchdog-${MERGE_TS}.log"
fi
