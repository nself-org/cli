#!/usr/bin/env bash
# Sourced preflight and snapshot setup for admin-merge.sh.
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
