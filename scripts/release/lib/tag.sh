# shellcheck shell=bash
# Purpose: release step S4 (tag-time denylist rules, then the annotated tag).
# Inputs:  helpers from common.sh; globals VERSION TAG PLAN WORK
# Outputs: step_tag. The tag is annotated, made on the verified release commit and
#          pushed from the cli clone; it is never moved.
# Constraints: DENY-001/002 need the build output and are evaluated by release.yml's
#          Pre-Release Gate (the `denylist` job), which re-checks all five rules.
#          DENY-003/004/005 are evaluated here so a doomed tag is never pushed.

LIC=""; LSCOPE=main

# lic_state: LIC = pending | success | <other conclusion> | none, for the newest
# license-audit.yml run on main (LSCOPE=main) or on any branch (LSCOPE=any, the
# exact query pre-release-gate.yml uses).
lic_state() {
  local br="--branch main"
  if [ "$LSCOPE" = any ]; then br=""; fi
  # shellcheck disable=SC2016,SC2086
  gh_need "the license-audit.yml runs" run list -R nself-org/cli --workflow license-audit.yml $br --limit 1 \
    --json status,conclusion --jq '.[0] | if . == null then "none" elif .status != "completed" then "pending" else .conclusion end'
  LIC=$GH_OUT
}
lic_settled() { lic_state; [ "$LIC" != pending ]; }
# lic_wait <main|any>: LIC after waiting (30 min at most) for an in-flight run.
lic_wait() {
  # shellcheck disable=SC2034  # read by poll() through dynamic scope
  local POLL_SECONDS=30
  LSCOPE=$1
  lic_state
  if [ "$LIC" = pending ]; then
    say "S4 tag: license-audit.yml ($1) is still running; waiting up to 30 minutes"
    poll "license-audit.yml run ($1)" 60 lic_settled || true
    lic_state
  fi
}

# deny_checks <sha>: DENY-003/004/005 for the commit about to be tagged.
deny_checks() {
  local sha=$1 prior cnt
  prior=$(git -C "$WORK/cli" tag --sort=-version:refname | grep -v "^${TAG}\$" | head -1 || true)
  if [ -n "$prior" ]; then
    cnt=$(git -C "$WORK/cli" rev-list --count "$prior..$sha") || die_step S4 tag "cannot count commits since $prior"
    [ "$cnt" -gt 0 ] || die_step S4 tag "DENY-003: no commits since $prior; merge the version bump (S3) first"
  fi
  lic_wait main
  case "$LIC" in
    success) ;;
    pending) die_step S4 tag "DENY-004: license-audit.yml on main is still running after 30 minutes; let it finish, then --from S4" ;;
    none) die_step S4 tag "DENY-004: no license-audit.yml run on main. Start one: gh workflow run license-audit.yml -R nself-org/cli --ref main, wait for it to pass, then --from S4" ;;
    *) die_step S4 tag "DENY-004: the newest license-audit.yml run on main is '$LIC'. Fix the flagged licenses, run: gh workflow run license-audit.yml -R nself-org/cli --ref main, wait for it to pass, then --from S4" ;;
  esac
  lic_wait any
  case "$LIC" in
    success|none) ;;
    pending) die_step S4 tag "DENY-004: a license-audit.yml run is still in flight after 30 minutes; the Pre-Release Gate reads the newest run on any branch. Let it finish, then --from S4" ;;
    *) die_step S4 tag "DENY-004: pre-release-gate.yml reads the newest license-audit.yml run on ANY branch and it is '$LIC' (a PR-branch run?), so the gate would fail after the tag is pushed. Start a newer run: gh workflow run license-audit.yml -R nself-org/cli --ref main, wait for it to pass, then --from S4" ;;
  esac
  # The gate reads every milestone titled exactly $TAG (any state) and needs the result
  # to be exactly "closed": one milestone, closed.
  gh_need "the cli milestones" api "repos/nself-org/cli/milestones?state=all&per_page=100" --paginate \
    --jq ".[] | select(.title == \"$TAG\") | .state"
  [ -n "$GH_OUT" ] || die_step S4 tag "DENY-005: no closed milestone titled exactly $TAG (a milestone named $VERSION does not count); create or close it, then --from S4"
  if printf '%s\n' "$GH_OUT" | grep -qv '^closed$'; then
    die_step S4 tag "DENY-005: a milestone titled $TAG is not closed (an open duplicate?); close or delete the duplicate so exactly one closed milestone has that title, then --from S4"
  fi
  [ "$(printf '%s\n' "$GH_OUT" | wc -l | tr -d ' ')" = 1 ] \
    || die_step S4 tag "DENY-005: more than one milestone is titled $TAG; pre-release-gate.yml needs exactly one (closed). Delete the duplicates, then --from S4"
}

step_tag() {
  local sha target cur merged
  if [ "$PLAN" = 1 ]; then
    plan_line S4 tag cli "gh pr list -R nself-org/cli --head release/v${VERSION} --state merged --json mergeCommit (the S3 merge commit)"
    plan_line S4 tag cli "git -C $WORK/cli fetch --tags origin main; tag origin/main, or the merge commit when main has moved on but still contains it; its .github/VERSION must be $VERSION"
    plan_line S4 tag cli "git -C $WORK/cli rev-list --count PRIOR_TAG..SHA (DENY-003: more than 0 commits since the prior tag)"
    plan_line S4 tag cli "gh run list -R nself-org/cli --workflow license-audit.yml --branch main --limit 1 (DENY-004: waits up to 30 min while running; success required)"
    plan_line S4 tag cli "gh run list -R nself-org/cli --workflow license-audit.yml --limit 1 (DENY-004 as pre-release-gate.yml reads it: any branch)"
    plan_line S4 tag cli "gh api repos/nself-org/cli/milestones?state=all&per_page=100 --paginate (DENY-005: exactly one milestone titled $TAG, and it is closed)"
    plan_line S4 tag cli "git -C $WORK/cli tag -a $TAG -m $TAG SHA && git -C $WORK/cli push origin refs/tags/$TAG"
    return 0
  fi
  gh_need "the release PR" pr list -R nself-org/cli --head "release/v${VERSION}" --state merged --json mergeCommit \
    --jq '.[0].mergeCommit.oid // empty'
  merged=$GH_OUT
  if repo_tag_sha cli "$TAG"; then
    sha=$TAG_SHA
    if [ -n "$merged" ]; then cur=$merged; else
      cur=$sha
      gh_need "the VERSION file at $TAG" api -H 'Accept: application/vnd.github.raw' "repos/nself-org/cli/contents/.github/VERSION?ref=$TAG"
      [ "$(printf '%s' "$GH_OUT" | tr -d '[:space:]')" = "$VERSION" ] \
        || die_step S4 tag "tag $TAG exists but its .github/VERSION is not $VERSION; tags are never moved, inspect by hand"
    fi
    [ "$sha" = "$cur" ] || die_step S4 tag "tag $TAG is at $sha but the merged release commit is $cur; tags are never moved, inspect by hand"
    step_done S4 tag; return 0
  fi
  step_confirm S4 tag
  ensure_clone cli
  reset_clone cli
  sha=$(git -C "$WORK/cli" rev-parse origin/main)
  target=$sha
  if [ -n "$merged" ] && [ "$merged" != "$sha" ]; then
    git -C "$WORK/cli" merge-base --is-ancestor "$merged" origin/main \
      || die_step S4 tag "the merged release commit $merged is not in origin/main's history (rewritten or reverted); inspect before tagging"
    say "S4 tag: origin/main ($sha) moved on; tagging the reviewed release commit $merged"
    target=$merged
  fi
  cur=$(git -C "$WORK/cli" show "$target:.github/VERSION" | tr -d '[:space:]') || die_step S4 tag "cannot read .github/VERSION at $target"
  [ "$cur" = "$VERSION" ] || die_step S4 tag "$target carries version $cur, not $VERSION (S3 not merged?)"
  deny_checks "$target"
  if [ "$(git -C "$WORK/cli" rev-parse -q --verify "refs/tags/$TAG^{commit}" || true)" != "$target" ]; then
    git -C "$WORK/cli" tag -a "$TAG" -m "$TAG" "$target" || die_step S4 tag "could not create the local tag (a stale $TAG in $WORK/cli, or git user.name / user.email unset?)"
  fi
  git -C "$WORK/cli" push -q origin "refs/tags/$TAG" || die_step S4 tag "push of $TAG was refused"
  cur=$(git -C "$WORK/cli" ls-remote origin "refs/tags/$TAG^{}" | awk '{print $1}')
  [ "$cur" = "$target" ] || die_step S4 tag "tag $TAG on the remote is '$cur', expected $target"
  state_set tag_sha "$target"
  say "S4 tag: $TAG -> $target (release.yml creates the release)"
}
