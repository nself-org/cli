#!/usr/bin/env bash
# nself-ci-protect.sh
#
# Add the app-pinned `nself-ci` required checks (the `pending_nself_ci:` block of
# scripts/.policy/branch-protection.yaml) to a repo's `main` protection WITHOUT
# touching any other setting, and put the saved pre-state back if asked.
# branch-protection-toggle.sh never applies that block; this script is the only
# reader of it. Built on scripts/lib/protection_snapshot.sh (P7-HYG-40): the PUT
# body is snapshot_to_put_body of the pre-state plus the pending checks, so every
# existing check keeps its app_id and every other field is replayed as it was.
#
# Usage:
#   scripts/ci/nself-ci-protect.sh --repo nself-org/cli --dry-run
#   scripts/ci/nself-ci-protect.sh --repo nself-org/cli --apply     # owner-named only
#   scripts/ci/nself-ci-protect.sh --repo nself-org/cli --restore <pre.json>
#   scripts/ci/nself-ci-protect.sh --self-test
#
# Modes (exactly one):
#   --dry-run          GET the live protection, print the added checks, the
#                      normalised diff and the PUT body. Reads only; the only
#                      file written is the pre-state snapshot.
#   --apply            --dry-run, then PUT the target body, GET again and compare
#                      the answer with the target. Exit 3 and print the diff when
#                      GitHub answers anything else. Changes live protection:
#                      run it only when the owner names the action (P7-CI-55).
#   --restore <file>   PUT the saved pre-state body, GET again and compare it with
#                      that pre-state (restore_protection from the library).
#   --self-test        Run scripts/ci/testdata/nself-ci-protect/self_test.sh
#                      (recorded GET fixtures and a stub gh; no network).
# Options:
#   --repo <owner/repo>  Target repo (main branch only).
#   --policy <file>      Policy YAML (default: scripts/.policy/branch-protection.yaml).
#   --out-dir <dir>      Where the pre-state snapshot is written
#                        (default: ${NSELF_CI_PROTECT_DIR:-$HOME/.nself/protection}).
#
# Exit codes: 0 ok (or nothing to add), 1 usage, 2 gh/policy error or a pin
# conflict, 3 apply/restore did not verify.
# Requirements: gh (admin scope for --apply/--restore), jq, yq or python3+PyYAML.
# Bash 3.2 compatible.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="${HERE}/../lib/protection_snapshot.sh"
POLICY_FILE="${HERE}/../.policy/branch-protection.yaml"
OUT_DIR="${NSELF_CI_PROTECT_DIR:-${HOME}/.nself/protection}"
MODE=""
REPO=""
RESTORE_FILE=""

usage() {
  sed -n '/^# Usage:/,/^# Exit codes:/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}
info() { printf '[info]  %s\n' "$*"; }
err() { printf '[error] %s\n' "$*" >&2; }
die() { err "$*"; exit "${2:-2}"; }

set_mode() {
  [ -z "${MODE}" ] || { err "choose exactly one of --dry-run, --apply, --restore, --self-test"; usage 1; }
  MODE="$1"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --repo)      [ $# -ge 2 ] || usage 1; REPO="$2"; shift 2 ;;
    --policy)    [ $# -ge 2 ] || usage 1; POLICY_FILE="$2"; shift 2 ;;
    --out-dir)   [ $# -ge 2 ] || usage 1; OUT_DIR="$2"; shift 2 ;;
    --dry-run)   set_mode dry-run; shift ;;
    --apply)     set_mode apply; shift ;;
    --restore)   [ $# -ge 2 ] || usage 1; set_mode restore; RESTORE_FILE="$2"; shift 2 ;;
    --self-test) set_mode self-test; shift ;;
    -h|--help)   usage 0 ;;
    *) err "unknown flag: $1"; usage 1 ;;
  esac
done

[ -n "${MODE}" ] || { err "choose one of --dry-run, --apply, --restore, --self-test"; usage 1; }

if [ "${MODE}" = "self-test" ]; then
  exec bash "${HERE}/testdata/nself-ci-protect/self_test.sh"
fi

[ -n "${REPO}" ] || { err "--repo <owner/repo> is required"; usage 1; }
case "${REPO}" in
  */*) ;;
  *) err "--repo must look like owner/repo, got: ${REPO}"; usage 1 ;;
esac
command -v gh >/dev/null 2>&1 || die "required tool not found: gh"
command -v jq >/dev/null 2>&1 || die "required tool not found: jq"
[ -r "${LIB}" ] || die "library not found: ${LIB}"
# shellcheck disable=SC1090,SC1091  # sourced by path; shellcheck runs without -x
. "${LIB}"

API="repos/${REPO}/branches/main/protection"

# pending_for_repo: the repo's `pending_nself_ci` list as a JSON array of
# {context, app_id}. A repo with no entry yields [].
pending_for_repo() {
  [ -r "${POLICY_FILE}" ] || die "policy file not readable: ${POLICY_FILE}"
  if command -v yq >/dev/null 2>&1; then
    yq -o=json '.' "${POLICY_FILE}" | jq -c --arg r "${REPO}" '.pending_nself_ci[$r] // []'
  elif command -v python3 >/dev/null 2>&1; then
    python3 -c '
import json, sys, yaml
with open(sys.argv[1]) as f: d = yaml.safe_load(f) or {}
print(json.dumps((d.get("pending_nself_ci") or {}).get(sys.argv[2]) or []))
' "${POLICY_FILE}" "${REPO}"
  else
    die "need yq or python3+PyYAML to read the policy YAML"
  fi
}

# validate_pending: every entry needs a non-empty string context and an integer
# app_id; an unpinned (-1) check is refused because the whole point is the pin.
validate_pending() {
  printf '%s' "$1" | jq -e 'all(.[]; (.context | type == "string" and length > 0)
    and (.app_id | type == "number" and . > 0))' >/dev/null \
    || die "pending_nself_ci for ${REPO} must be a list of {context, app_id > 0}"
}

# Target snapshot = pre-state + pending checks (GET shape, so the library's
# normalise/compare/snapshot_to_put_body apply unchanged). A pending context
# already required with the SAME app_id is skipped; with a DIFFERENT app_id it is
# a conflict (changing an existing pin is not this tool's job) and errors.
# shellcheck disable=SC2016  # jq program, single-quoted on purpose
JQ_TARGET='
  def aid: (.app_id // -1);
  (.required_status_checks // {strict: false, contexts: [], checks: []}) as $r
  | ($r.checks // []) as $have
  | [$pending[] | select(. as $w | $have | map(select(.context == $w.context and aid != ($w | aid))) | length > 0)] as $conflict
  | [$pending[] | select(. as $w | $have | map(select(.context == $w.context)) | length == 0)] as $add
  | if ($conflict | length) > 0
    then error("pin conflict: " + ($conflict | map(.context) | join(", ")) + " is already required with another app_id")
    else .required_status_checks = ($r
      | .checks = ($have + $add)
      | .contexts = ((.contexts // []) + ($add | map(.context)))) end'

# Snapshot GET. Not protected / unreadable is an error: there is nothing to extend.
fetch_pre() {
  local out errf
  errf="$(mktemp "${TMPDIR:-/tmp}/ncp-err.XXXXXX")"
  if ! out="$(gh api "${API}" 2>"${errf}")" || ! printf '%s' "${out}" | jq -e 'type == "object"' >/dev/null 2>&1; then
    err "cannot read ${API}: $(cat "${errf}")"
    rm -f "${errf}"
    exit 2
  fi
  rm -f "${errf}"
  printf '%s' "${out}"
}

# Prints "ADD <context> app_id <n>" for each check target has and pre lacks.
added_checks() {
  jq -rn --argjson a "$(normalise "$1")" --argjson b "$(normalise "$2")" '
    ($a.required_status_checks.checks // []) as $old
    | ($b.required_status_checks.checks // [])[]
    | select(. as $c | $old | index($c) | not)
    | "ADD \(.context) app_id \(.app_id)"'
}

# plan: shared by --dry-run and --apply. Sets PRE, TARGET, BODY, PRE_FILE.
plan() {
  local pending
  pending="$(pending_for_repo)" || exit 2
  validate_pending "${pending}"
  PRE="$(fetch_pre)"
  ps_unexpressible "${PRE}" || die "refusing: the pre-state cannot be replayed through the PUT API"
  TARGET="$(printf '%s' "${PRE}" | jq -S --argjson pending "${pending}" "${JQ_TARGET}")" \
    || die "cannot compute the target for ${REPO} (see pin conflict above)"
  BODY="$(snapshot_to_put_body "${TARGET}")" || die "cannot build the PUT body"

  mkdir -p "${OUT_DIR}"
  # One file per run, never overwritten: a second run sees the already-changed
  # state and must not replace the original restore point.
  PRE_FILE="${OUT_DIR}/$(printf '%s' "${REPO}" | tr '/' '_')-pre-$(date -u +%Y%m%dT%H%M%SZ)-$$.json"
  printf '%s\n' "${PRE}" > "${PRE_FILE}"
  info "pre-state saved: ${PRE_FILE} (restore with --restore)"

  ADDED="$(added_checks "${PRE}" "${TARGET}")"
  DIFF="$(compare "${PRE}" "${TARGET}" || true)"
  if [ -z "${ADDED}" ]; then
    info "[${REPO}] every pending check is already required: nothing to add"
  else
    printf '%s\n' "${ADDED}"
    printf 'normalised diff (expected = live, actual = target):\n%s\n' "${DIFF}"
  fi
}

# print_body is the last thing a mode prints, so the JSON ends the output.
print_body() { printf 'PUT body for %s:\n%s\n' "${API}" "${BODY}"; }

cmd_dry_run() {
  plan
  ps_audit "NSELF_CI_PROTECT  repo=${REPO}  mode=dry-run  added=$(printf '%s' "${ADDED}" | grep -c . || true)"
  info "[${REPO}] DRY-RUN: no PUT was sent"
  print_body
}

cmd_apply() {
  local after diff errf
  plan
  if [ -z "${ADDED}" ]; then
    ps_audit "NSELF_CI_PROTECT  repo=${REPO}  mode=apply  result=noop"
    return 0
  fi
  print_body
  if ! printf '%s' "${BODY}" | gh api "${API}" -X PUT --input - -H "Accept: application/vnd.github.v3+json" >/dev/null; then
    err "PUT rejected for ${REPO}; restore with: $0 --repo ${REPO} --restore ${PRE_FILE}"
    ps_audit "NSELF_CI_PROTECT  repo=${REPO}  mode=apply  result=put_rejected  pre=${PRE_FILE}"
    return 3
  fi
  errf="$(mktemp "${TMPDIR:-/tmp}/ncp-err.XXXXXX")"
  if ! after="$(gh api "${API}" 2>"${errf}")" || ! printf '%s' "${after}" | jq -e 'type == "object"' >/dev/null 2>&1; then
    err "verify GET failed: $(cat "${errf}")"
    rm -f "${errf}"
    ps_audit "NSELF_CI_PROTECT  repo=${REPO}  mode=apply  result=verify_get_failed  pre=${PRE_FILE}"
    return 3
  fi
  rm -f "${errf}"
  if ! diff="$(compare "${TARGET}" "${after}")"; then
    err "protection after the PUT differs from the target (expected = target, actual = live):"
    printf '%s\n' "${diff}" >&2
    err "restore with: $0 --repo ${REPO} --restore ${PRE_FILE}"
    ps_audit "NSELF_CI_PROTECT  repo=${REPO}  mode=apply  result=mismatch  pre=${PRE_FILE}"
    return 3
  fi
  info "[${REPO}] applied and verified: live protection equals the target"
  ps_audit "NSELF_CI_PROTECT  repo=${REPO}  mode=apply  result=verified  pre=${PRE_FILE}"
}

cmd_restore() {
  [ -r "${RESTORE_FILE}" ] || die "pre-state file not readable: ${RESTORE_FILE}"
  restore_protection "${REPO}" "${RESTORE_FILE}" 0 "nself_ci_protect_restore" || exit 3
}

case "${MODE}" in
  dry-run) cmd_dry_run ;;
  apply)   cmd_apply ;;
  restore) cmd_restore ;;
esac
