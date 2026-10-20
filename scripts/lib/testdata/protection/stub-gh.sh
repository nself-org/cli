#!/usr/bin/env bash
# Stub gh. State: $STUB_DIR/state.json (GET shape). Env: STUB_DROP="field ..."
# (server forgets those fields on PUT), STUB_PUT_FAIL=1, STUB_KILL_PARENT=1,
# STUB_SLOW_CI=1 (first 3 `run list` calls say in_progress), STUB_FAIL_AFTER_LAND_ONCE=1
# (first PUT lands but exits 1), STUB_GET_STDERR=1 (GET also writes to stderr),
# STUB_MERGE_MODE=auto (`pr merge` only sets auto-merge: `pr view` says OPEN) |
# fail (`pr merge` exits 1); STUB_MERGE_LAG=N (with auto: OPEN for the first N
# `pr view` calls, then MERGED; `pr merge --disable-auto` stops that, and with
# STUB_LATE_MERGE=1 it merges just then), STUB_VIEW=CLOSED|NONE (`pr view` says
# CLOSED | fails). Default: `pr merge` merges at once.
# PUT bodies are checked against the documented schema (422 when invalid, incl.
# `contexts` and `checks` sent together).
D="${STUB_DIR:?}"
case "$1" in
  auth) echo "Token scopes: 'repo'"; exit 0 ;;
  run)
    n=$(cat "${D}/runcount" 2>/dev/null || echo 0); echo $((n + 1)) > "${D}/runcount"
    if [ "${STUB_SLOW_CI:-0}" = 1 ] && [ "${n}" -lt 3 ]; then echo "in_progress/"; else echo "completed/success"; fi
    exit 0 ;;
  pr)
    printf '%s\n' "$*" >> "${D}/calls"
    if [ "$2" = checks ]; then
      n=$(cat "$D/checkcount" 2>/dev/null || echo 0); echo $((n + 1)) > "$D/checkcount"
      bucket=${STUB_CHECK_BUCKET:-pending}
      [ "${STUB_CHECK_CHANGE:-0}" != 1 ] || [ "$n" -eq 0 ] || bucket=fail
      if [ "$bucket" = none ]; then echo '[]'; exit 0; fi
      printf '[{"bucket":"%s"}]\n' "$bucket"
      case "$bucket" in pending) exit 8;; fail) exit 1;; *) exit 0;; esac
    fi
    if [ "$2" = merge ] && [ "${STUB_KILL_PARENT:-0}" = 1 ]; then kill -9 "${PPID}"; fi
    if [ "$2" = merge ] && [ "${STUB_MERGE_MODE:-}" = fail ]; then echo "stub: merge failed" >&2; exit 1; fi
    if [ "$2" = merge ] && [ "${STUB_MERGE_MODE:-}" = auto ]; then
      if [ "$*" = "${*/--disable-auto/}" ]; then touch "${D}/armed"; echo "will be automatically merged"
      else touch "${D}/disabled"; [ "${STUB_LATE_MERGE:-0}" != 1 ] || touch "${D}/merged"; fi
    fi
    if [ "$2" = view ]; then
      n=$(cat "${D}/viewcount" 2>/dev/null || echo 0); echo $((n + 1)) > "${D}/viewcount"
      if [ "${STUB_VIEW:-}" = NONE ]; then exit 1; fi
      if [ "${STUB_VIEW:-}" = CLOSED ]; then echo CLOSED; exit 0; fi
      if [ -f "${D}/armed" ] && [ ! -f "${D}/disabled" ] && [ "${n}" -ge "${STUB_MERGE_LAG:-999999}" ]; then touch "${D}/merged"; fi
      if [ "${STUB_MERGE_MODE:-}" = fail ]; then echo OPEN; exit 0; fi
      if [ -f "${D}/armed" ] && [ ! -f "${D}/merged" ]; then echo OPEN; else echo MERGED; fi
    fi
    exit 0 ;;
  api) ;;
  *) exit 0 ;;
esac
shift
path="$1"; shift; method=GET
while [ $# -gt 0 ]; do
  case "$1" in -X) method="$2"; shift 2 ;; -H|--input) shift 2 ;; *) shift ;; esac
done
if [ "${method}" = PUT ]; then
  echo "PUT ${path}" >> "${D}/calls"
  body="$(cat)"
  if [ "${STUB_PUT_FAIL:-0}" = 1 ]; then echo "HTTP 422 stub" >&2; exit 1; fi
  printf '%s' "${body}" | jq -e '
    ((keys - ["required_status_checks","enforce_admins","required_pull_request_reviews","restrictions","required_linear_history","allow_force_pushes","allow_deletions","block_creations","required_conversation_resolution","lock_branch","allow_fork_syncing"]) | length == 0)
    and ((["required_status_checks","enforce_admins","required_pull_request_reviews","restrictions"] - keys) | length == 0)
    and (.enforce_admins | type == "boolean")
    and (.required_status_checks == null or ((.required_status_checks.strict | type == "boolean") and ((.required_status_checks | has("contexts") and has("checks")) | not) and ((.required_status_checks.contexts // .required_status_checks.checks) | type == "array")))
    and (.restrictions == null or ((.restrictions.users | type == "array") and (.restrictions.teams | type == "array")))' > /dev/null \
    || { echo "HTTP 422 stub: body violates the documented schema" >&2; exit 1; }
  landfail=0
  if [ "${STUB_FAIL_AFTER_LAND_ONCE:-0}" = 1 ] && [ ! -f "${D}/landed" ]; then touch "${D}/landed"; landfail=1; fi
  printf '%s' "${body}" | jq '
    def pr: {users: [(.users // [])[] | {login: .}], teams: [(.teams // [])[] | {slug: .}], apps: [(.apps // [])[] | {slug: .}]};
    def en: {enabled: (. // false)};
    {url: "stub",
     required_status_checks: (if .required_status_checks then .required_status_checks as $r | {
        strict: $r.strict,
        contexts: (($r.contexts // []) + [($r.checks // [])[].context]),
        checks: ((($r.contexts // []) | map({context: ., app_id: null}))
          + (($r.checks // []) | map({context, app_id: (if .app_id == -1 then null else .app_id end)})))} else null end),
     enforce_admins: (.enforce_admins | en),
     required_pull_request_reviews: (if .required_pull_request_reviews then .required_pull_request_reviews as $r
        | ($r | {dismiss_stale_reviews, require_code_owner_reviews, required_approving_review_count, require_last_push_approval})
        + (if $r.dismissal_restrictions then {dismissal_restrictions: ($r.dismissal_restrictions | pr)} else {} end)
        + (if $r.bypass_pull_request_allowances then {bypass_pull_request_allowances: ($r.bypass_pull_request_allowances | pr)} else {} end)
        else null end),
     restrictions: (if .restrictions then (.restrictions | pr) else null end),
     required_linear_history: (.required_linear_history | en), allow_force_pushes: (.allow_force_pushes | en),
     allow_deletions: (.allow_deletions | en), block_creations: (.block_creations | en),
     required_conversation_resolution: (.required_conversation_resolution | en),
     lock_branch: (.lock_branch | en), allow_fork_syncing: (.allow_fork_syncing | en)}
    | with_entries(select(.value != null))' > "${D}/state.json"
  for f in ${STUB_DROP:-}; do jq "del(.${f})" "${D}/state.json" > "${D}/s.tmp" && mv "${D}/s.tmp" "${D}/state.json"; done
  if [ "${landfail}" = 1 ]; then echo "stub: connection reset after PUT landed" >&2; exit 1; fi
  exit 0
fi
echo "GET ${path}" >> "${D}/calls"
if [ "${STUB_GET_STDERR:-0}" = 1 ]; then echo "gh: notice: a new release is available" >&2; fi
cat "${D}/state.json"
