#!/usr/bin/env bash
# release-create.sh
#
# Wrapper around `gh release create` that enforces the release denylist
# defined in .github/release-denylist.json (P101 S8.T05).
#
# Usage:
#   bash scripts/release-create.sh <tag> [gh-release-args...]
#   bash scripts/release-create.sh <tag> --skip-denylist [gh-release-args...]
#
# Examples:
#   bash scripts/release-create.sh v1.0.13 --generate-notes
#   bash scripts/release-create.sh v1.0.14 --skip-denylist --notes-file RELEASE_NOTES.md
#
# Exits non-zero on any denylist hit unless --skip-denylist is supplied.
# When --skip-denylist is supplied, prompts interactively for a justification
# and appends an audit record to audit/release-overrides.log before proceeding.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DENYLIST_FILE="${REPO_ROOT}/.github/release-denylist.json"
AUDIT_DIR="${REPO_ROOT}/audit"
AUDIT_LOG="${AUDIT_DIR}/release-overrides.log"

# ── Argument parsing ─────────────────────────────────────────────────────────
if [ $# -lt 1 ]; then
  echo "ERROR: usage: $0 <tag> [--skip-denylist] [gh-release-args...]" >&2
  exit 2
fi

TAG="$1"
shift

SKIP_DENYLIST=0
PASSTHROUGH_ARGS=()
for arg in "$@"; do
  if [ "${arg}" = "--skip-denylist" ]; then
    SKIP_DENYLIST=1
  else
    PASSTHROUGH_ARGS+=("${arg}")
  fi
done

if [ ! -f "${DENYLIST_FILE}" ]; then
  echo "ERROR: denylist file not found: ${DENYLIST_FILE}" >&2
  exit 2
fi

# ── Helpers ──────────────────────────────────────────────────────────────────
log_info() { printf "[release-create] %s\n" "$*"; }
log_warn() { printf "[release-create] WARN: %s\n" "$*" >&2; }
log_fail() { printf "[release-create] FAIL: %s\n" "$*" >&2; }

# Returns 0 if rule passes, 1 if it triggers (block).
check_rule_signature() {
  # DENY-001: signature-check. Look for any *.cosign.bundle in dist/. The bundles exist only
  # after release.yml builds, so at tag time (no dist/) the rule is skipped; the Pre-Release
  # Gate in release.yml enforces it post-build (D-0727).
  if [ ! -d "${REPO_ROOT}/dist" ]; then
    log_info "no dist/ yet; DENY-001 is enforced post-build by release.yml (skipped)"
    return 0
  fi
  if [ -d "${REPO_ROOT}/dist" ] && ls "${REPO_ROOT}/dist"/*.cosign.bundle >/dev/null 2>&1; then
    return 0
  fi
  return 1
}

check_rule_sbom() {
  # DENY-002: missing-sbom. Require both SPDX + CycloneDX in dist/ or repo root. Like DENY-001
  # it needs the build output, so it is skipped when there is no dist/ (D-0727).
  if [ ! -d "${REPO_ROOT}/dist" ]; then
    log_info "no dist/ yet; DENY-002 is enforced post-build by release.yml (skipped)"
    return 0
  fi
  for f in sbom.spdx.json sbom.cdx.json; do
    if [ ! -f "${REPO_ROOT}/dist/${f}" ] && [ ! -f "${REPO_ROOT}/${f}" ]; then
      return 1
    fi
  done
  return 0
}

check_rule_commits_since_prior() {
  # DENY-003: zero-commits-since-prior-tag.
  local prior_tag
  prior_tag=$(git -C "${REPO_ROOT}" tag --sort=-version:refname | grep -v "^${TAG}$" | head -1 || true)
  if [ -z "${prior_tag}" ]; then
    log_info "no prior tag found; treating as first release (PASS)"
    return 0
  fi
  local count
  count=$(git -C "${REPO_ROOT}" rev-list "${prior_tag}..HEAD" --count 2>/dev/null || echo "0")
  if [ "${count}" -gt 0 ]; then
    return 0
  fi
  return 1
}

check_rule_license_scan() {
  # DENY-004: failed-license-scan. Query the latest license-audit.yml run on main (a run on
  # another branch says nothing about the release).
  if ! command -v gh >/dev/null 2>&1; then
    log_warn "gh CLI not found; cannot verify license-audit. Treating as PASS (CI gate handles it)."
    return 0
  fi
  local conclusion
  conclusion=$(gh run list --workflow license-audit.yml --branch main --limit 1 --json conclusion -q '.[0].conclusion' 2>/dev/null || echo "")
  if [ "${conclusion}" = "success" ]; then
    return 0
  fi
  return 1
}

check_rule_milestone_closed() {
  # DENY-005: no-milestone-closed.
  if ! command -v gh >/dev/null 2>&1; then
    log_warn "gh CLI not found; cannot verify milestone. Treating as PASS (CI gate handles it)."
    return 0
  fi
  local milestone_state
  milestone_state=$(gh api "repos/{owner}/{repo}/milestones?state=closed" \
    --jq ".[] | select(.title==\"${TAG}\") | .state" 2>/dev/null || echo "")
  if [ "${milestone_state}" = "closed" ]; then
    return 0
  fi
  return 1
}

# ── Run all rules ────────────────────────────────────────────────────────────
TRIGGERED_RULES=()

log_info "evaluating denylist for tag: ${TAG}"

if ! check_rule_signature; then TRIGGERED_RULES+=("DENY-001 signature-check"); fi
if ! check_rule_sbom; then TRIGGERED_RULES+=("DENY-002 missing-sbom"); fi
if ! check_rule_commits_since_prior; then TRIGGERED_RULES+=("DENY-003 zero-commits-since-prior-tag"); fi
if ! check_rule_license_scan; then TRIGGERED_RULES+=("DENY-004 failed-license-scan"); fi
if ! check_rule_milestone_closed; then TRIGGERED_RULES+=("DENY-005 no-milestone-closed"); fi

if [ ${#TRIGGERED_RULES[@]} -eq 0 ]; then
  log_info "denylist: PASS (0 rules triggered)"
else
  log_fail "denylist: ${#TRIGGERED_RULES[@]} rule(s) triggered"
  for r in "${TRIGGERED_RULES[@]}"; do
    printf "  - %s\n" "${r}" >&2
  done

  if [ "${SKIP_DENYLIST}" -ne 1 ]; then
    log_fail "release blocked. Re-run with --skip-denylist + justification to override."
    exit 1
  fi

  # ── Override path ──────────────────────────────────────────────────────────
  log_warn "--skip-denylist provided. Interactive confirmation required."
  if [ -t 0 ]; then
    printf "Type the word OVERRIDE to confirm: " >&2
    read -r CONFIRM
    if [ "${CONFIRM}" != "OVERRIDE" ]; then
      log_fail "override aborted by operator"
      exit 1
    fi
    printf "Reason for override (one line, will be logged): " >&2
    read -r REASON
    if [ -z "${REASON}" ]; then
      log_fail "override reason required"
      exit 1
    fi
  else
    REASON="${RELEASE_OVERRIDE_REASON:-}"
    if [ -z "${REASON}" ]; then
      log_fail "non-interactive override requires RELEASE_OVERRIDE_REASON env var"
      exit 1
    fi
  fi

  mkdir -p "${AUDIT_DIR}"
  TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  GIT_SHA=$(git -C "${REPO_ROOT}" rev-parse HEAD)
  OPERATOR="${GITHUB_ACTOR:-${USER:-unknown}}"
  RULE_IDS=$(printf "%s," "${TRIGGERED_RULES[@]}" | sed 's/,$//')

  printf "%s | sha=%s | tag=%s | operator=%s | skipped_rules=[%s] | reason=%s\n" \
    "${TIMESTAMP}" "${GIT_SHA}" "${TAG}" "${OPERATOR}" "${RULE_IDS}" "${REASON}" \
    >> "${AUDIT_LOG}"
  log_warn "override logged to ${AUDIT_LOG}"
fi

# ── Invoke gh release create ─────────────────────────────────────────────────
log_info "invoking: gh release create ${TAG} ${PASSTHROUGH_ARGS[*]:-}"
exec gh release create "${TAG}" "${PASSTHROUGH_ARGS[@]}"
