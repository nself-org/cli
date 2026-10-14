#!/usr/bin/env bash
# branch-protection-toggle.sh
#
# Idempotent branch-protection toggle for nself-org repos.
# Generalizes the inline gh-api protection toggling previously scattered
# across release scripts (admin-merge.sh, etc.) into a single canonical
# entry point with audit log, dry-run, multi-repo batch, and flap-guard.
#
# Authority: P101 S8.T01 (V8-02)
#
# Usage:
#   scripts/branch-protection-toggle.sh --on  --repo nself-org/cli
#   scripts/branch-protection-toggle.sh --off --repo nself-org/cli --branch main
#   scripts/branch-protection-toggle.sh --on  --repo nself-org/cli --dry-run
#   scripts/branch-protection-toggle.sh --on  --repo nself-org/cli --repo nself-org/admin
#   scripts/branch-protection-toggle.sh --on  --repos-file repos.txt
#   scripts/branch-protection-toggle.sh --rollback --repo nself-org/cli   # restore from last state
#
# Flags:
#   --on                       Apply the canonical baseline (from policy YAML).
#   --off                      Remove branch protection entirely.
#   --repo <owner/repo>        Target repo (repeatable for batch mode).
#   --repos-file <path>        Newline-delimited file of owner/repo entries.
#   --branch <name>            Target branch (default: main).
#   --dry-run                  Print intended changes, make no API calls.
#   --force                    Bypass flap-guard (rapid-toggle <5min).
#   --rollback                 Restore previous state from state file.
#   --policy <path>            Override default policy YAML.
#   --reason <text>            Audit-log reason (recommended with --force).
#   -h, --help                 This help.
#
# Exit codes:
#   0  success, no-op, or n/a (repo has no default branch)
#   1  usage error
#   2  gh API error or missing dependency
#   3  flap-guard tripped (rapid toggle without --force)
#
# Requirements: gh (authenticated, admin:repo scope), jq.
#               yq preferred for YAML; falls back to python3+PyYAML.
#
# Files:
#   cli/scripts/.policy/branch-protection.yaml   canonical baseline
#   ~/.nself/branch-protection-audit.log         append-only audit trail
#   ~/.nself/branch-protection-state.json        flap-guard timestamps
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_POLICY="${SCRIPT_DIR}/.policy/branch-protection.yaml"
AUDIT_LOG="${HOME}/.nself/branch-protection-audit.log"
STATE_FILE="${HOME}/.nself/branch-protection-state.json"
FLAP_WINDOW_SECONDS=300  # 5 minutes
ACTION="" REPOS_FILE="" BRANCH="main" REASON=""
DRY_RUN=0 FORCE=0 ROLLBACK=0 POLICY_FILE="${DEFAULT_POLICY}"
REPOS=()
usage() {
  sed -n '1,40p' "${BASH_SOURCE[0]}" | sed -n '/^# Usage:/,/^# Exit codes:/p'
  exit "${1:-0}"
}
while [ $# -gt 0 ]; do
  case "$1" in
    --on)         ACTION="on"; shift ;;
    --off)        ACTION="off"; shift ;;
    --repo)       REPOS+=("$2"); shift 2 ;;
    --repos-file) REPOS_FILE="$2"; shift 2 ;;
    --branch)     BRANCH="$2"; shift 2 ;;
    --dry-run)    DRY_RUN=1; shift ;;
    --force)      FORCE=1; shift ;;
    --rollback)   ROLLBACK=1; shift ;;
    --policy)     POLICY_FILE="$2"; shift 2 ;;
    --reason)     REASON="$2"; shift 2 ;;
    -h|--help)    usage 0 ;;
    *) printf 'Unknown flag: %s\n' "$1" >&2; usage 1 ;;
  esac
done
TS() { date -u +'%Y-%m-%dT%H:%M:%SZ'; }
EPOCH() { date -u +'%s'; }
info()  { printf '[info]  %s\n' "$*"; }
warn()  { printf '[warn]  %s\n' "$*" >&2; }
err()   { printf '[error] %s\n' "$*" >&2; }
die()   { err "$*"; exit 2; }
audit() {
  # ISO8601 | actor | repo | branch | action | dry_run | result | reason
  local repo="$1" branch="$2" action="$3" result="$4" reason="$5" actor
  actor="$(gh api user --jq .login 2>/dev/null || printf 'unknown')"
  mkdir -p "$(dirname "${AUDIT_LOG}")"
  printf '%s | %s | %s | %s | %s | dry_run=%s | %s | %s\n' \
    "$(TS)" "${actor}" "${repo}" "${branch}" "${action}" "${DRY_RUN}" "${result}" "${reason}" \
    >> "${AUDIT_LOG}"
}
need_cmd() { command -v "$1" >/dev/null 2>&1 || die "Required tool not found: $1"; }
if [ "${ROLLBACK}" -eq 1 ]; then
  [ -n "${ACTION}" ] && { err "--rollback is mutually exclusive with --on/--off"; usage 1; }
elif [ -z "${ACTION}" ]; then
  err "Must specify --on, --off, or --rollback"; usage 1
fi
if [ -n "${REPOS_FILE}" ]; then
  [ -r "${REPOS_FILE}" ] || die "Cannot read repos file: ${REPOS_FILE}"
  while IFS= read -r line; do
    line="${line%%#*}"
    line="$(printf '%s' "${line}" | tr -d '[:space:]')"
    [ -n "${line}" ] && REPOS+=("${line}")
  done < "${REPOS_FILE}"
fi
[ "${#REPOS[@]}" -eq 0 ] && { err "Must specify at least one --repo or --repos-file"; usage 1; }
need_cmd gh; need_cmd jq
# Load merged policy JSON for a repo.
load_policy_for_repo() {
  local repo="$1"
  [ -r "${POLICY_FILE}" ] || die "Policy file not readable: ${POLICY_FILE}"
  if command -v yq >/dev/null 2>&1; then
    yq -o=json '.' "${POLICY_FILE}" 2>/dev/null \
      | jq --arg r "${repo}" '
          def base: del(.repos, .pending_nself_ci);
          (.pending_nself_ci[$r] // []) as $pend
          | (.repos[$r] // {}) as $ovr
          | base * $ovr
          | del(.repos, .pending_nself_ci)
          | .pending_nself_ci = $pend
        '
  elif command -v python3 >/dev/null 2>&1; then
    python3 -c '
import json, sys, yaml
with open(sys.argv[1]) as f: d = yaml.safe_load(f)
repos = d.pop("repos", {}) or {}
pend = (d.pop("pending_nself_ci", None) or {}).get(sys.argv[2]) or []
ovr = repos.get(sys.argv[2], {}) or {}
def merge(a, b):
    if isinstance(a, dict) and isinstance(b, dict):
        out = dict(a)
        for k, v in b.items(): out[k] = merge(a.get(k), v)
        return out
    return b if b is not None else a
out = merge(d, ovr)
out["pending_nself_ci"] = pend  # kept only when already live, never applied here
print(json.dumps(out))
' "${POLICY_FILE}" "${repo}"
  else
    die "Need yq or python3+PyYAML to read policy YAML"
  fi
}
# Build a PUT body preserving live app pins; absent pins use -1 (any app).
# Refuse ambiguous pins and retain only pending_nself_ci checks already live.
# Send {strict, checks}: contexts alone unpins checks, both fields cause HTTP 422.
build_protection_body() {
  local policy_json="$1" live_json="${2:-}"
  [ -n "${live_json}" ] || live_json='{}'
  printf '%s' "${policy_json}" | jq --argjson live "${live_json}" '
  (($live.required_status_checks.checks // []) | map({context, app_id: (.app_id // -1)})) as $lc
  | ($lc | group_by(.context) | map(select((map(.app_id) | unique | length) > 1) | .[0].context)) as $dup
  | (if ($dup | length) > 0 then error("live protection pins a context to more than one app: " + ($dup | join(", "))) else . end)
  | ($lc | map({key: .context, value: .app_id}) | from_entries) as $pins
  | (.required_status_checks.contexts // []) as $names
  | [(.pending_nself_ci // [])[] | select(. as $p | $lc | any(.context == $p.context and .app_id == $p.app_id))
     | select(.context as $c | $names | index($c) | not) | {context, app_id}] as $kept
  | {
    required_status_checks: (if .required_status_checks == null then null else {
      strict: .required_status_checks.strict,
      checks: ([$names[] | {context: ., app_id: ($pins[.] // -1)}] + $kept)
    } end),
    enforce_admins: .enforce_admins,
    required_pull_request_reviews: .required_pull_request_reviews, restrictions: .restrictions,
    allow_force_pushes: (.allow_force_pushes // false), allow_deletions: (.allow_deletions // false),
    required_linear_history: (.required_linear_history // false), required_conversation_resolution: (.required_conversation_resolution // false)
  }'
}
# Compare writable fields; strip GitHub links recursively and sort check names.
protection_matches_policy() {
  local probe="$1" body="$2" nourl current_body want_body
  nourl='def nourl: walk(if type == "object" then with_entries(select((.key == "url" or (.key | endswith("_url"))) | not)) else . end);'
  current_body="$(printf '%s' "${probe}" | jq -S -c "${nourl}"'{
    required_status_checks: ((.required_status_checks // {}) | {strict: .strict, checks: ((.checks // ((.contexts // []) | map({context: .}))) | map({context, app_id: (.app_id // -1)}) | sort_by(.context))}),
    enforce_admins: (.enforce_admins.enabled // false),
    required_pull_request_reviews: (.required_pull_request_reviews // null), restrictions: (.restrictions // null),
    allow_force_pushes: (.allow_force_pushes.enabled // false), allow_deletions: (.allow_deletions.enabled // false)
  } | nourl')"
  want_body="$(printf '%s' "${body}" | jq -S -c "${nourl}"'{
    required_status_checks: ((.required_status_checks // {}) | {strict: .strict, checks: ((.checks // []) | map({context, app_id: (.app_id // -1)}) | sort_by(.context))}),
    enforce_admins: .enforce_admins,
    required_pull_request_reviews: (.required_pull_request_reviews // null), restrictions: (.restrictions // null),
    allow_force_pushes: .allow_force_pushes, allow_deletions: .allow_deletions
  } | nourl')"
  [ "${current_body}" = "${want_body}" ]
}
mkdir -p "$(dirname "${STATE_FILE}")"
[ -f "${STATE_FILE}" ] || printf '{}' > "${STATE_FILE}"
check_flap_guard() {
  local repo="$1" branch="$2" action="$3"
  local key="${repo}:${branch}" last_ts now diff
  last_ts=$(jq -r --arg k "${key}" '.[$k].last_ts // 0' "${STATE_FILE}")
  now=$(EPOCH); diff=$(( now - last_ts ))
  if [ "${last_ts}" -gt 0 ] && [ "${diff}" -lt "${FLAP_WINDOW_SECONDS}" ]; then
    if [ "${FORCE}" -eq 0 ]; then
      err "Flap-guard tripped: ${repo}/${branch} toggled ${diff}s ago (<${FLAP_WINDOW_SECONDS}s). Use --force --reason '<text>' to override."
      audit "${repo}" "${branch}" "${action}" "flap_guard" "${REASON:-no reason}"
      return 3
    fi
    warn "Flap-guard overridden by --force (last toggle ${diff}s ago). Reason: ${REASON:-<none>}"
  fi
  return 0
}
record_state() {
  local repo="$1" branch="$2" action="$3"
  local key="${repo}:${branch}" tmp
  tmp="$(mktemp)"
  jq --arg k "${key}" --arg ts "$(EPOCH)" --arg act "${action}" \
    '.[$k] = {last_ts: ($ts|tonumber), last_action: $act}' \
    "${STATE_FILE}" > "${tmp}" && mv "${tmp}" "${STATE_FILE}"
}
# Probe, build target, compare, then apply after the flap-guard.
toggle_one() {
  local repo="$1" current_exists target body
  local api_path="repos/${repo}/branches/${BRANCH}/protection"
  info "[${repo}/${BRANCH}] checking current protection state..."
  # Only recognized 404s are safe to treat as absent.
  local probe probe_err probe_rc=0 errf
  errf="$(mktemp)"
  probe="$(gh api "${api_path}" 2>"${errf}")" || probe_rc=$?
  probe_err="$(cat "${errf}")"; rm -f "${errf}"
  if [ "${probe_rc}" -eq 0 ]; then
    # Fail closed: a partial 200 response could unpin every check.
    if ! printf '%s' "${probe}" | jq -e --arg p "repos/${repo}/branches/${BRANCH}/protection" \
      'type == "object" and ((.url // "") | endswith($p)) and (.enforce_admins | type == "object")' >/dev/null 2>&1; then
      err "[${repo}/${BRANCH}] ${api_path} answered 200 but not with a branch-protection document; refusing, nothing sent"
      audit "${repo}" "${BRANCH}" "${ACTION}" "error" "${REASON}"; return 2
    fi
    current_exists=1
  elif printf '%s %s' "${probe}" "${probe_err}" | grep -q 'Branch not found'; then
    info "[${repo}/${BRANCH}] n/a: no default branch"
    audit "${repo}" "${BRANCH}" "${ACTION}" "n/a" "${REASON}"; return 0
  elif printf '%s %s' "${probe}" "${probe_err}" | grep -q 'Branch not protected'; then
    current_exists=0
  else
    err "[${repo}/${BRANCH}] gh API error reading protection: ${probe_err:-${probe}}"
    audit "${repo}" "${BRANCH}" "${ACTION}" "error" "${REASON}"; return 2
  fi
  case "${ACTION}" in on) target=1 ;; off) target=0 ;; *) die "internal: bad action ${ACTION}" ;; esac
  if [ "${current_exists}" -eq "${target}" ] && [ "${ACTION}" = "off" ]; then
    info "[${repo}/${BRANCH}] already off — no-op"
    audit "${repo}" "${BRANCH}" "${ACTION}" "noop" "${REASON}"; return 0
  fi
  check_flap_guard "${repo}" "${BRANCH}" "${ACTION}" || return $?
  if [ "${ACTION}" = "on" ]; then
    local policy_json live_json='{}'
    policy_json="$(load_policy_for_repo "${repo}")" || { err "[${repo}/${BRANCH}] cannot load policy"; return 2; }
    if [ "${current_exists}" -eq 1 ]; then live_json="${probe}"; fi
    body="$(build_protection_body "${policy_json}" "${live_json}")" \
      || { err "[${repo}/${BRANCH}] cannot build the PUT body (see above); nothing sent"; audit "${repo}" "${BRANCH}" "${ACTION}" "error" "${REASON}"; return 2; }
    if [ "${current_exists}" -eq 1 ] && protection_matches_policy "${probe}" "${body}"; then
      info "[${repo}/${BRANCH}] already matches baseline — no-op"
      audit "${repo}" "${BRANCH}" "${ACTION}" "noop" "${REASON}"; return 0
    fi
  fi
  apply_protection "${repo}" "${api_path}" "${body:-}"
}
# Apply only after probe, policy comparison, and flap-guard have succeeded.
apply_protection() {
  local repo="$1" api_path="$2" body="$3" method verb rc=0
  if [ "${ACTION}" = "on" ]; then method=PUT; verb=applied; else method=DELETE; verb=removed; fi
  if [ "${DRY_RUN}" -eq 1 ]; then
    info "[${repo}/${BRANCH}] DRY-RUN: would ${method} ${api_path}"
    if [ "${ACTION}" = "on" ]; then printf '%s\n' "${body}" | jq .; fi
    audit "${repo}" "${BRANCH}" "${ACTION}" "dry_run" "${REASON}"; return 0
  fi
  if [ "${ACTION}" = "on" ]; then
    printf '%s' "${body}" | gh api -X PUT "${api_path}" --input - >/dev/null 2>&1 || rc=$?
  else
    gh api -X DELETE "${api_path}" >/dev/null 2>&1 || rc=$?
  fi
  if [ "${rc}" -eq 0 ]; then
    info "[${repo}/${BRANCH}] protection ${verb}"
    record_state "${repo}" "${BRANCH}" "${ACTION}"
    audit "${repo}" "${BRANCH}" "${ACTION}" "success" "${REASON}"; return 0
  else
    err "[${repo}/${BRANCH}] gh API ${method} failed"
    audit "${repo}" "${BRANCH}" "${ACTION}" "error" "${REASON}"; return 2
  fi
}
rollback_one() {
  local repo="$1" key last_action invert
  key="${repo}:${BRANCH}"
  last_action="$(jq -r --arg k "${key}" '.[$k].last_action // ""' "${STATE_FILE}")"
  if [ -z "${last_action}" ]; then
    err "[${repo}/${BRANCH}] no previous state recorded — cannot rollback"
    audit "${repo}" "${BRANCH}" "rollback" "no_state" "${REASON}"; return 1
  fi
  case "${last_action}" in on) invert="off" ;; off) invert="on" ;; *) err "[${repo}] unknown last_action: ${last_action}"; return 1 ;; esac
  info "[${repo}/${BRANCH}] rolling back last_action=${last_action} → applying ${invert}"
  ACTION="${invert}" FORCE=1  # rollback bypasses flap-guard by design
  toggle_one "${repo}"
}
FAILED=()
for repo in "${REPOS[@]}"; do
  if [ "${ROLLBACK}" -eq 1 ]; then rollback_one "${repo}" || FAILED+=("${repo}")
  else toggle_one "${repo}" || FAILED+=("${repo}"); fi
done
if [ "${#FAILED[@]}" -gt 0 ]; then
  err "Batch failures: ${FAILED[*]}"
  exit 2
fi
exit 0
