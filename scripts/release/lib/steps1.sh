# shellcheck shell=bash
# Purpose: release steps S1-S4 (preflight, admin lockstep PR, cli version PR, tag).
# Inputs:  helpers from common.sh; globals VERSION TAG PLAN WORK ADMIN_MERGE RELEASE_CREATE
# Outputs: step_* functions. Each prints plan rows in plan mode, otherwise detects
#          remote completion first (prints "S<n> <id> done") and then does the work.
# Constraints: merges only through admin-merge.sh (owner-answered); the tag is an
#          annotated tag pushed from the cli clone on the verified merge sha, after
#          the tag-time denylist rules pass (release.yml's Pre-Release Gate runs all five).

# stale_pr <S#> <id> <repo> <branch>: a PR on the branch that cannot be reused.
# Closed unmerged, or merged while main lacks the bump (reverted): stop with the fix.
stale_pr() {
  case "$PR_INFO" in
    *CLOSED) die_step "$1" "$2" "PR ${PR_INFO%% *} on $3 was closed unmerged; delete branch $4 (gh api -X DELETE repos/nself-org/$3/git/refs/heads/$4) and retry" ;;
    *MERGED) die_step "$1" "$2" "PR ${PR_INFO%% *} on $3 is merged but main does not carry v${VERSION} (the bump was reverted or main was reset); delete branch $4 (gh api -X DELETE repos/nself-org/$3/git/refs/heads/$4) and re-run to open a fresh PR" ;;
  esac
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
  local br="release/v${VERSION}-lockstep"
  if [ "$PLAN" = 1 ]; then
    plan_line S2 admin-lockstep admin "scripts/bump-version.sh $VERSION (cwd $WORK/cli; edits admin package.json, src/lib/cli-version.ts, Dockerfile, .github/wiki/Home.md)"
    plan_line S2 admin-lockstep admin "git -C $WORK/admin checkout -b $br; git commit; git push origin $br"
    plan_line S2 admin-lockstep admin "gh pr create -R nself-org/admin --base main --head $br"
    plan_line S2 admin-lockstep admin "gh pr checks N -R nself-org/admin --required (polled, 45 min budget)"
    plan_line S2 admin-lockstep admin "scripts/admin-merge.sh --repo nself-org/admin --pr N"
    return 0
  fi
  gh_need "admin package.json" api -H 'Accept: application/vnd.github.raw' repos/nself-org/admin/contents/package.json
  if [ "$(printf '%s' "$GH_OUT" | jq -r .version 2>/dev/null || true)" = "$VERSION" ]; then
    step_done S2 admin-lockstep; return 0
  fi
  pr_info admin "$br"
  stale_pr S2 admin-lockstep admin "$br"
  step_confirm S2 admin-lockstep
  if [ -z "$PR_INFO" ]; then
    ensure_clone cli; ensure_clone admin; reset_clone cli; reset_clone admin
    bump_all --no-goldens || die_step S2 admin-lockstep "scripts/bump-version.sh failed in the cli clone"
    git -C "$WORK/admin" checkout -q -B "$br" || die_step S2 admin-lockstep "could not create branch $br"
    commit_all admin "chore(release): bump admin to v${VERSION} (lockstep with cli)" \
      || die_step S2 admin-lockstep "bump changed nothing in admin"
    reset_clone cli   # the bump also edited cli files; S3 starts clean and re-runs it
  else
    checkout_branch admin "$br"
  fi
  pr_flow S2 admin-lockstep admin "$br" admin_pr "chore(release): bump admin to v${VERSION}" \
    "Lockstep bump for the cli v${VERSION} release (package.json, src/lib/cli-version.ts, Dockerfile, .github/wiki/Home.md). Opened by scripts/release/release.sh."
}

step_cli_version() {
  local br="release/v${VERSION}"
  if [ "$PLAN" = 1 ]; then
    plan_line S3 cli-version cli "scripts/bump-version.sh $VERSION (cwd $WORK/cli; also regenerates the 4 version-hash goldens: UPDATE_GOLDEN=1 go test ./internal/build ./cmd/commands, then re-runs them plain)"
    plan_line S3 cli-version cli "go run -mod=vendor ./tools/facts && go run -mod=vendor ./tools/facts -readme README.md (facts regeneration, only when tools/facts exists)"
    plan_line S3 cli-version cli "git -C $WORK/cli checkout -b $br; git commit; git push origin $br; gh pr create -R nself-org/cli --base main --head $br"
    plan_line S3 cli-version cli "gh pr checks N -R nself-org/cli --required (polled, 45 min budget)"
    plan_line S3 cli-version cli "scripts/admin-merge.sh --repo nself-org/cli --pr N"
    return 0
  fi
  gh_need "cli .github/VERSION" api -H 'Accept: application/vnd.github.raw' repos/nself-org/cli/contents/.github/VERSION
  if [ "$(printf '%s' "$GH_OUT" | tr -d '[:space:]')" = "$VERSION" ]; then
    step_done S3 cli-version; return 0
  fi
  pr_info cli "$br"
  stale_pr S3 cli-version cli "$br"
  step_confirm S3 cli-version
  if [ -z "$PR_INFO" ]; then
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
