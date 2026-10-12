# shellcheck shell=bash
# Purpose: release steps S1-S4 (preflight, admin lockstep PR, cli version PR, tag).
# Inputs:  helpers from common.sh; globals VERSION TAG PLAN WORK ADMIN_MERGE RELEASE_CREATE
# Outputs: step_* functions. Each prints plan rows in plan mode, otherwise detects
#          remote completion first (prints "S<n> <id> done") and then does the work.
# Constraints: merges only through admin-merge.sh (owner-answered); the tag only
#          through release-create.sh on the verified merge sha.

# checkout_branch <repo> <branch>: put the clone on the branch (remote copy if any).
checkout_branch() {
  ensure_clone "$1"
  if git -C "$WORK/$1" ls-remote --exit-code --heads origin "$2" >/dev/null 2>&1; then
    git -C "$WORK/$1" fetch -q origin "$2"
    git -C "$WORK/$1" checkout -q -B "$2" FETCH_HEAD
  else
    git -C "$WORK/$1" checkout -q -b "$2"
  fi
}

# commit_all <repo> <message>: the clone is ours, so `add -A` takes only the bump.
commit_all() {
  git -C "$WORK/$1" add -A
  if git -C "$WORK/$1" diff --cached --quiet; then return 1; fi
  git -C "$WORK/$1" commit -q -m "$2"
}

step_preflight() {
  local t v
  if [ "$PLAN" = 1 ]; then
    plan_line S1 preflight cli "gh auth status"
    for t in cli admin homebrew-nself; do
      plan_line S1 preflight "$t" "gh repo clone nself-org/$t $WORK/$t"
    done
    return 0
  fi
  for t in git jq curl gh; do
    command -v "$t" >/dev/null 2>&1 || die_step S1 preflight "missing tool: $t"
  done
  gh auth status >/dev/null 2>&1 || die_step S1 preflight "gh is not logged in (run: gh auth login)"
  for t in cli admin homebrew-nself; do ensure_clone "$t" || die_step S1 preflight "clone of $t failed"; done
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
    ensure_clone cli; ensure_clone admin
    bump_all || die_step S2 admin-lockstep "scripts/bump-version.sh failed in the cli clone"
    git -C "$WORK/admin" checkout -q -b "$br"
    commit_all admin "chore(release): bump admin to v${VERSION} (lockstep with cli)" \
      || die_step S2 admin-lockstep "bump changed nothing in admin"
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
    ensure_clone cli; ensure_clone admin
    bump_all || die_step S3 cli-version "scripts/bump-version.sh failed in the cli clone"
    if [ -d "$WORK/cli/tools/facts" ]; then
      (cd "$WORK/cli" && go run -mod=vendor ./tools/facts && go run -mod=vendor ./tools/facts -readme README.md) \
        || die_step S3 cli-version "facts regeneration failed"
    else
      say "S3 cli-version: tools/facts absent at this commit; no facts regeneration"
    fi
    git -C "$WORK/cli" checkout -q -b "$br"
    commit_all cli "chore(release): bump to v${VERSION}" || die_step S3 cli-version "bump changed nothing in cli"
  else
    checkout_branch cli "$br"
  fi
  pr_flow S3 cli-version cli "$br" cli_pr "chore(release): bump to v${VERSION}" \
    "Version bump for v${VERSION}, with facts regenerated when tools/facts exists. Opened by scripts/release/release.sh."
}

step_tag() {
  local sha merged cur
  if [ "$PLAN" = 1 ]; then
    plan_line S4 tag cli "git -C $WORK/cli fetch origin main; verify merged sha == git rev-parse origin/main"
    plan_line S4 tag cli "scripts/release-create.sh $TAG -R nself-org/cli --target <sha> --latest=false --title $TAG --notes <in progress; release.yml publishes the assets>"
    return 0
  fi
  if tag_exists; then step_done S4 tag; return 0; fi
  step_confirm S4 tag
  ensure_clone cli
  git -C "$WORK/cli" fetch -q origin main
  sha=$(git -C "$WORK/cli" rev-parse origin/main)
  merged=$(gh pr list -R nself-org/cli --head "release/v${VERSION}" --state merged --json mergeCommit \
    --jq '.[0].mergeCommit.oid // empty' || true)
  if [ -n "$merged" ] && [ "$merged" != "$sha" ]; then
    die_step S4 tag "origin/main ($sha) moved past the merged release commit ($merged); inspect, then tag by hand"
  fi
  cur=$(git -C "$WORK/cli" show origin/main:.github/VERSION | tr -d '[:space:]')
  [ "$cur" = "$VERSION" ] || die_step S4 tag "origin/main carries version $cur, not $VERSION (S3 not merged?)"
  (cd "$WORK/cli" && bash "$RELEASE_CREATE" "$TAG" -R nself-org/cli --target "$sha" --latest=false \
    --title "$TAG" --notes "In progress: release.yml publishes the assets.") \
    || die_step S4 tag "release-create.sh refused or failed (denylist?)"
  cur=$(git -C "$WORK/cli" ls-remote origin "refs/tags/$TAG" | awk '{print $1}')
  [ "$cur" = "$sha" ] || die_step S4 tag "tag $TAG on the remote is '$cur', expected $sha"
  state_set tag_sha "$sha"
  say "S4 tag: $TAG -> $sha"
}
