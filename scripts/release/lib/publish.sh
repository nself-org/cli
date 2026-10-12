# shellcheck shell=bash
# Purpose: release step S7 (release wait, published-hash check, web lock PR).
# Inputs:  helpers from common.sh, hashes.sh, steps2.sh; globals VERSION TAG PLAN WORK
# Outputs: step_release_wait. The web PR is opened but never merged: the owner does that.
# Constraints: GET-only except the web branch push and PR; no deploy.

web_pr() {
  local br="release/cli-v${VERSION}" url
  pr_info web "$br"
  if [ -n "$PR_INFO" ]; then say "S7 release-wait web done (PR #${PR_INFO%% *}, ${PR_INFO##* })"; return 0; fi
  command -v node >/dev/null 2>&1 || die_step S7 release-wait "node is not installed (needed for update-lock.mjs)"
  step_confirm S7 release-wait
  ensure_clone web; reset_clone web
  git -C "$WORK/web" checkout -q -B "$br" || die_step S7 release-wait "could not create branch $br"
  (cd "$WORK/web" && node org/scripts/sources/update-lock.mjs cli "$TAG") \
    || die_step S7 release-wait "update-lock.mjs failed in the web clone"
  if ! commit_all web "chore(sources): pin cli ${TAG}"; then
    say "S7 release-wait web done (lock already pins $TAG)"; return 0
  fi
  git -C "$WORK/web" push -q origin "$br" || die_step S7 release-wait "push of $br to web was refused"
  url=$(gh pr create -R nself-org/web --base main --head "$br" --title "chore(sources): pin cli ${TAG}" \
    --body "Pins the public cli source to ${TAG} (update-lock.mjs). The owner merges; no deploy.") \
    || die_step S7 release-wait "gh pr create failed on web"
  say "S7 release-wait web: opened $url (owner merges; no deploy)"
}

failed_jobs() {
  gh_try run view "$RUN_ID" -R nself-org/cli --json jobs --jq '[.jobs[] | select(.conclusion == "failure") | .name] | join(", ")' || GH_OUT="?"
  printf '%s\n' "${GH_OUT:-?}"
}

step_release_wait() {
  local early=0 n
  if [ "$PLAN" = 1 ]; then
    plan_line S7 release-wait cli "gh run view RUN_ID -R nself-org/cli --json status,conclusion (wait for success); gh release view $TAG -R nself-org/cli --json assets"
    plan_line S7 release-wait cli "gh release download $TAG -R nself-org/cli -p checksums.txt; compare its darwin lines with on_arm/on_intel of Formula/nself.rb on homebrew-nself main"
    plan_line S7 release-wait web "node org/scripts/sources/update-lock.mjs cli $TAG (fresh web clone); gh pr create -R nself-org/web --head release/cli-v${VERSION} (owner merges; no deploy)"
    return 0
  fi
  find_run
  [ -n "$RUN_ID" ] || die_step S7 release-wait "no release.yml run found for $TAG"
  release_state "$RUN_ID"
  if [ "$REL_ST" = success ]; then early=1; fi
  if [ "$REL_ST" = pending ]; then
    poll "release run $RUN_ID" 120 release_finished || die_step S7 release-wait "release run $RUN_ID still running after the wait budget"
    release_state "$RUN_ID"
  fi
  [ "$REL_ST" = success ] || die_step S7 release-wait "release run $RUN_ID concluded '$REL_ST'; failed jobs: $(failed_jobs). 'Verify Homebrew formula lockstep': --from S6. 'Pre-Release Gate': satisfy the DENY rule in its log (the tag never moves), then gh run rerun $RUN_ID -R nself-org/cli --failed. Any other job: fix, then gh run rerun $RUN_ID -R nself-org/cli --failed. Then --from S7"
  gh_need "the release $TAG" release view "$TAG" -R nself-org/cli --json assets --jq '.assets[].name'
  printf '%s\n' "$GH_OUT" | grep -qx 'checksums.txt' || die_step S7 release-wait "release $TAG has no checksums.txt asset"
  n=$(printf '%s\n' "$GH_OUT" | wc -l | tr -d ' ')
  verify_published
  if [ "$early" = 1 ]; then step_done S7 release-wait; else
    say "S7 release-wait: published with $n assets; formula hashes match the published checksums"
  fi
  web_pr
}
