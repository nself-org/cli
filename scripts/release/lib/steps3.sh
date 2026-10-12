# shellcheck shell=bash
# Purpose: release steps S8-S9 (cli image check; admin tag, image wait and check).
# Inputs:  helpers from common.sh; globals VERSION TAG PLAN WORK
# Outputs: step_docker_verify, step_admin_image. Images are checked on Docker Hub with
#          curl (no login); a dispatch happens only when no publish run is on the way.
# Constraints: cli's docker-publish.yml starts from the Release run (workflow_run) and
#          takes input `version`. Admin's starts on a `v*` tag push (also repository_dispatch
#          and workflow_dispatch with `version`, no v), so S9 tags admin and waits for that
#          run instead of dispatching a second publish. Both need the cli release (S7).

IMG_S=""; IMG_ID=""; IMG_REPO=""; IMG_HUB=""; IMG_BEFORE=""; PUB=""; ADMIN_SHA=""
HUB_API="https://hub.docker.com/v2/repositories"

# hub_ok <hub-repo>: the X.Y.Z tag exists with linux/amd64 and linux/arm64.
hub_ok() {
  local j
  j=$(curl -fsS "$HUB_API/$1/tags/$VERSION" 2>/dev/null) || return 1
  printf '%s' "$j" | jq -e '[.images[]? | "\(.os)/\(.architecture)"] | contains(["linux/amd64","linux/arm64"])' >/dev/null 2>&1
}
hub_ok_img() { hub_ok "$IMG_HUB"; }

# image_ready: the image is up, or the dispatch we made has failed (then die).
image_ready() {
  local st
  if hub_ok "$IMG_HUB"; then return 0; fi
  gh_need "the docker-publish.yml dispatches" run list -R "nself-org/$IMG_REPO" --workflow docker-publish.yml \
    --event workflow_dispatch --limit 1 --json databaseId,status,conclusion \
    --jq '.[0] | if . == null or (.databaseId|tostring) == "'"${IMG_BEFORE:-0}"'" then "none" elif .status != "completed" then "pending" else .conclusion end'
  st=$GH_OUT
  case "$st" in
    failure|cancelled|timed_out) die_step "$IMG_S" "$IMG_ID" "docker-publish.yml run for $IMG_REPO concluded '$st'; fix it, then re-run from $IMG_S" ;;
  esac
  return 1
}

# publish_run_present: a docker-publish.yml run on IMG_REPO created after the cli release.
publish_run_present() {
  gh_try run list -R "nself-org/$IMG_REPO" --workflow docker-publish.yml --limit 1 --json createdAt \
    --jq '.[0].createdAt // ""' || return 1
  [ -n "$GH_OUT" ] && ! [[ "$GH_OUT" < "$PUB" ]]
}

# image_step <S#> <id> <repo> <hub-repo>: done if the image is up; otherwise wait
# (about 3 min) for the publish run the release or the tag push starts, wait on it
# while it runs, and dispatch only when there is none.
image_step() {
  IMG_S=$1; IMG_ID=$2; IMG_REPO=$3; IMG_HUB=$4
  local pending recent
  if hub_ok "$IMG_HUB"; then step_done "$IMG_S" "$IMG_ID"; return 0; fi
  gh_need "the cli release $TAG" release view "$TAG" -R nself-org/cli --json publishedAt --jq .publishedAt
  PUB=$GH_OUT
  poll "a docker-publish.yml run on $IMG_REPO" 9 publish_run_present || true
  gh_need "the docker-publish.yml runs of $IMG_REPO" run list -R "nself-org/$IMG_REPO" --workflow docker-publish.yml \
    --limit 5 --json status --jq '[.[] | select(.status != "completed")] | length'
  pending=$GH_OUT
  if [ "${pending:-0}" -gt 0 ]; then
    say "$IMG_S $IMG_ID: a docker-publish.yml run is in flight on $IMG_REPO; waiting for $IMG_HUB:$VERSION"
    poll "$IMG_HUB:$VERSION" 90 hub_ok_img \
      || die_step "$IMG_S" "$IMG_ID" "the in-flight run on $IMG_REPO did not publish $IMG_HUB:$VERSION; check it, then --from $IMG_S"
    say "$IMG_S $IMG_ID: $IMG_HUB:$VERSION is up"; return 0
  fi
  gh_need "the docker-publish.yml runs of $IMG_REPO" run list -R "nself-org/$IMG_REPO" --workflow docker-publish.yml \
    --limit 5 --json createdAt,conclusion \
    --jq "[.[] | select(.conclusion == \"success\" and .createdAt >= \"$PUB\")] | length"
  recent=$GH_OUT
  if [ "${recent:-0}" -gt 0 ] && poll "$IMG_HUB:$VERSION after a recent run" 6 hub_ok_img; then
    say "$IMG_S $IMG_ID: $IMG_HUB:$VERSION is up"; return 0
  fi
  step_confirm "$IMG_S" "$IMG_ID"
  gh_need "the docker-publish.yml dispatches" run list -R "nself-org/$IMG_REPO" --workflow docker-publish.yml \
    --event workflow_dispatch --limit 1 --json databaseId --jq '.[0].databaseId // 0'
  IMG_BEFORE=$GH_OUT
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
    plan_line S8 docker-verify cli "gh run list -R nself-org/cli --workflow docker-publish.yml (wait about 3 min for the run the Release starts, then for it to finish)"
    plan_line S8 docker-verify cli "gh workflow run docker-publish.yml -R nself-org/cli -f version=$VERSION (once, only if no run publishes the tag), then wait and re-check"
    return 0
  fi
  image_step S8 docker-verify cli nself/nself
}

# admin_bump_commit: no merged lockstep PR was found, so ADMIN_SHA = the oldest commit
# of the newest run of commits that carry package.json at VERSION (the bump itself, not
# the tip of main). Looks back over the last 20 commits that touched package.json.
admin_bump_commit() {
  local c prev=""
  gh_need "the admin commits that touched package.json" api "repos/nself-org/admin/commits?path=package.json&per_page=20" --jq '.[].sha'
  for c in $GH_OUT; do
    gh_need "admin package.json at $c" api -H 'Accept: application/vnd.github.raw' "repos/nself-org/admin/contents/package.json?ref=$c"
    if [ "$(printf '%s' "$GH_OUT" | jq -r .version 2>/dev/null || true)" = "$VERSION" ]; then prev=$c; else break; fi
  done
  [ -n "$prev" ] || die_step S9 admin-image "no admin commit sets package.json to $VERSION and no merged lockstep PR was found; run S2" S2
  ADMIN_SHA=$prev
}

# admin_tag: annotated tag vX.Y.Z on admin's S2 merge commit, pushed from the admin
# clone. admin's hygiene release-tag-drift check fails 6 h after the bump without it.
# An existing tag on another commit stops the run; a tag is never moved.
admin_tag() {
  gh_need "the admin lockstep PR" pr list -R nself-org/admin --head "release/v${VERSION}-lockstep" --state merged \
    --json mergeCommit --jq '.[0].mergeCommit.oid // empty'
  ADMIN_SHA=$GH_OUT
  if [ -z "$ADMIN_SHA" ]; then
    admin_bump_commit
  fi
  if repo_tag_sha admin "$TAG"; then
    [ "$TAG_SHA" = "$ADMIN_SHA" ] || die_step S9 admin-image "admin tag $TAG is at $TAG_SHA but the lockstep release commit is $ADMIN_SHA; tags are never moved, inspect by hand"
    say "S9 admin-image: admin tag $TAG already at $ADMIN_SHA"; return 0
  fi
  step_confirm S9 admin-image
  ensure_clone admin; reset_clone admin
  git -C "$WORK/admin" merge-base --is-ancestor "$ADMIN_SHA" origin/main \
    || die_step S9 admin-image "the admin release commit $ADMIN_SHA is not in origin/main's history; inspect before tagging"
  if [ "$(git -C "$WORK/admin" rev-parse -q --verify "refs/tags/$TAG^{commit}" || true)" != "$ADMIN_SHA" ]; then
    git -C "$WORK/admin" tag -a "$TAG" -m "$TAG" "$ADMIN_SHA" || die_step S9 admin-image "could not create the admin tag (a stale $TAG in $WORK/admin, or git user.name / user.email unset?)"
  fi
  git -C "$WORK/admin" push -q origin "refs/tags/$TAG" || die_step S9 admin-image "push of the admin tag $TAG was refused"
  say "S9 admin-image: tagged admin $TAG -> $ADMIN_SHA (this starts admin's docker-publish.yml)"
}

step_admin_image() {
  if [ "$PLAN" = 1 ]; then
    plan_line S9 admin-image admin "gh pr list -R nself-org/admin --head release/v${VERSION}-lockstep --state merged --json mergeCommit (the S2 merge commit)"
    plan_line S9 admin-image admin "git -C $WORK/admin tag -a $TAG -m $TAG SHA && git -C $WORK/admin push origin refs/tags/$TAG (admin's docker-publish.yml starts on a v* tag; hygiene's release-tag-drift check needs the tag)"
    plan_line S9 admin-image admin "gh run list -R nself-org/admin --workflow docker-publish.yml (wait about 3 min for the tag-push run, then for it to finish)"
    plan_line S9 admin-image admin "gh workflow run docker-publish.yml -R nself-org/admin -f version=$VERSION (only if no run publishes the tag)"
    plan_line S9 admin-image admin "curl -fsS $HUB_API/nself/nself-admin/tags/$VERSION (need linux/amd64 and linux/arm64)"
    return 0
  fi
  admin_tag
  image_step S9 admin-image admin nself/nself-admin
}
