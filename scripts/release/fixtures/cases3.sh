# shellcheck shell=bash
# Purpose: third part of test.sh (sourced inside its pty block): license gate, moved
# main, hash integrity, release gate, bounded checks, S10, admin tag, stale PRs.

advance_main() { # <repo>: another merge lands on main
  git clone -q "$FIX/$1.git" "$T/adv-$1"
  echo more > "$T/adv-$1/more.txt"; git -C "$T/adv-$1" add more.txt
  git -C "$T/adv-$1" commit -q -m more && git -C "$T/adv-$1" push -q origin HEAD:main
}
cli_main() { git --git-dir="$FIX/cli.git" rev-parse main; }
cli_tag() { git --git-dir="$FIX/cli.git" rev-parse -q --verify "refs/tags/v$V^{commit}"; }

# 12. DENY-004: wait for an in-flight run, and read the newest run the way the gate does -
mk_env "$HAVE_GO"; bump_mains; echo 3 > "$ST/license_inflight"
run_pty --version "$V" --from S4 --yes
check "in-flight license-audit is waited for" test "$RC" -eq 0 -a -n "$(cli_tag)"
check "in-flight license-audit says so" out_has "still running; waiting"
rm -rf "$T"
mk_env "$HAVE_GO"; bump_mains; echo 999 > "$ST/license_inflight"
run_pty --version "$V" --from S4 --yes
check "license-audit that never ends stops S4 with no tag" test "$RC" -eq 1 -a -z "$(cli_tag)"
check "license-audit timeout is named" out_has "still running after 45 minutes"
echo 0 > "$ST/license_inflight"
run_pty --version "$V" --from S4 --yes
check "S4 resumes after the license backlog clears" test "$RC" -eq 0 -a -n "$(cli_tag)"
rm -rf "$T"
mk_env "$HAVE_GO"; bump_mains; touch "$ST/license_pr_fail"
run_pty --version "$V" --from S4 --yes
check "a failed PR-branch license-audit run stops S4 before tagging" test "$RC" -eq 1 -a -z "$(cli_tag)"
check "the gate's any-branch query is named" out_has "ANY branch"
rm -rf "$T"
mk_env "$HAVE_GO"; bump_mains; touch "$ST/license_none"
run_pty --version "$V" --from S4 --yes
check "no license-audit run on main stops S4" test "$RC" -eq 1 -a -z "$(cli_tag)"
check "no-run message gives the dispatch command" out_has "no license-audit.yml run on main"
rm -rf "$T"

# 13. main moved after the S3 merge: tag the reviewed commit, not the new head ------------
mk_env "$HAVE_GO"; bump_mains
rev=$(cli_main); fake_merged_pr cli "release/v$V" "$rev"; advance_main cli
run_pty --version "$V" --from S4 --yes
check "moved main: exits 0" test "$RC" -eq 0
check "moved main: the merge commit is tagged" test "$(cli_tag)" = "$rev"
check "moved main: the tag is not on the new head" test "$(cli_tag)" != "$(cli_main)"
check "moved main: says so" out_has "moved on; tagging the reviewed release commit"
rm -rf "$T"
mk_env "$HAVE_GO"; bump_mains
git clone -q "$FIX/cli.git" "$T/side"; git -C "$T/side" checkout -q -b side
echo s > "$T/side/s.txt"; git -C "$T/side" add s.txt; git -C "$T/side" commit -q -m side
git -C "$T/side" push -q origin side
fake_merged_pr cli "release/v$V" "$(git -C "$T/side" rev-parse HEAD)"; advance_main cli
run_pty --version "$V" --from S4 --yes
check "a merge commit outside main's history is refused" test "$RC" -eq 1 -a -z "$(cli_tag)"
check "outside-history message" out_has "is not in origin/main's history"
rm -rf "$T"

# 14. hash integrity: stale run, formula hashes, published checksums ------------------------
mk_env "$HAVE_GO"
git --git-dir="$FIX/cli.git" tag "v$V" main; bump_mains admin
mkdir -p "$TMPDIR/nself-release-$V/checksums"; echo "poison  nself-$V-darwin-arm64.tar.gz" > "$TMPDIR/nself-release-$V/checksums/checksums.txt"
printf 'run=9999\n' > "$TMPDIR/nself-release-$V.state"
run_pty --version "$V" --from S6 --yes
check "stale cached run is looked up again" out_has "is not the build of"
check "stale cache: run exits 0" test "$RC" -eq 0
check "stale cache: formula has the fixture arm hash" test "$(formula_hash arm)" = "$ARM"
rm -rf "$T"
mk_env "$HAVE_GO"
git --git-dir="$FIX/cli.git" tag "v$V" main; bump_mains admin; bump_brew
run_pty --version "$V" --from S6 --yes
check "formula at the version but with old hashes is not done" test "$RC" -eq 0 -a "$(formula_hash arm)" = "$ARM" -a "$(formula_hash intel)" = "$INTEL"
check "stale-hash rewrite says so" out_has "hashes differ"
rm -rf "$T"
mk_env "$HAVE_GO"
git --git-dir="$FIX/cli.git" tag "v$V" main; bump_mains admin; touch "$ST/published_differs"
run_pty --version "$V" --from S6 --yes
check "S7 refuses a formula that differs from the published checksums" test "$RC" -eq 1
check "S7 mismatch names the file and the recovery" out_has "does not match the published checksums.txt"
check "S7 mismatch recovery is --from S6" out_has "--from S6"
run_pty --version "$V" --from S6 --yes
ALT=639ecc2daad4e41e102743725f3369010658cbdd72a1343a675af421ee99b664
check "--from S6 rewrites the formula from the published checksums" test "$RC" -eq 0 -a "$(formula_hash arm)" = "$ALT"
rm -rf "$T"

# 15. expired artifact, release gate, bounded checks -------------------------------------------
mk_env "$HAVE_GO"; git --git-dir="$FIX/cli.git" tag "v$V" main; touch "$ST/artifact_expired"
run_pty --version "$V" --from S5 --yes
check "expired artifact exits 1" test "$RC" -eq 1
check "expired artifact gives the real recovery" out_has "gh run rerun 5551 -R nself-org/cli"
rm -rf "$T"
mk_env "$HAVE_GO"; git --git-dir="$FIX/cli.git" tag "v$V" main; bump_mains admin; touch "$ST/gate_fail"
run_pty --version "$V" --from S6 --yes
check "a failed gate job stops S6 before any merge" test "$RC" -eq 1 -a ! -s "$ADMIN_LOG"
check "the failed job is named" out_has "'Pre-Release Gate' failed"
rm -rf "$T"
mk_env "$HAVE_GO"; echo 3 > "$ST/pending_left"
run_pty --version "$V" --from S2 --yes
check "pending checks are waited for" test "$RC" -eq 0 -a "$(nlines "$ADMIN_LOG")" -eq 3
rm -rf "$T"
mk_env "$HAVE_GO"; touch "$ST/pending_forever"
run_pty --version "$V" --from S2 --yes
check "checks pending past the budget stop the run" test "$RC" -eq 1 -a ! -s "$ADMIN_LOG"
check "pending budget message names the way back" out_has "pending after 45 minutes"
rm -rf "$T"

# 16. S10 and the admin tag ------------------------------------------------------------------------
mk_env "$HAVE_GO"; git --git-dir="$FIX/cli.git" tag "v$V" main; touch "$ST/published" "$ST/workflows_fail"
run_pty --version "$V" --from S10 --yes
check "an API error in S10 is not 'nothing to dispatch'" test "$RC" -eq 1
check "S10 API error is reported" out_has "cannot read the cli workflow list"
rm -rf "$T"
mk_env "$HAVE_GO"; bump_mains admin; git --git-dir="$FIX/cli.git" tag "v$V" main; touch "$ST/published"
git --git-dir="$FIX/admin.git" tag "v$V" main~1
run_pty --version "$V" --from S9 --yes
check "an admin tag on another commit is refused" test "$RC" -eq 1
check "the admin tag was not moved" test "$(git --git-dir="$FIX/admin.git" rev-parse "refs/tags/v$V")" = "$(git --git-dir="$FIX/admin.git" rev-parse main~1)"
rm -rf "$T"
mk_env "$HAVE_GO"; bump_mains admin; git --git-dir="$FIX/cli.git" tag "v$V" main; touch "$ST/published" "$ST/admin_tagrun_fail"
run_pty --version "$V" --from S9 --yes
check "a failed tag-push publish falls back to one dispatch" test "$RC" -eq 0 -a -n "$(grep 'workflow run docker-publish.yml -R nself-org/admin' "$GH_LOG")"
rm -rf "$T"

# 17. a merged PR whose bump is not on main ----------------------------------------------------------
mk_env "$HAVE_GO"
fake_merged_pr admin "release/v$V-lockstep" "$(git --git-dir="$FIX/admin.git" rev-parse main)"
run_pty --version "$V" --from S2 --yes
check "a reverted bump gets its own message" test "$RC" -eq 1
check "reverted bump message" out_has "main does not carry v$V"
rm -rf "$T"

# 18. skipped required check, duplicate milestone, admin bump commit ----------------------------------
mk_env "$HAVE_GO"
mkdir -p "$ST/required"; touch "$ST/required/admin" "$ST/skip_ctx"
run_pty --version "$V" --from S2 --yes
check "a skipped required check counts as satisfied" test "$RC" -eq 0 -a "$(nlines "$ADMIN_LOG")" -eq 3
rm -rf "$T"
mk_env "$HAVE_GO"; bump_mains; touch "$ST/dup_open_milestone"
run_pty --version "$V" --from S4 --yes
check "an open duplicate milestone stops S4" test "$RC" -eq 1 -a -z "$(cli_tag)"
check "the duplicate milestone is named" out_has "is not closed (an open duplicate?)"
rm -rf "$T"
mk_env "$HAVE_GO"; bump_mains admin; advance_main admin
git --git-dir="$FIX/cli.git" tag "v$V" main; touch "$ST/published"
bump=$(git --git-dir="$FIX/admin.git" log --format=%H -1 -- package.json)
run_pty --version "$V" --from S9 --yes
check "without a PR record the admin tag goes on the bump commit" test "$RC" -eq 0 -a "$(git --git-dir="$FIX/admin.git" rev-parse "refs/tags/v$V^{commit}")" = "$bump"
check "the admin tag is not on the tip of main" test "$bump" != "$(git --git-dir="$FIX/admin.git" rev-parse main)"
rm -rf "$T"
