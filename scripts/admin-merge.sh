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
#                          [--admin-bypass-pending hosted-runner-backlog]
#
# Flags:
#   --repo <owner/repo>    Target repo (default: auto-detected from git remote)
#   --branch <branch>      Branch to merge (default: current branch)
#   --pr <number>          PR number to merge (auto-detected from branch if omitted)
#   --dry-run              Print what would happen (incl. the restore PUT body); change nothing
#   --no-wait-ci           Skip CI wait; restore immediately after merge (risky)
#   --wait-minutes <N>     CI wait budget in minutes (default: 20)
#   --admin-bypass-pending hosted-runner-backlog
#                          Merge pending required checks only for this named backlog; failures are refused.
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
case "${MERGE_POLL_SECONDS:-}" in
  ''|*[!0-9]*|0|00*) MERGE_POLL_SECONDS=5 ;;
esac
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
ADMIN_BYPASS_CASE=""
MERGE_METHOD=--auto

while [ $# -gt 0 ]; do
  case "$1" in
    --repo)           TARGET_REPO="$2";    shift 2 ;;
    --branch)         TARGET_BRANCH="$2";  shift 2 ;;
    --pr)             PR_NUMBER="$2";      shift 2 ;;
    --dry-run)        DRY_RUN=1;           shift ;;
    --no-wait-ci)     NO_WAIT_CI=1;        shift ;;
    --admin-bypass-pending)
      [ $# -ge 2 ] || { printf 'ERROR: --admin-bypass-pending needs hosted-runner-backlog\n' >&2; exit 1; }
      ADMIN_BYPASS_CASE="$2"; shift 2 ;;
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

case "$ADMIN_BYPASS_CASE" in
  ''|hosted-runner-backlog) ;;
  *) printf 'ERROR: unknown pending-check backlog case: %s\n' "$ADMIN_BYPASS_CASE" >&2; exit 1 ;;
esac

case "${WAIT_MINUTES}" in
  ''|*[!0-9]*) printf 'ERROR: --wait-minutes needs a whole number, got: %s\n' "${WAIT_MINUTES}" >&2; exit 1 ;;
esac

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

. "${SCRIPT_DIR}/lib/admin_merge_prepare.sh"
. "${SCRIPT_DIR}/lib/admin_merge_watchdog.sh"

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

. "${SCRIPT_DIR}/lib/admin_merge_finish.sh"
