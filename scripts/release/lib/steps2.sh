# shellcheck shell=bash
# Purpose: release steps S5-S7 (build wait + checksums, homebrew formula PR,
#          release wait + web lock PR).
# Inputs:  helpers from common.sh and steps1.sh; globals VERSION TAG PLAN WORK
# Outputs: step_* functions. release.yml is the engine; this only waits on it.
# Constraints: the formula is written by homebrew-nself's own update-formula.sh from
#          the build's checksums file (both darwin hashes, no release download).

RUN_ID=""

run_present() { [ -n "$(release_run_id)" ]; }

# build_state <run-id>: success | failure | pending for the build job.
build_state() {
  gh run view "$1" -R nself-org/cli --json jobs --jq '
    [.jobs[] | select(.name == "Build release artifacts" or .name == "build")][0]
    | if . == null or .status != "completed" then "pending" else .conclusion end'
}
build_finished() { [ "$(build_state "$RUN_ID")" != pending ]; }

# release_state <run-id>: success | failure | pending for the whole run.
release_state() {
  gh run view "$1" -R nself-org/cli --json status,conclusion \
    --jq 'if .status != "completed" then "pending" else .conclusion end'
}
release_finished() { [ "$(release_state "$RUN_ID")" != pending ]; }

step_build_wait() {
  local s early=0 f
  if [ "$PLAN" = 1 ]; then
    plan_line S5 build-wait cli "gh run list -R nself-org/cli --workflow release.yml --branch $TAG"
    plan_line S5 build-wait cli "gh run view <run-id> -R nself-org/cli --json jobs (wait for the build job)"
    plan_line S5 build-wait cli "gh run download <run-id> -R nself-org/cli -n release-checksums -D $WORK/checksums"
    return 0
  fi
  poll "release.yml run for $TAG" 30 run_present \
    || die_step S5 build-wait "no release.yml run appeared for $TAG (check the Actions tab of nself-org/cli)"
  RUN_ID=$(release_run_id)
  s=$(build_state "$RUN_ID")
  if [ "$s" = success ]; then
    early=1
  elif [ "$s" = pending ]; then
    poll "build job of run $RUN_ID" 120 build_finished || die_step S5 build-wait "build job still running after the wait budget"
    s=$(build_state "$RUN_ID")
  fi
  [ "$s" = success ] || die_step S5 build-wait "build job concluded '$s' in run $RUN_ID; fix it, then re-run the failed jobs"
  f=$(ensure_checksums) || die_step S5 build-wait "could not fetch checksums.txt (artifact release-checksums and release asset both missing)"
  if [ "$early" = 1 ]; then step_done S5 build-wait; else say "S5 build-wait: build ok, checksums at $f"; fi
}

formula_version() {
  gh_raw homebrew-nself Formula/nself.rb | sed -n 's/^[[:space:]]*version "\([^"]*\)".*/\1/p' | head -1
}

# rerun_lockstep: once the formula is on main, re-run the lockstep job that failed.
rerun_lockstep() {
  local id c
  id=$(release_run_id)
  if [ -z "$id" ]; then return 0; fi
  RUN_ID=$id
  poll "run $id to settle" 60 release_finished || return 0
  c=$(release_state "$id")
  if [ "$c" != success ]; then
    gh run rerun "$id" -R nself-org/cli --failed || die_step S6 homebrew-formula "could not re-run the failed jobs of run $id"
    say "S6 homebrew-formula: re-ran failed jobs of run $id"
    sleep "$POLL_SECONDS"
  fi
}

step_homebrew_formula() {
  local br="release/nself-v${VERSION}" info f
  if [ "$PLAN" = 1 ]; then
    plan_line S6 homebrew-formula homebrew-nself "bash .github/scripts/update-formula.sh --version $TAG --checksums-file $WORK/checksums/checksums.txt (cwd $WORK/homebrew-nself; writes on_arm and on_intel)"
    plan_line S6 homebrew-formula homebrew-nself "git checkout -b $br; git commit; git push origin $br; gh pr create -R nself-org/homebrew-nself --base main --head $br"
    plan_line S6 homebrew-formula homebrew-nself "gh pr checks <n> -R nself-org/homebrew-nself --required --watch"
    plan_line S6 homebrew-formula homebrew-nself "scripts/admin-merge.sh --repo nself-org/homebrew-nself --pr N"
    plan_line S6 homebrew-formula cli "gh run rerun <run-id> -R nself-org/cli --failed (the verify-homebrew-lockstep job)"
    return 0
  fi
  if [ "$(formula_version)" = "$VERSION" ]; then step_done S6 homebrew-formula; return 0; fi
  info=$(pr_info homebrew-nself "$br")
  case "$info" in *CLOSED) die_step S6 homebrew-formula "PR ${info%% *} was closed unmerged; delete branch $br and retry" ;; esac
  step_confirm S6 homebrew-formula
  if [ -z "$info" ]; then
    f=$(ensure_checksums) || die_step S6 homebrew-formula "no checksums.txt (run S5 first)"
    ensure_clone homebrew-nself
    git -C "$WORK/homebrew-nself" checkout -q -b "$br"
    (cd "$WORK/homebrew-nself" && bash .github/scripts/update-formula.sh --version "$TAG" --checksums-file "$f") \
      || die_step S6 homebrew-formula "update-formula.sh refused the checksums"
    commit_all homebrew-nself "chore(formula): nself ${TAG}" || die_step S6 homebrew-formula "formula unchanged"
  else
    checkout_branch homebrew-nself "$br"
  fi
  pr_flow S6 homebrew-formula homebrew-nself "$br" brew_pr "chore(formula): nself ${TAG}" \
    "Formula for cli ${TAG}, written by .github/scripts/update-formula.sh from the build run's checksums.txt (both darwin hashes). Opened by scripts/release/release.sh in the cli repo."
  rerun_lockstep
}

web_pr() {
  local br="release/cli-v${VERSION}" info url
  info=$(pr_info web "$br")
  if [ -n "$info" ]; then say "S7 release-wait web done (PR #${info%% *}, ${info##* })"; return 0; fi
  step_confirm S7 release-wait
  ensure_clone web
  git -C "$WORK/web" checkout -q -b "$br"
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

step_release_wait() {
  local s early=0 assets
  if [ "$PLAN" = 1 ]; then
    plan_line S7 release-wait cli "gh run view <run-id> -R nself-org/cli --json status,conclusion (wait for success); gh release view $TAG -R nself-org/cli --json assets"
    plan_line S7 release-wait web "node org/scripts/sources/update-lock.mjs cli $TAG (fresh web clone); gh pr create -R nself-org/web --head release/cli-v${VERSION} (owner merges; no deploy)"
    return 0
  fi
  RUN_ID=$(release_run_id)
  [ -n "$RUN_ID" ] || die_step S7 release-wait "no release.yml run found for $TAG"
  s=$(release_state "$RUN_ID")
  if [ "$s" = success ]; then early=1; fi
  if [ "$s" = pending ]; then
    poll "release run $RUN_ID" 120 release_finished || die_step S7 release-wait "release run $RUN_ID still running after the wait budget"
    s=$(release_state "$RUN_ID")
  fi
  [ "$s" = success ] || die_step S7 release-wait "release run $RUN_ID concluded '$s' (after S6, re-run failed jobs: gh run rerun $RUN_ID -R nself-org/cli --failed)"
  assets=$(gh release view "$TAG" -R nself-org/cli --json assets --jq '.assets[].name')
  printf '%s\n' "$assets" | grep -qx 'checksums.txt' || die_step S7 release-wait "release $TAG has no checksums.txt asset"
  if [ "$early" = 1 ]; then step_done S7 release-wait; else
    say "S7 release-wait: published with $(printf '%s\n' "$assets" | wc -l | tr -d ' ') assets"
  fi
  web_pr
}
