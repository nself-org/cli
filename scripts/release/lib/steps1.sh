# shellcheck shell=bash
# Purpose: release steps S1-S4 (preflight, admin lockstep PR, cli version PR, tag).
# Inputs:  helpers from common.sh; globals VERSION TAG PLAN WORK ADMIN_MERGE RELEASE_CREATE
# Outputs: step_* functions. Each prints plan rows in plan mode, otherwise detects
#          remote completion first (prints "S<n> <id> done") and then does the work.
# Constraints: merges only through admin-merge.sh (owner-answered); the tag is an
#          annotated tag pushed from the cli clone on the verified merge sha, after
#          the tag-time denylist rules pass (release.yml's Pre-Release Gate runs all five).

step_preflight() {
  local t v
  if [ "$PLAN" = 1 ]; then
    plan_line S1 preflight cli "gh auth status"
    for t in cli admin homebrew-nself; do
      plan_line S1 preflight "$t" "gh repo clone nself-org/$t $WORK/$t"
    done
    return 0
  fi
  for t in git jq curl gh node; do
    command -v "$t" >/dev/null 2>&1 || die_step S1 preflight "missing tool: $t"
  done
  gh auth status >/dev/null 2>&1 || die_step S1 preflight "gh is not logged in (run: gh auth login)"
  for t in cli admin homebrew-nself; do ensure_clone "$t" || die_step S1 preflight "clone of $t failed"; done
  if [ -d "$WORK/cli/tools/facts" ]; then
    command -v go >/dev/null 2>&1 || die_step S1 preflight "missing tool: go (cli has tools/facts; S3 regenerates facts)"
  fi
  v=$(tr -d '[:space:]' < "$WORK/cli/.github/VERSION")
  say "S1 preflight: cli main is at $v, releasing $VERSION; work dir $WORK"
  if ! gh api repos/nself-org/cli/actions/organization-secrets --jq '.secrets[].name' 2>/dev/null | grep -q DOCKERHUB_TOKEN; then
    err "S1 preflight: warning: DOCKERHUB_TOKEN not visible to nself-org/cli (P7-PROD-61); S8/S9 will fail without it"
  fi
}

step_admin_lockstep() {
  local br="release/v${VERSION}-lockstep" info
  if [ "$PLAN" = 1 ]; then
    plan_line S2 admin-lockstep admin "scripts/bump-version.sh $VERSION (cwd $WORK/cli; edits admin package.json, src/lib/cli-version.ts, Dockerfile)"
    plan_line S2 admin-lockstep admin "git -C $WORK/admin checkout -b $br; git commit; git push origin $br"
    plan_line S2 admin-lockstep admin "gh pr create -R nself-org/admin --base main --head $br"
    plan_line S2 admin-lockstep admin "gh pr checks <n> -R nself-org/admin --required --watch"
    plan_line S2 admin-lockstep admin "scripts/admin-merge.sh --repo nself-org/admin --pr N"
    return 0
  fi
  if [ "$(gh_raw admin package.json | jq -r .version 2>/dev/null || true)" = "$VERSION" ]; then
    step_done S2 admin-lockstep; return 0
  fi
  info=$(pr_info admin "$br")
  case "$info" in *CLOSED) die_step S2 admin-lockstep "PR ${info%% *} on admin was closed unmerged; delete branch $br and retry" ;; esac
  step_confirm S2 admin-lockstep
  if [ -z "$info" ]; then
    ensure_clone cli; ensure_clone admin; reset_clone cli; reset_clone admin
    bump_all || die_step S2 admin-lockstep "scripts/bump-version.sh failed in the cli clone"
    git -C "$WORK/admin" checkout -q -B "$br" || die_step S2 admin-lockstep "could not create branch $br"
    commit_all admin "chore(release): bump admin to v${VERSION} (lockstep with cli)" \
      || die_step S2 admin-lockstep "bump changed nothing in admin"
    reset_clone cli   # the bump also edited cli files; S3 starts clean and re-runs it
  else
    checkout_branch admin "$br"
  fi
  pr_flow S2 admin-lockstep admin "$br" admin_pr "chore(release): bump admin to v${VERSION}" \
    "Lockstep bump for the cli v${VERSION} release (package.json, src/lib/cli-version.ts, Dockerfile). Opened by scripts/release/release.sh."
}

step_cli_version() {
  local br="release/v${VERSION}" info
  if [ "$PLAN" = 1 ]; then
    plan_line S3 cli-version cli "scripts/bump-version.sh $VERSION (cwd $WORK/cli)"
    plan_line S3 cli-version cli "go run -mod=vendor ./tools/facts && go run -mod=vendor ./tools/facts -readme README.md (facts regeneration, only when tools/facts exists)"
    plan_line S3 cli-version cli "git -C $WORK/cli checkout -b $br; git commit; git push origin $br; gh pr create -R nself-org/cli --base main --head $br"
    plan_line S3 cli-version cli "gh pr checks <n> -R nself-org/cli --required --watch"
    plan_line S3 cli-version cli "scripts/admin-merge.sh --repo nself-org/cli --pr N"
    return 0
  fi
  if [ "$(gh_raw cli .github/VERSION | tr -d '[:space:]' || true)" = "$VERSION" ]; then
    step_done S3 cli-version; return 0
  fi
  info=$(pr_info cli "$br")
  case "$info" in *CLOSED) die_step S3 cli-version "PR ${info%% *} on cli was closed unmerged; delete branch $br and retry" ;; esac
  step_confirm S3 cli-version
  if [ -z "$info" ]; then
    ensure_clone cli; ensure_clone admin; reset_clone cli; reset_clone admin
    bump_all || die_step S3 cli-version "scripts/bump-version.sh failed in the cli clone"
    if [ -d "$WORK/cli/tools/facts" ]; then
      command -v go >/dev/null 2>&1 || die_step S3 cli-version "go is not installed (needed for tools/facts)"
      (cd "$WORK/cli" && go run -mod=vendor ./tools/facts && go run -mod=vendor ./tools/facts -readme README.md) \
        || die_step S3 cli-version "facts regeneration failed"
    else
      say "S3 cli-version: tools/facts absent at this commit; no facts regeneration"
    fi
    git -C "$WORK/cli" checkout -q -B "$br" || die_step S3 cli-version "could not create branch $br"
    commit_all cli "chore(release): bump to v${VERSION}" || die_step S3 cli-version "bump changed nothing in cli"
  else
    checkout_branch cli "$br"
  fi
  pr_flow S3 cli-version cli "$br" cli_pr "chore(release): bump to v${VERSION}" \
    "Version bump for v${VERSION}, with facts regenerated when tools/facts exists. Opened by scripts/release/release.sh."
}

# merged_sha: the merge commit of the release/vX.Y.Z PR on cli (empty when none).
merged_sha() {
  gh pr list -R nself-org/cli --head "release/v${VERSION}" --state merged --json mergeCommit \
    --jq '.[0].mergeCommit.oid // empty'
}

# deny_checks <sha>: the tag-time denylist rules (DENY-003/004/005), each a GET or a
# local git read. DENY-001/002 need the build output and are evaluated by
# release.yml's Pre-Release Gate (the `denylist` job), which also re-checks all five.
deny_checks() {
  local sha=$1 prior cnt c ms
  prior=$(git -C "$WORK/cli" tag --sort=-version:refname | grep -v "^${TAG}\$" | head -1 || true)
  if [ -n "$prior" ]; then
    cnt=$(git -C "$WORK/cli" rev-list --count "$prior..$sha")
    [ "$cnt" -gt 0 ] || die_step S4 tag "DENY-003: no commits since $prior; merge the version bump (S3) first"
  fi
  c=$(gh run list -R nself-org/cli --workflow license-audit.yml --branch main --limit 1 --json conclusion \
    --jq '.[0].conclusion // empty') || die_step S4 tag "cannot read the license-audit.yml runs"
  [ "$c" = success ] || die_step S4 tag "DENY-004: latest license-audit.yml run on main is '${c:-none}'; fix the flagged licenses and let it pass, then --from S4"
  ms=$(gh api "repos/nself-org/cli/milestones?state=closed&per_page=100" --paginate \
    --jq ".[] | select(.title == \"$TAG\" or .title == \"$VERSION\") | .title") \
    || die_step S4 tag "cannot read the cli milestones"
  [ -n "$ms" ] || die_step S4 tag "DENY-005: no closed milestone titled $TAG or $VERSION (either is accepted); close it, then --from S4"
}

step_tag() {
  local sha merged cur
  if [ "$PLAN" = 1 ]; then
    plan_line S4 tag cli "gh api repos/nself-org/cli/milestones?state=closed (DENY-005: a closed milestone titled $TAG or $VERSION)"
    plan_line S4 tag cli "gh run list -R nself-org/cli --workflow license-audit.yml --branch main --limit 1 (DENY-004: conclusion success)"
    plan_line S4 tag cli "git -C $WORK/cli rev-list --count PRIOR_TAG..SHA (DENY-003: more than 0 commits since the prior tag)"
    plan_line S4 tag cli "git -C $WORK/cli fetch origin main; verify merged sha == git rev-parse origin/main"
    plan_line S4 tag cli "git -C $WORK/cli tag -a $TAG -m $TAG SHA && git -C $WORK/cli push origin refs/tags/$TAG"
    return 0
  fi
  merged=$(merged_sha) || die_step S4 tag "cannot read the release PR"
  if tag_exists; then
    sha=$(gh api "repos/nself-org/cli/commits/$TAG" --jq .sha) || die_step S4 tag "cannot resolve $TAG"
    if [ -n "$merged" ]; then cur=$merged; else
      cur=$sha
      [ "$(gh_raw cli ".github/VERSION?ref=$TAG" | tr -d '[:space:]')" = "$VERSION" ] \
        || die_step S4 tag "tag $TAG exists but its .github/VERSION is not $VERSION; tags are never moved, inspect by hand"
    fi
    [ "$sha" = "$cur" ] || die_step S4 tag "tag $TAG is at $sha but the merged release commit is $cur; tags are never moved, inspect by hand"
    step_done S4 tag; return 0
  fi
  step_confirm S4 tag
  ensure_clone cli
  reset_clone cli
  sha=$(git -C "$WORK/cli" rev-parse origin/main)
  if [ -n "$merged" ] && [ "$merged" != "$sha" ]; then
    die_step S4 tag "origin/main ($sha) moved past the merged release commit ($merged); inspect, then tag by hand"
  fi
  cur=$(git -C "$WORK/cli" show origin/main:.github/VERSION | tr -d '[:space:]')
  [ "$cur" = "$VERSION" ] || die_step S4 tag "origin/main carries version $cur, not $VERSION (S3 not merged?)"
  deny_checks "$sha"
  if [ "$(git -C "$WORK/cli" rev-parse -q --verify "refs/tags/$TAG^{commit}" || true)" != "$sha" ]; then
    git -C "$WORK/cli" tag -a "$TAG" -m "$TAG" "$sha" || die_step S4 tag "could not create the local tag (a stale $TAG in $WORK/cli? remove the clone)"
  fi
  git -C "$WORK/cli" push -q origin "refs/tags/$TAG" || die_step S4 tag "push of $TAG was refused"
  cur=$(git -C "$WORK/cli" ls-remote origin "refs/tags/$TAG^{}" | awk '{print $1}')
  [ "$cur" = "$sha" ] || die_step S4 tag "tag $TAG on the remote is '$cur', expected $sha"
  state_set tag_sha "$sha"
  say "S4 tag: $TAG -> $sha (release.yml creates the release)"
}
