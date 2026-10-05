#!/usr/bin/env bash
# Stub gh for the nself-ci-protect tests (P7-CI-60). Never touches the network.
# State: $STUB_DIR/state.json (GET shape). Every call is appended to
# $STUB_DIR/calls; the last PUT body is kept in $STUB_DIR/put-last.json.
# Env: STUB_POST_STATE=<file>  after a PUT, the server state becomes that file
#                              (a server that lands something different);
#      STUB_PUT_FAIL=1         PUT answers HTTP 422;
#      STUB_GET_FAIL=1         GET answers 404 "Branch not protected".
# A PUT body is checked against the documented schema (422 when invalid) and
# turned into GET-shaped state the way GitHub does (app_id -1 reads back null).
D="${STUB_DIR:?}"
[ "$1" = api ] || exit 0
shift
path="$1"; shift; method=GET
while [ $# -gt 0 ]; do
  case "$1" in -X) method="$2"; shift 2 ;; -H|--input|--jq) shift 2 ;; *) shift ;; esac
done
if [ "${path}" = user ]; then echo "stub-user"; exit 0; fi
if [ "${method}" = PUT ]; then
  echo "PUT ${path}" >> "${D}/calls"
  body="$(cat)"
  printf '%s' "${body}" > "${D}/put-last.json"
  if [ "${STUB_PUT_FAIL:-0}" = 1 ]; then echo "HTTP 422 stub" >&2; exit 1; fi
  printf '%s' "${body}" | jq -e '
    ((keys - ["required_status_checks","enforce_admins","required_pull_request_reviews","restrictions","required_linear_history","allow_force_pushes","allow_deletions","block_creations","required_conversation_resolution","lock_branch","allow_fork_syncing"]) | length == 0)
    and ((["required_status_checks","enforce_admins","required_pull_request_reviews","restrictions"] - keys) | length == 0)
    and (.enforce_admins | type == "boolean")
    and (.required_status_checks == null or ((.required_status_checks.strict | type == "boolean") and (.required_status_checks.contexts | type == "array")
         and ((.required_status_checks.checks // []) | all(.app_id | type == "number"))))' > /dev/null \
    || { echo "HTTP 422 stub: body violates the documented schema" >&2; exit 1; }
  printf '%s' "${body}" | jq --arg url "https://api.github.com/${path}" '
    def pr: {users: [(.users // [])[] | {login: .}], teams: [(.teams // [])[] | {slug: .}], apps: [(.apps // [])[] | {slug: .}]};
    def en: {enabled: (. // false)};
    {url: $url,
     required_status_checks: (if .required_status_checks then .required_status_checks as $r | {
        strict: $r.strict,
        contexts: (($r.contexts // []) + [($r.checks // [])[].context]),
        checks: ((($r.contexts // []) | map({context: ., app_id: null}))
          + (($r.checks // []) | map({context, app_id: (if .app_id == -1 then null else .app_id end)})))} else null end),
     enforce_admins: (.enforce_admins | en),
     required_pull_request_reviews: (if .required_pull_request_reviews then .required_pull_request_reviews as $r
        | ($r | {dismiss_stale_reviews, require_code_owner_reviews, required_approving_review_count, require_last_push_approval})
        + (if $r.dismissal_restrictions then {dismissal_restrictions: ($r.dismissal_restrictions | pr)} else {} end)
        else null end),
     restrictions: (if .restrictions then (.restrictions | pr) else null end),
     required_linear_history: (.required_linear_history | en), allow_force_pushes: (.allow_force_pushes | en),
     allow_deletions: (.allow_deletions | en), block_creations: (.block_creations | en),
     required_conversation_resolution: (.required_conversation_resolution | en),
     lock_branch: (.lock_branch | en), allow_fork_syncing: (.allow_fork_syncing | en)}
    | with_entries(select(.value != null))' > "${D}/state.json"
  if [ -n "${STUB_POST_STATE:-}" ]; then cp "${STUB_POST_STATE}" "${D}/state.json"; fi
  exit 0
fi
echo "GET ${path}" >> "${D}/calls"
if [ "${STUB_GET_FAIL:-0}" = 1 ]; then echo "gh: Branch not protected (HTTP 404)" >&2; exit 1; fi
cat "${D}/state.json"
