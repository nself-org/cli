# shellcheck shell=bash
# Purpose: release steps S8-S10 (cli image check, admin image dispatch + check, SDK
#          publish dispatches).
# Inputs:  helpers from common.sh; globals VERSION TAG PLAN
# Outputs: step_* functions. Images are checked on Docker Hub with curl (no login);
#          dispatches go through gh and run only when the artifact is absent.
# Constraints: docker-publish.yml in cli takes input `version` (blank = latest, with
#          or without v); admin's takes `version` without v. Both need a published
#          GitHub release (cli S7 guarantees it). Match scripts/ci/docker-publish-version.sh.

IMG_S=""; IMG_ID=""; IMG_REPO=""; IMG_HUB=""; IMG_BEFORE=""
HUB_API="https://hub.docker.com/v2/repositories"

# hub_ok <hub-repo>: the X.Y.Z tag exists with linux/amd64 and linux/arm64.
hub_ok() {
  local j
  j=$(curl -fsS "$HUB_API/$1/tags/$VERSION" 2>/dev/null) || return 1
  printf '%s' "$j" | jq -e '[.images[]? | "\(.os)/\(.architecture)"] | contains(["linux/amd64","linux/arm64"])' >/dev/null 2>&1
}

newest_dispatch() { # <repo> <field-expr>
  gh run list -R "nself-org/$IMG_REPO" --workflow docker-publish.yml --event workflow_dispatch --limit 1 \
    --json databaseId,status,conclusion --jq "$1"
}

# image_ready: the image is up, or the dispatch we made has failed (then die).
image_ready() {
  local st
  if hub_ok "$IMG_HUB"; then return 0; fi
  st=$(newest_dispatch '.[0] | if . == null or (.databaseId|tostring) == "'"${IMG_BEFORE:-0}"'" then "none" elif .status != "completed" then "pending" else .conclusion end')
  case "$st" in
    failure|cancelled|timed_out) die_step "$IMG_S" "$IMG_ID" "docker-publish.yml run for $IMG_REPO concluded '$st'; fix it, then re-run from $IMG_S" ;;
  esac
  return 1
}

hub_ok_img() { hub_ok "$IMG_HUB"; }

# image_step <S#> <id> <repo> <hub-repo>: done if the image is up; wait on a
# docker-publish.yml run that is in flight or finished since the release; dispatch
# only when none exists.
image_step() {
  IMG_S=$1; IMG_ID=$2; IMG_REPO=$3; IMG_HUB=$4
  local pending pub recent
  if hub_ok "$IMG_HUB"; then step_done "$IMG_S" "$IMG_ID"; return 0; fi
  pending=$(gh run list -R "nself-org/$IMG_REPO" --workflow docker-publish.yml --limit 5 --json status \
    --jq '[.[] | select(.status != "completed")] | length')
  if [ "${pending:-0}" -gt 0 ]; then
    say "$IMG_S $IMG_ID: a docker-publish.yml run is in flight on $IMG_REPO; waiting for $IMG_HUB:$VERSION"
    poll "$IMG_HUB:$VERSION" 90 hub_ok_img \
      || die_step "$IMG_S" "$IMG_ID" "the in-flight run on $IMG_REPO did not publish $IMG_HUB:$VERSION; check it, then --from $IMG_S"
    say "$IMG_S $IMG_ID: $IMG_HUB:$VERSION is up"; return 0
  fi
  pub=$(gh release view "$TAG" -R nself-org/cli --json publishedAt --jq .publishedAt)
  recent=$(gh run list -R "nself-org/$IMG_REPO" --workflow docker-publish.yml --limit 5 --json createdAt,conclusion \
    --jq "[.[] | select(.conclusion == \"success\" and .createdAt >= \"$pub\")] | length")
  if [ "${recent:-0}" -gt 0 ] && poll "$IMG_HUB:$VERSION after a recent run" 6 hub_ok_img; then
    say "$IMG_S $IMG_ID: $IMG_HUB:$VERSION is up"; return 0
  fi
  step_confirm "$IMG_S" "$IMG_ID"
  IMG_BEFORE=$(newest_dispatch '.[0].databaseId // 0')
  gh workflow run docker-publish.yml -R "nself-org/$IMG_REPO" -f "version=$VERSION" \
    || die_step "$IMG_S" "$IMG_ID" "could not dispatch docker-publish.yml on $IMG_REPO"
  say "$IMG_S $IMG_ID: dispatched docker-publish.yml on $IMG_REPO"
  sleep "$POLL_SECONDS"
  poll "$IMG_HUB:$VERSION" 90 image_ready \
    || die_step "$IMG_S" "$IMG_ID" "$IMG_HUB:$VERSION (amd64 + arm64) not on Docker Hub after the wait budget"
  say "$IMG_S $IMG_ID: $IMG_HUB:$VERSION is up (linux/amd64, linux/arm64)"
}

step_docker_verify() {
  if [ "$PLAN" = 1 ]; then
    plan_line S8 docker-verify cli "curl -fsS $HUB_API/nself/nself/tags/$VERSION (need linux/amd64 and linux/arm64)"
    plan_line S8 docker-verify cli "gh workflow run docker-publish.yml -R nself-org/cli -f version=$VERSION (once, only if the tag is absent), then wait and re-check"
    return 0
  fi
  image_step S8 docker-verify cli nself/nself
}

step_admin_image() {
  if [ "$PLAN" = 1 ]; then
    plan_line S9 admin-image admin "gh workflow run docker-publish.yml -R nself-org/admin -f version=$VERSION (only if the tag is absent)"
    plan_line S9 admin-image admin "curl -fsS $HUB_API/nself/nself-admin/tags/$VERSION (need linux/amd64 and linux/arm64)"
    return 0
  fi
  image_step S9 admin-image admin nself/nself-admin
}

# sdk_files <ref>: names of the sdk publish workflows on that ref.
sdk_files() {
  gh api "repos/nself-org/cli/contents/.github/workflows?ref=$1" --jq '.[].name' \
    | grep -E '^sdk-.*publish.*\.yml$' || true
}
sdk_file_text() { gh api -H 'Accept: application/vnd.github.raw' "repos/nself-org/cli/contents/.github/workflows/$1?ref=$2"; }

# sdk_run_state <file>: success | pending | failure | none for runs on the tag.
sdk_run_state() {
  # shellcheck disable=SC2016
  gh run list -R nself-org/cli --workflow "$1" --branch "$TAG" --limit 10 --json status,conclusion --jq '
    (map(select(.conclusion == "success")) | length) as $ok
    | (map(select(.status != "completed")) | length) as $p
    | if $ok > 0 then "success" elif $p > 0 then "pending" elif length > 0 then "failure" else "none" end'
}
SDK_F=""
sdk_settled() { [ "$(sdk_run_state "$SDK_F")" != pending ]; }
sdk_ok() { [ "$(sdk_run_state "$SDK_F")" = success ]; }

step_sdk_publish() {
  local ref=$TAG f txt st args n=0
  if [ "$PLAN" = 1 ]; then ref=main; fi
  for f in $(sdk_files "$ref"); do
    n=$((n + 1))
    txt=$(sdk_file_text "$f" "$ref")
    if ! printf '%s\n' "$txt" | grep -q '^[[:space:]]*workflow_dispatch:'; then
      if [ "$PLAN" = 1 ]; then plan_line S10 sdk-publish cli "manual: $f has no workflow_dispatch; publish it by hand"; else
        say "S10 sdk-publish: $f has no workflow_dispatch; publish by hand if its tag run failed"; fi
      continue
    fi
    args="--ref $TAG"
    if printf '%s\n' "$txt" | grep -q '^[[:space:]]*version:[[:space:]]*$'; then args="$args -f version=$VERSION"; fi
    if [ "$PLAN" = 1 ]; then
      plan_line S10 sdk-publish cli "gh workflow run $f -R nself-org/cli $args (skipped when a run on $TAG already succeeded)"
      continue
    fi
    SDK_F=$f
    st=$(sdk_run_state "$f")
    if [ "$st" = pending ]; then
      poll "$f run on $TAG" 90 sdk_settled || die_step S10 sdk-publish "$f still running after the wait budget"
      st=$(sdk_run_state "$f")
    fi
    if [ "$st" = success ]; then step_done S10 "sdk-publish $f"; continue; fi
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
