# shellcheck shell=bash
# Purpose: release step S10 (SDK publish dispatches).
# Inputs:  helpers from common.sh; globals VERSION TAG PLAN
# Outputs: step_sdk_publish. A workflow is dispatched only when no run on the tag has
#          succeeded. Workflows are dispatched on the tag's ref, so each publishes the
#          manifest version committed at the tag.
# Constraints: only sdk-publish-go.yml takes the CLI version as an input; the py, ts and
#          ts-sdk workflows would overwrite their own manifest version with it, so they
#          get no version input.

SDK_LIST=""; SDK_TXT=""; SDK_ST=""; SDK_F=""

# sdk_files <ref>: SDK_LIST = names of the sdk publish workflows on that ref.
sdk_files() {
  gh_need "the cli workflow list at $1" api "repos/nself-org/cli/contents/.github/workflows?ref=$1" --jq '.[].name'
  SDK_LIST=$(printf '%s\n' "$GH_OUT" | grep -E '^sdk-.*publish.*\.yml$' || true)
}

# sdk_run_state <file>: SDK_ST = success | pending | failure | none for runs on the tag.
sdk_run_state() {
  # shellcheck disable=SC2016
  gh_need "the $1 runs" run list -R nself-org/cli --workflow "$1" --branch "$TAG" --limit 10 --json status,conclusion --jq '
    (map(select(.conclusion == "success")) | length) as $ok
    | (map(select(.status != "completed")) | length) as $p
    | if $ok > 0 then "success" elif $p > 0 then "pending" elif length > 0 then "failure" else "none" end'
  SDK_ST=$GH_OUT
}
sdk_settled() { sdk_run_state "$SDK_F"; [ "$SDK_ST" != pending ]; }
sdk_ok() { sdk_run_state "$SDK_F"; [ "$SDK_ST" = success ]; }

step_sdk_publish() {
  local ref=$TAG f args n=0
  if [ "$PLAN" = 1 ]; then ref=main; fi
  sdk_files "$ref"
  for f in $SDK_LIST; do
    n=$((n + 1))
    gh_need "$f at $ref" api -H 'Accept: application/vnd.github.raw' "repos/nself-org/cli/contents/.github/workflows/$f?ref=$ref"
    SDK_TXT=$GH_OUT
    if ! printf '%s\n' "$SDK_TXT" | grep -q '^[[:space:]]*workflow_dispatch:'; then
      if [ "$PLAN" = 1 ]; then plan_line S10 sdk-publish cli "manual: $f has no workflow_dispatch; publish it by hand"; else
        say "S10 sdk-publish: $f has no workflow_dispatch; publish by hand if its tag run failed"; fi
      continue
    fi
    args="--ref $TAG"
    if [ "$f" = sdk-publish-go.yml ] && printf '%s\n' "$SDK_TXT" | grep -q '^[[:space:]]*version:[[:space:]]*$'; then
      args="$args -f version=$VERSION"
    fi
    if [ "$PLAN" = 1 ]; then
      plan_line S10 sdk-publish cli "gh workflow run $f -R nself-org/cli $args (skipped when a run on $TAG already succeeded)"
      continue
    fi
    SDK_F=$f
    sdk_run_state "$f"
    if [ "$SDK_ST" = pending ]; then
      poll "$f run on $TAG" 90 sdk_settled || die_step S10 sdk-publish "$f still running after the wait budget"
      sdk_run_state "$f"
    fi
    if [ "$SDK_ST" = success ]; then step_done S10 "sdk-publish $f"; continue; fi
    step_confirm S10 "sdk-publish $f"
    # shellcheck disable=SC2086
    gh workflow run "$f" -R nself-org/cli $args || die_step S10 sdk-publish "could not dispatch $f"
    sleep "$POLL_SECONDS"
    poll "$f success on $TAG" 90 sdk_ok || die_step S10 sdk-publish "$f has no successful run on $TAG (registry secrets set? P7-PROD-61)"
    say "S10 sdk-publish: $f succeeded"
  done
  if [ "$n" = 0 ]; then
    if [ "$PLAN" = 1 ]; then plan_line S10 sdk-publish cli "no sdk-*publish*.yml workflow found on $ref"; else
      say "S10 sdk-publish: no sdk-*publish*.yml workflow on $ref; nothing to dispatch"; fi
  fi
}
