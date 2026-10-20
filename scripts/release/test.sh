#!/usr/bin/env bash
# Purpose: fixture tests for scripts/release/release.sh. Stub gh/curl/sleep/node on PATH
#          log every call and answer from fixtures/; bare repos stand in for GitHub.
# Usage:   bash scripts/release/test.sh        (exit 0 only when every case passes)
# Inputs:  none. Needs git, jq, awk; go for the facts case; `script` for the pty cases.
# Outputs: PASS / FAIL / SKIP lines. Scratch lives under $TMPDIR (default /tmp) and is removed.
# Constraints: no network, no token, never touches real GitHub or any real repo.
# ROOT, ORIG_PATH, RC and friends are consumed by the sourced fixtures/setup.sh.
# shellcheck disable=SC2034
set -u
RELDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$RELDIR/../.." && pwd)"
FIXTURES="$RELDIR/fixtures"
ORIG_PATH="$PATH"
BASE=$(mktemp -d "${TMPDIR:-/tmp}/release-test.XXXXXX")
trap 'rm -rf "$BASE"' EXIT
# shellcheck disable=SC1091
. "$FIXTURES/setup.sh"

PASS=0; FAILN=0
ok() { echo "PASS $1"; PASS=$((PASS + 1)); }
bad() { echo "FAIL $1${2:+ ($2)}"; FAILN=$((FAILN + 1)); }
skip() { echo "SKIP $1 ($2)"; }
check() { local n=$1; shift; if "$@" >/dev/null; then ok "$n"; else bad "$n"; fi; }
# checkc <name> <shell snippet>: for checks that need a pipe.
checkc() { if eval "$2" >/dev/null; then ok "$1"; else bad "$1"; fi; }
nlines() { wc -l < "$1" | tr -d ' '; }
out_has() { grep -q -- "$1" "$T/out"; }
HAVE_PTY=0; command -v script >/dev/null 2>&1 && HAVE_PTY=1
HAVE_GO=0; command -v go >/dev/null 2>&1 && HAVE_GO=1
V=1.5.0
ARM=554a93c77f0a4d50fabbdc3ed7bd01638f092ce082f2d54d866d659d662bfd77
INTEL=6f823864cb70bb06e0d87f751db0fafcf96375da658da2b281b0fb1d9a4deeba

# 1. plan: S1..S10 in order, GET-only, writes nothing -----------------------------
mk_env "$HAVE_GO"
before=$(snap)
run_plain --plan --version 9.9.9
check "plan exits 0" test "$RC" -eq 0
seq=$(awk '/^S[0-9]+ /{sub(/^S/, "", $1); print $1}' "$T/out" | uniq | tr '\n' ' ')
check "plan lists S1..S10 in order" test "$seq" = "1 2 3 4 5 6 7 8 9 10 "
ids=$(awk '/^S[0-9]+ /{print $2}' "$T/out" | uniq | tr '\n' ' ')
check "plan step ids" test "$ids" = "preflight admin-lockstep cli-version tag build-wait homebrew-formula release-wait docker-verify admin-image sdk-publish "
for r in admin cli homebrew-nself; do
  check "plan shows admin-merge for $r" grep -q "scripts/admin-merge.sh --repo nself-org/$r --pr" "$T/out"
done
check "plan shows facts regeneration" out_has 'tools/facts -readme README.md'
check "plan shows the web update-lock PR" grep -q '^S7 release-wait web .*update-lock.mjs cli v9.9.9' "$T/out"
check "plan shows the tag-time denylist checks" test "$(grep -c -E '^S4 tag cli .*DENY-00[345]' "$T/out")" -eq 4
check "plan shows the admin tag" grep -q '^S9 admin-image admin .*tag -a v9.9.9 .*push origin refs/tags/v9.9.9' "$T/out"
check "plan shows the release gate before the formula merge" grep -q '^S6 homebrew-formula cli .*Sign and publish' "$T/out"
check "plan shows the annotated tag push" grep -q "^S4 tag cli .*tag -a v9.9.9 .*push origin refs/tags/v9.9.9" "$T/out"
check "plan never mentions release-create" test -z "$(grep 'release-create' "$T/out")"
check "plan shows the checksums-file formula command" out_has 'update-formula.sh --version v9.9.9 --checksums-file'
check "plan gh calls are all api GETs" test -z "$(grep -v '^api ' "$GH_LOG")"
check "plan gh calls use no method flag" test -z "$(grep -E -e ' -X | --method | -f | -F ' "$GH_LOG")"
check "plan changed no fixture repo ref" test "$(snap)" = "$before"
check "plan left the checkout clean" test -z "$(git -C "$T/cwd" status --porcelain)"
check "plan wrote no work dir or state" test ! -e "$TMPDIR/nself-release-9.9.9" -a ! -e "$TMPDIR/nself-release-9.9.9.state"
check "plan made no clone" test -z "$(grep 'repo clone' "$GH_LOG")"
rm -rf "$T"

# 2. refused input -----------------------------------------------------------------
mk_env "$HAVE_GO"
run_plain --plan --version 1.4.13;      check "version == latest exits 2" test "$RC" -eq 2
run_plain --plan --version 1.4.12;      check "version < latest exits 2" test "$RC" -eq 2
git --git-dir="$FIX/cli.git" tag v1.5.0 main
run_plain --plan --version 1.5.0;       check "existing tag exits 2" test "$RC" -eq 2
check "existing tag message" grep -q 'already exists' "$T/err"
run_plain --plan --version 1.5.0 --from S3; check "tag exists with --from S3 exits 2" test "$RC" -eq 2
run_plain --plan --version 1.5.0 --from S4; check "tag exists with --from S4 is allowed" test "$RC" -eq 0
run_plain --plan --version 1.4.13 --from S4; check "version == latest with --from S4 exits 2" test "$RC" -eq 2
run_plain --plan --version 1.4.13 --from S5; check "version == latest with --from S5 is allowed" test "$RC" -eq 0
run_plain --plan --version 1.4.12 --from S5; check "version < latest with --from S5 exits 2" test "$RC" -eq 2
run_plain --plan;                       check "missing --version exits 2" test "$RC" -eq 2
run_plain --plan --version abc;         check "bad --version exits 2" test "$RC" -eq 2
run_plain --plan --version 2.0.0 --from S11; check "bad --from exits 2" test "$RC" -eq 2
: > "$GH_LOG"
run_plain --version 2.0.0;              check "non-plan without a TTY exits 2" test "$RC" -eq 2
check "non-plan without a TTY made no gh call" test ! -s "$GH_LOG"
rm -rf "$T"

# shellcheck disable=SC1091
. "$FIXTURES/cases4.sh"

if [ "$HAVE_PTY" = 0 ]; then
  skip "pty cases (full run, rerun, --from S6, facts absent, red checks)" "script(1) not found"
else
# 3. full run under a pty: S1..S10, --yes, owner-answered merges --------------------
mk_env "$HAVE_GO"
run_pty --version "$V" --yes
check "full run exits 0" test "$RC" -eq 0
check "full run completes" out_has "complete"
check "admin-merge ran three times" test "$(nlines "$ADMIN_LOG")" -eq 3
check "admin-merge got a TTY every time" test "$(grep -c 'tty=yes' "$ADMIN_LOG")" -eq 3
check "admin-merge read the owner's typed answer" test "$(grep -c 'ans=y$' "$ADMIN_LOG")" -eq 3
check "merge order admin, cli, homebrew" test "$(sed 's/ pr=.*//' "$ADMIN_LOG" | tr '\n' ' ')" = "repo=admin repo=cli repo=homebrew-nself "
check "admin package.json bumped" test "$(show admin package.json | jq -r .version)" = "$V"
checkc "admin Dockerfile bumped" "show admin Dockerfile | grep -q 'ARG NSELF_VERSION=$V'"
for pkg in ts ts-sdk ts-cli-api; do
  check "cli SDK $pkg bumped" test "$(show cli sdk/$pkg/package.json | jq -r .version)" = "$V"
done
check "cli VERSION bumped" test "$(show cli .github/VERSION)" = "$V"
sha=$(git --git-dir="$FIX/cli.git" rev-parse main)
check "tag v$V points at cli main" test "$(git --git-dir="$FIX/cli.git" rev-parse "refs/tags/v$V^{commit}")" = "$sha"
check "tag is annotated" test "$(git --git-dir="$FIX/cli.git" cat-file -t "refs/tags/v$V")" = tag
check "no placeholder release was made" test -z "$(grep -E '^release create' "$GH_LOG")"
checkc "formula version" "show homebrew-nself Formula/nself.rb | grep -q 'version \"$V\"'"
check "formula on_arm hash" test "$(formula_hash arm)" = "$ARM"
check "formula on_intel hash" test "$(formula_hash intel)" = "$INTEL"
check "formula hashes differ" test "$(formula_hash arm)" != "$(formula_hash intel)"
check "formula step needed no release download" test -z "$(grep 'releases/download' "$CURL_LOG")"
check "lockstep job re-run" grep -q 'run rerun 5551 -R nself-org/cli --failed' "$GH_LOG"
check "cli image dispatched" grep -q "workflow run docker-publish.yml -R nself-org/cli -f version=$V" "$GH_LOG"
check "admin tagged on its lockstep merge commit" test "$(git --git-dir="$FIX/admin.git" rev-parse "refs/tags/v$V^{commit}")" = "$(git --git-dir="$FIX/admin.git" rev-parse main)"
check "admin tag is annotated" test "$(git --git-dir="$FIX/admin.git" cat-file -t "refs/tags/v$V")" = tag
check "admin image came from the tag push, not a dispatch" test -z "$(grep 'workflow run docker-publish.yml -R nself-org/admin' "$GH_LOG")"
check "sdk ts dispatched on the tag with no version input" grep -q "workflow run sdk-ts-publish.yml -R nself-org/cli --ref v$V\$" "$GH_LOG"
check "sdk py dispatched on the tag with no version input" grep -q "workflow run sdk-py-publish.yml -R nself-org/cli --ref v$V\$" "$GH_LOG"
check "sdk go dispatched with the cli version" grep -q "workflow run sdk-publish-go.yml -R nself-org/cli --ref v$V -f version=$V" "$GH_LOG"
check "S8 waited for the Release-started publish run" out_has "a docker-publish.yml run on cli (9/9)"
check "sdk without workflow_dispatch is manual" out_has "sdk-manual-publish.yml has no workflow_dispatch"
check "web lock PR branch pushed" git --git-dir="$FIX/web.git" rev-parse -q --verify "refs/heads/release/cli-v$V"
check "web lock pins the tag" test "$(git --git-dir="$FIX/web.git" show "release/cli-v$V:org/sources.lock.json" | jq -r .cli)" = "v$V"
check "work dir and state cleaned up" test ! -e "$TMPDIR/nself-release-$V" -a ! -e "$TMPDIR/nself-release-$V.state"
if [ "$HAVE_GO" = 1 ]; then
  git clone -q "$FIX/cli.git" "$T/chk"
  check "bump commit passes tools/facts -check" bash -c "cd '$T/chk' && go run -mod=vendor ./tools/facts -check -readme README.md"
  check "facts.json regenerated in the bump commit" test "$(git --git-dir="$FIX/cli.git" show main:.github/facts.json | jq -r .version)" = "$V"
  checkc "README facts regenerated" "show cli README.md | grep -q 'cli version: $V'"
else skip "facts present case" "go not installed"; fi

# 4. rerun of finished steps: everything says done, nothing changes ------------------
before=$(snap); mut_before=$(grep -c -E '^(pr create|workflow run|run rerun)' "$GH_LOG")
run_pty --version "$V" --from S4 --yes
check "rerun exits 0" test "$RC" -eq 0
for s in "S4 tag" "S5 build-wait" "S6 homebrew-formula" "S7 release-wait" "S8 docker-verify" "S9 admin-image" "S10 sdk-publish"; do
  check "rerun: $s done" grep -q "$s.* done" "$T/out"
done
check "rerun changed no ref" test "$(snap)" = "$before"
check "rerun made no PR, dispatch or re-run" test "$(grep -c -E '^(pr create|workflow run|run rerun)' "$GH_LOG")" -eq "$mut_before"
check "rerun asked admin-merge nothing" test "$(nlines "$ADMIN_LOG")" -eq 3
rm -rf "$T"

# 5. --from S6 skips S1-S5 -------------------------------------------------------------
mk_env "$HAVE_GO"
git --git-dir="$FIX/cli.git" tag "v$V" main; bump_mains admin
run_pty --version "$V" --from S6 --yes
check "--from S6 exits 0" test "$RC" -eq 0
check "--from S6 prints no S1-S5 step" test -z "$(grep -E '^S[1-5] ' "$T/out")"
check "--from S6 merged only homebrew" test "$(sed 's/ pr=.*//' "$ADMIN_LOG" | tr '\n' ' ')" = "repo=homebrew-nself "
check "--from S6 did not clone cli" test -z "$(grep -E 'repo clone nself-org/cli ' "$GH_LOG")"
check "--from S6 formula on_arm" test "$(formula_hash arm)" = "$ARM"
check "--from S6 formula on_intel" test "$(formula_hash intel)" = "$INTEL"
rm -rf "$T"

# 6. facts absent: S3 skips regeneration -----------------------------------------------
mk_env 0; bump_mains admin
run_pty --version "$V" --from S3 --yes
check "no-facts run exits 0" test "$RC" -eq 0
check "no-facts run says so" out_has "tools/facts absent"
check "no-facts commit has no facts.json" test -z "$(git --git-dir="$FIX/cli.git" ls-tree --name-only main .github/ | grep facts.json)"
check "no-facts cli VERSION bumped" test "$(show cli .github/VERSION)" = "$V"
rm -rf "$T"

# 7. red checks never merge --------------------------------------------------------------
mk_env "$HAVE_GO"
export STUB_CHECKS_RED=1
run_pty --version "$V" --from S2 --yes
unset STUB_CHECKS_RED
check "red checks exit 1" test "$RC" -eq 1
check "red checks name the step" out_has "S2 admin-lockstep failed"
check "red checks never called admin-merge" test ! -s "$ADMIN_LOG"
check "red checks left admin main alone" test "$(show admin package.json | jq -r .version)" = 1.4.13
rm -rf "$T"

# shellcheck disable=SC1091
. "$FIXTURES/cases2.sh"
# shellcheck disable=SC1091
. "$FIXTURES/cases3.sh"
fi

echo "---- $PASS passed, $FAILN failed"
[ "$FAILN" -eq 0 ]
