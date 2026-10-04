#!/usr/bin/env bash
# protection_snapshot.sh
#
# Library (source it; do not execute) for admin-merge.sh: turn the branch
# protection JSON returned by `GET /repos/{o}/{r}/branches/{b}/protection`
# (the "snapshot") into a PUT body that restores EVERY field the PUT API
# accepts, then GET-verify that the restore landed.
#
# Functions
#   ps_unexpressible <snapshot>      exit 1 + message when the snapshot holds a
#                                    field the PUT API cannot express (unknown
#                                    top-level key, or required_signatures on).
#   normalise <snapshot>             canonical, key-sorted, PUT-settable-only
#                                    JSON (booleans unwrapped, logins/slugs
#                                    sorted, contexts folded into checks).
#   snapshot_to_put_body <snapshot>  the PUT body that reproduces the snapshot.
#   build_relax_body <snapshot>      the temporary relaxed PUT body.
#   compare <a> <b>                  exit 0 when normalise(a) == normalise(b);
#                                    otherwise print one line per differing
#                                    field and exit 1.
#   restore_protection <repo> <snapshot-file> <dry_run> <reason>
#                                    PUT the snapshot body, GET again, compare.
#                                    Returns 0 verified, 3 on any failure
#                                    (audit lines RESTORE_FAILED or
#                                    restore_mismatch). dry_run=1 prints the
#                                    body and calls nothing.
#
# Audit lines go to ${AUDIT_LOG} (default ~/.nself/admin-merge-audit.log).
# Status checks: checks that all accept any app are sent as `contexts` (the
# long-standing form); any check pinned to an app id is sent as `checks` with
# `contexts: []`. Bash 3.2 compatible; needs jq and gh.
#
# Authority: P7-HYG-40; Constitution 14.3 (restore from the snapshot, verify)
# and 4.1 (no silent wrong answer).

# shellcheck disable=SC2016  # jq programs are single-quoted on purpose
PS_KNOWN_KEYS='["url","required_status_checks","enforce_admins","required_pull_request_reviews","restrictions","required_linear_history","allow_force_pushes","allow_deletions","block_creations","required_conversation_resolution","lock_branch","allow_fork_syncing","required_signatures"]'

# GET shape -> canonical PUT-settable shape.
PS_JQ_NORMALISE='
def names(f): [(. // [])[] | f] | sort;
def principals: {users: .users | names(.login), teams: .teams | names(.slug), apps: .apps | names(.slug)};
def en(k): (.[k].enabled // false);
(.required_status_checks.checks // []) as $ch
| ($ch | map(.context)) as $named
| {
    required_status_checks: (if .required_status_checks then {
        strict: (.required_status_checks.strict // false),
        checks: (($ch | map({context, app_id: (.app_id // -1)}))
          + [(.required_status_checks.contexts // [])[]
             | select(. as $c | $named | index($c) | not) | {context: ., app_id: -1}]
          | sort_by(.context, .app_id))
      } else null end),
    enforce_admins: en("enforce_admins"),
    required_pull_request_reviews: (if .required_pull_request_reviews then
      .required_pull_request_reviews as $r | {
        dismiss_stale_reviews: ($r.dismiss_stale_reviews // false),
        require_code_owner_reviews: ($r.require_code_owner_reviews // false),
        required_approving_review_count: ($r.required_approving_review_count // 0),
        require_last_push_approval: ($r.require_last_push_approval // false)
      }
      + (if $r.dismissal_restrictions then {dismissal_restrictions: ($r.dismissal_restrictions | principals)} else {} end)
      + (if ($r.bypass_pull_request_allowances | principals | [.[] | length] | add // 0) > 0
         then {bypass_pull_request_allowances: ($r.bypass_pull_request_allowances | principals)} else {} end)
      else null end),
    restrictions: (if .restrictions then (.restrictions | principals) else null end),
    required_linear_history: en("required_linear_history"),
    allow_force_pushes: en("allow_force_pushes"),
    allow_deletions: en("allow_deletions"),
    block_creations: en("block_creations"),
    required_conversation_resolution: en("required_conversation_resolution"),
    lock_branch: en("lock_branch"),
    allow_fork_syncing: en("allow_fork_syncing")
  }'

# Canonical shape -> PUT body (contexts form unless a check is pinned to an app).
PS_JQ_PUT='
.required_status_checks |= (if . == null then null
  elif all(.checks[]; .app_id == -1) then {strict: .strict, contexts: (.checks | map(.context))}
  else {strict: .strict, contexts: [], checks: .checks} end)'

ps_audit() {
  local log="${AUDIT_LOG:-${HOME}/.nself/admin-merge-audit.log}"
  mkdir -p "$(dirname "${log}")" 2>/dev/null || true
  printf '%s  %s\n' "$(date '+%Y-%m-%dT%H:%M:%S%z')" "$*" >> "${log}" 2>/dev/null || true
}

ps_unexpressible() {
  local bad
  bad=$(printf '%s' "$1" | jq -r --argjson known "${PS_KNOWN_KEYS}" '
    [(keys - $known)[]] + (if .required_signatures.enabled == true then ["required_signatures (enabled)"] else [] end)
    | join(", ")') || return 1
  if [ -n "${bad}" ]; then
    printf 'snapshot holds a field the protection PUT API cannot express: %s\n' "${bad}" >&2
    return 1
  fi
}

normalise() { printf '%s' "$1" | jq -S "${PS_JQ_NORMALISE}"; }

snapshot_to_put_body() {
  ps_unexpressible "$1" || return 1
  printf '%s' "$1" | jq -S "${PS_JQ_NORMALISE} | ${PS_JQ_PUT}"
}

# Same shape as the historical relax payload; checks keep their context names.
build_relax_body() {
  printf '%s' "$1" | jq '{
    required_status_checks: {strict: false, contexts: (.required_status_checks.contexts // [])},
    enforce_admins: false,
    required_pull_request_reviews: {required_approving_review_count: 0, dismiss_stale_reviews: false},
    restrictions: null,
    required_linear_history: false,
    allow_force_pushes: false,
    allow_deletions: false
  }'
}

compare() {
  local na nb diff
  na=$(normalise "$1") || return 1
  nb=$(normalise "$2") || return 1
  diff=$(jq -rn --argjson a "${na}" --argjson b "${nb}" '
    (($a | keys) + ($b | keys) | unique)[] as $k
    | select($a[$k] != $b[$k])
    | "  \($k): expected \($a[$k] | tojson) actual \($b[$k] | tojson)"')
  if [ -n "${diff}" ]; then
    printf '%s\n' "${diff}"
    return 1
  fi
}

restore_protection() {
  local repo="$1" snapfile="$2" dry_run="${3:-0}" reason="${4:-scheduled_restore}"
  local snap body out after diff
  local api="repos/${repo}/branches/main/protection"

  printf '[RESTORE] Restoring branch protection on %s/main (%s)...\n' "${repo}" "${reason}" >&2
  if ! snap=$(cat "${snapfile}" 2>/dev/null) || [ -z "${snap}" ]; then
    printf '[RESTORE] FAILED: snapshot file %s missing or empty\n' "${snapfile}" >&2
    ps_audit "RESTORE_FAILED  repo=${repo}  reason=${reason}  detail=no_snapshot"
    return 3
  fi
  if ! body=$(snapshot_to_put_body "${snap}"); then
    ps_audit "RESTORE_FAILED  repo=${repo}  reason=${reason}  detail=unexpressible_snapshot"
    return 3
  fi
  if [ "${dry_run}" -eq 1 ]; then
    printf '[RESTORE][DRY-RUN] Would PUT %s:\n%s\n' "${api}" "${body}" >&2
    return 0
  fi

  if ! out=$(gh api "${api}" -X PUT --input - -H "Accept: application/vnd.github.v3+json" <<< "${body}" 2>&1 >/dev/null); then
    printf '[RESTORE] FAILED: PUT rejected: %s\n' "${out}" >&2
    printf '[RESTORE] Manual recovery: scripts/admin-merge.sh --repo %s --force-restore --snapshot %s\n' "${repo}" "${snapfile}" >&2
    ps_audit "RESTORE_FAILED  repo=${repo}  reason=${reason}  detail=put_rejected  snapshot=${snapfile}"
    return 3
  fi
  if ! after=$(gh api "${api}" 2>&1); then
    printf '[RESTORE] FAILED: verify GET failed: %s\n' "${after}" >&2
    ps_audit "restore_mismatch  repo=${repo}  reason=${reason}  detail=verify_get_failed  snapshot=${snapfile}"
    return 3
  fi
  if ! diff=$(compare "${snap}" "${after}"); then
    printf '[RESTORE] FAILED: protection differs from the pre-merge snapshot:\n%s\n' "${diff}" >&2
    printf '[RESTORE] Manual recovery: scripts/admin-merge.sh --repo %s --force-restore --snapshot %s\n' "${repo}" "${snapfile}" >&2
    ps_audit "restore_mismatch  repo=${repo}  reason=${reason}  snapshot=${snapfile}  diff=$(printf '%s' "${diff}" | tr '\n' ';' | tr -s ' ')"
    return 3
  fi
  printf '[RESTORE] Protection restored and verified identical to the snapshot.\n' >&2
  ps_audit "RESTORE  repo=${repo}  reason=${reason}  verified=1"
}
