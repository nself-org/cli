# shellcheck shell=bash
# Purpose: release steps S5-S6 (build wait + checksums, homebrew formula PR).
# Inputs:  helpers from common.sh, checks.sh, hashes.sh; globals VERSION TAG PLAN WORK
# Outputs: step_* functions. release.yml is the engine; this only waits on it.
# Constraints: the formula is written by homebrew-nself's own update-formula.sh from
#          the release's checksums file (both darwin hashes) and is merged only
#          while the release can still publish.

BUILD_ST=""; REL_ST=""

# build_state <run-id>: BUILD_ST = success | failure | pending for the build job.
build_state() {
  gh_need "the jobs of run $1" run view "$1" -R nself-org/cli --json jobs --jq '
    [.jobs[] | select(.name == "Build release artifacts" or .name == "build")][0]
    | if . == null or .status != "completed" then "pending" else .conclusion end'
  BUILD_ST=$GH_OUT
}
build_finished() { build_state "$RUN_ID"; [ "$BUILD_ST" != pending ]; }

# release_state <run-id>: REL_ST = success | failure | pending for the whole run.
release_state() {
  gh_need "run $1" run view "$1" -R nself-org/cli --json status,conclusion \
    --jq 'if .status != "completed" then "pending" else .conclusion end'
  REL_ST=$GH_OUT
}
release_finished() { release_state "$RUN_ID"; [ "$REL_ST" != pending ]; }

no_checksums() {
  die_step "$CUR_S" "$CUR_ID" "no checksums.txt: the release-checksums artifact of run ${RUN_ID:-?} has expired (retention is 1 day) and the release is not published. Re-run the WHOLE run (gh run rerun ${RUN_ID:-ID} -R nself-org/cli, not --failed), wait for its build to finish (checksums are fetched fresh each time, nothing cached to clear)" S5
}

step_build_wait() {
  local early=0
  if [ "$PLAN" = 1 ]; then
    plan_line S5 build-wait cli "gh run list -R nself-org/cli --workflow release.yml --branch $TAG (the run whose headSha is the tag commit)"
    plan_line S5 build-wait cli "gh run view RUN_ID -R nself-org/cli --json jobs (wait for the build job)"
    plan_line S5 build-wait cli "gh run download RUN_ID -R nself-org/cli -n release-checksums -D $WORK/checksums"
    return 0
  fi
  poll "release.yml run for $TAG" 30 run_present \
    || die_step S5 build-wait "no release.yml run appeared for $TAG (check the Actions tab of nself-org/cli)"
  build_state "$RUN_ID"
  if [ "$BUILD_ST" = success ]; then
    early=1
  elif [ "$BUILD_ST" = pending ]; then
    poll "build job of run $RUN_ID" 120 build_finished || die_step S5 build-wait "build job still running after the wait budget"
  fi
  build_state "$RUN_ID"
  [ "$BUILD_ST" = success ] || die_step S5 build-wait "build job concluded '$BUILD_ST' in run $RUN_ID; fix it, then re-run the failed jobs"
  ensure_checksums || no_checksums
  if [ "$early" = 1 ]; then step_done S5 build-wait; else say "S5 build-wait: build ok, checksums from $CHECKSUMS_SRC"; fi
}

# rerun_lockstep <wait 0|1>: with the formula on main, re-run the lockstep job of
# the release run if that job failed and the run is not successful. With wait=0 a
# run still in progress is left alone.
rerun_lockstep() {
  local c
  # The standalone tag-push gate runs before S6 can merge the formula.
  # Repair that expected failure as well as release.yml's embedded gate.
  gh_need "the standalone Homebrew gate for $TAG" run list -R nself-org/cli \
    --workflow homebrew-version-lockstep.yml --branch "$TAG" --event push \
    --json databaseId,status,conclusion \
    --jq '[.[] | select(.status == "completed" and .conclusion == "failure")][0].databaseId // empty'
  if [ -n "$GH_OUT" ]; then
    gh run rerun "$GH_OUT" -R nself-org/cli --failed \
      || die_step S6 homebrew-formula "could not re-run standalone Homebrew gate $GH_OUT"
    say "S6 homebrew-formula: re-ran standalone Homebrew gate $GH_OUT"
  fi
  find_run
  if [ -z "$RUN_ID" ]; then return 0; fi
  if [ "$1" = 1 ]; then poll "run $RUN_ID to settle" 60 release_finished || return 0; fi
  release_state "$RUN_ID"; c=$REL_ST
  if [ "$c" = success ] || [ "$c" = pending ]; then return 0; fi
  gh_need "the jobs of run $RUN_ID" run view "$RUN_ID" -R nself-org/cli --json jobs \
    --jq '[.jobs[] | select(.name | test("Homebrew"; "i")) | .conclusion] | first // empty'
  if [ "$GH_OUT" = failure ]; then
    gh run rerun "$RUN_ID" -R nself-org/cli --failed || die_step S6 homebrew-formula "could not re-run the failed jobs of run $RUN_ID"
    say "S6 homebrew-formula: re-ran failed jobs of run $RUN_ID"
    sleep "$POLL_SECONDS"
  else
    say "S6 homebrew-formula: run $RUN_ID concluded '$c' but its lockstep job did not fail; not re-running"
  fi
}

step_homebrew_formula() {
  local br arm intel
  if [ "$PLAN" = 1 ]; then
    plan_line S6 homebrew-formula cli "gh run view RUN_ID -R nself-org/cli --json jobs (wait until every job except Homebrew lockstep and Sign and publish succeeded)"
    plan_line S6 homebrew-formula homebrew-nself "bash .github/scripts/update-formula.sh --version $TAG --checksums-file $WORK/checksums/checksums.txt (cwd $WORK/homebrew-nself; writes on_arm and on_intel; the published checksums win once the release exists)"
    plan_line S6 homebrew-formula homebrew-nself "git checkout -B release/nself-v${VERSION}-ARM8; git commit; git push origin release/nself-v${VERSION}-ARM8; gh pr create -R nself-org/homebrew-nself --base main --head release/nself-v${VERSION}-ARM8"
    plan_line S6 homebrew-formula homebrew-nself "gh pr checks N -R nself-org/homebrew-nself --required (polled, 45 min budget)"
    plan_line S6 homebrew-formula homebrew-nself "scripts/admin-merge.sh --repo nself-org/homebrew-nself --pr N"
    plan_line S6 homebrew-formula cli "gh run rerun RUN_ID -R nself-org/cli --failed (the verify-homebrew-lockstep job)"
    return 0
  fi
  load_formula
  if ensure_checksums; then
    arm=$(sums_hash "$CHECKSUMS_FILE" arm64); intel=$(sums_hash "$CHECKSUMS_FILE" amd64)
    [ -n "$arm" ] && [ -n "$intel" ] || die_step S6 homebrew-formula "checksums.txt has no darwin arm64/amd64 lines"
    if [ "$F_VER" = "$VERSION" ] && [ "$F_ARM" = "$arm" ] && [ "$F_INTEL" = "$intel" ]; then
      step_done S6 homebrew-formula; rerun_lockstep 0; return 0
    fi
    [ "$F_VER" != "$VERSION" ] || say "S6 homebrew-formula: formula is at $VERSION but its hashes differ from $CHECKSUMS_SRC; rewriting"
  else
    if [ "$F_VER" = "$VERSION" ]; then
      say "S6 homebrew-formula: formula is at $VERSION; checksums are gone so the hashes cannot be re-checked here (S7 checks them against the published file)"
      step_done S6 homebrew-formula; rerun_lockstep 0; return 0
    fi
    no_checksums
  fi
  br="release/nself-v${VERSION}-${arm:0:8}"
  pr_info homebrew-nself "$br"
  case "$PR_INFO" in
    *CLOSED) die_step S6 homebrew-formula "PR ${PR_INFO%% *} was closed unmerged; delete branch $br (gh api -X DELETE repos/nself-org/homebrew-nself/git/refs/heads/$br) and retry" ;;
    *MERGED) die_step S6 homebrew-formula "PR ${PR_INFO%% *} for these hashes is merged but Formula/nself.rb on main does not carry them (reverted?); restore the formula by hand, then --from S6" ;;
  esac
  release_gate
  step_confirm S6 homebrew-formula
  if [ -z "$PR_INFO" ]; then
    ensure_clone homebrew-nself; reset_clone homebrew-nself
    git -C "$WORK/homebrew-nself" checkout -q -B "$br" || die_step S6 homebrew-formula "could not create branch $br"
    (cd "$WORK/homebrew-nself" && bash .github/scripts/update-formula.sh --version "$TAG" --checksums-file "$CHECKSUMS_FILE") \
      || die_step S6 homebrew-formula "update-formula.sh refused the checksums"
    commit_all homebrew-nself "chore(formula): nself ${TAG}" || die_step S6 homebrew-formula "formula unchanged"
  else
    checkout_branch homebrew-nself "$br"
  fi
  pr_flow S6 homebrew-formula homebrew-nself "$br" brew_pr "chore(formula): nself ${TAG}" \
    "Formula for cli ${TAG}, written by .github/scripts/update-formula.sh from ${CHECKSUMS_SRC} (both darwin hashes). Opened by scripts/release/release.sh in the cli repo."
  rerun_lockstep 1
}
