# shellcheck shell=bash
# Purpose: second half of test.sh (sourced inside its pty block).

# 8. required checks that never report: nothing merges; none required: proceeds ------
mk_env "$HAVE_GO"
mkdir -p "$ST/required"; touch "$ST/nochecks" "$ST/required/admin"
run_pty --version "$V" --from S2 --yes
check "unreported required checks exit 1" test "$RC" -eq 1
check "unreported required checks name the step" out_has "S2 admin-lockstep failed"
check "unreported required checks never called admin-merge" test ! -s "$ADMIN_LOG"
check "unreported required checks left admin main alone" test "$(show admin package.json | jq -r .version)" = 1.4.13
rm -rf "$T"
mk_env "$HAVE_GO"
touch "$ST/nochecks"
run_pty --version "$V" --from S2 --yes
check "no required checks: run proceeds" test "$RC" -eq 0
check "no required checks: says so" out_has "requires no status checks"
check "no required checks: three merges" test "$(nlines "$ADMIN_LOG")" -eq 3
rm -rf "$T"

mk_env "$HAVE_GO"
mkdir -p "$ST/required"; touch "$ST/required/admin" "$ST/missing_ctx"
run_pty --version "$V" --from S2 --yes
check "green but a required context missing exits 1" test "$RC" -eq 1
check "missing context is named" out_has "(ci )"
check "missing context never called admin-merge" test ! -s "$ADMIN_LOG"
rm -rf "$T"
mk_env "$HAVE_GO"
mkdir -p "$ST/required"; touch "$ST/required/admin"
run_pty --version "$V" --from S2 --yes
check "green with every required context present proceeds" test "$RC" -eq 0
check "required context present: three merges" test "$(nlines "$ADMIN_LOG")" -eq 3
rm -rf "$T"

# 9. S2/S3 done-detection; tag-time denylist and the tag guard ---------------------------
mk_env "$HAVE_GO"
bump_mains
run_pty --version "$V" --from S2 --yes
check "bumped mains: run exits 0" test "$RC" -eq 0
check "bumped mains: S2 done" grep -q "S2 admin-lockstep.* done" "$T/out"
check "bumped mains: S3 done" grep -q "S3 cli-version.* done" "$T/out"
check "bumped mains: only homebrew merged" test "$(sed 's/ pr=.*//' "$ADMIN_LOG" | tr '\n' ' ')" = "repo=homebrew-nself "
check "bumped mains: tag created" git --git-dir="$FIX/cli.git" rev-parse -q --verify "refs/tags/v$V"
rm -rf "$T"
mk_env "$HAVE_GO"; bump_mains
git --git-dir="$FIX/cli.git" tag v1.4.99 main
run_pty --version "$V" --from S4 --yes
check "DENY-003 stops the tag" test "$RC" -eq 1 -a -z "$(git --git-dir="$FIX/cli.git" tag -l "v$V")"
check "DENY-003 is named" out_has "DENY-003"
rm -rf "$T"
mk_env "$HAVE_GO"; bump_mains; touch "$ST/license_fail"
run_pty --version "$V" --from S4 --yes
check "DENY-004 stops the tag" test "$RC" -eq 1 -a -z "$(git --git-dir="$FIX/cli.git" tag -l "v$V")"
check "DENY-004 is named" out_has "DENY-004"
rm -rf "$T"
mk_env "$HAVE_GO"; bump_mains; touch "$ST/no_milestone"
run_pty --version "$V" --from S4 --yes
check "DENY-005 stops the tag" test "$RC" -eq 1 -a -z "$(git --git-dir="$FIX/cli.git" tag -l "v$V")"
check "DENY-005 is named" out_has "DENY-005"
rm -rf "$T"
mk_env "$HAVE_GO"; bump_mains
old=$(git --git-dir="$FIX/cli.git" rev-parse main~1)
git --git-dir="$FIX/cli.git" tag "v$V" "$old"
run_pty --version "$V" --from S4 --yes
check "a tag on another commit is refused" test "$RC" -eq 1
check "a refused tag was not moved" test "$(git --git-dir="$FIX/cli.git" rev-parse "refs/tags/v$V")" = "$old"
rm -rf "$T"

mk_env "$HAVE_GO"; bump_mains; touch "$ST/only_bare_milestone"
run_pty --version "$V" --from S4 --yes
check "milestone without the v prefix does not pass DENY-005" test "$RC" -eq 1 -a -z "$(git --git-dir="$FIX/cli.git" tag -l "v$V")"
check "bare milestone names DENY-005" out_has "DENY-005"
rm -rf "$T"
mk_env "$HAVE_GO"; bump_mains; touch "$ST/license_fail"
run_pty --version "$V" --from S4 --yes
check "DENY-004 message gives the recovery command" out_has "gh workflow run license-audit.yml -R nself-org/cli --ref main"
rm -rf "$T"

# 10. S6 done path still re-runs a failed lockstep job; S8 waits on an in-flight publish -
mk_env "$HAVE_GO"
git --git-dir="$FIX/cli.git" tag "v$V" main; bump_brew good; bump_mains admin
run_pty --version "$V" --from S6 --yes
check "S6 done path exits 0" test "$RC" -eq 0
check "S6 done path says done" grep -q "S6 homebrew-formula.* done" "$T/out"
check "S6 done path re-ran the lockstep job" grep -q 'run rerun 5551 -R nself-org/cli --failed' "$GH_LOG"
check "S6 done path merged nothing" test ! -s "$ADMIN_LOG"
rm -rf "$T"
mk_env "$HAVE_GO"
git --git-dir="$FIX/cli.git" tag "v$V" main; bump_mains admin; touch "$ST/published" "$ST/inflight@cli"
run_pty --version "$V" --from S8 --yes
check "in-flight publish: run exits 0" test "$RC" -eq 0
check "in-flight publish: cli image not dispatched again" test -z "$(grep 'workflow run docker-publish.yml -R nself-org/cli' "$GH_LOG")"
check "in-flight publish: admin image still came from its tag push" test -n "$(git --git-dir="$FIX/admin.git" tag -l "v$V")"
rm -rf "$T"

# 11. leftovers from an earlier attempt stop with a recovery line, not a raw git error -----
mk_env "$HAVE_GO"
git clone -q "$FIX/admin.git" "$T/lo"; echo x > "$T/lo/stray"; git -C "$T/lo" add stray
git -C "$T/lo" commit -q -m stray; git -C "$T/lo" push -q origin "HEAD:refs/heads/release/v$V-lockstep"
run_pty --version "$V" --from S2 --yes
check "stale remote branch exits 1" test "$RC" -eq 1
check "stale remote branch gives the delete command" out_has "gh api -X DELETE repos/nself-org/admin/git/refs/heads/release/v$V-lockstep"
check "stale remote branch never reached admin-merge" test ! -s "$ADMIN_LOG"
rm -rf "$T"
mk_env "$HAVE_GO"
mkdir -p "$TMPDIR/nself-release-$V/cli"; touch "$TMPDIR/nself-release-$V/cli/junk"
run_pty --version "$V" --from S2 --yes
check "non-clone work dir exits 1" test "$RC" -eq 1
check "non-clone work dir says what to remove" out_has "is not a git clone"
rm -rf "$T"
