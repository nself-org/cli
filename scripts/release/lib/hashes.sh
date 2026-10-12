# shellcheck shell=bash
# Purpose: the release run, the checksums it produced, and the Homebrew formula
#          hashes, with checks that they belong together (S5, S6, S7).
# Inputs:  helpers from common.sh; globals VERSION TAG PLAN WORK CUR_S CUR_ID
# Outputs: find_run (RUN_ID), ensure_checksums (CHECKSUMS_FILE), load_formula
#          (F_VER F_ARM F_INTEL), sums_hash, release_gate, verify_published.
# Constraints: GET-only. Build tarballs embed a build date, so a rebuilt release has
#          new hashes: nothing here trusts a cached run or checksums file blindly.

# shellcheck disable=SC2034
RUN_ID=""; CHECKSUMS_SRC=""; CHECKSUMS_FILE=""; F_VER=""; F_ARM=""; F_INTEL=""; GATE=""

# find_run: RUN_ID = the release.yml run built from the tag's commit ("" if none yet).
# A run id kept in the state file is used only if its headSha is the tag's commit.
find_run() {
  local id
  RUN_ID=""
  if ! repo_tag_sha cli "$TAG"; then return 0; fi
  id=$(state_get run)
  if [ -n "$id" ]; then
    if gh_try run view "$id" -R nself-org/cli --json headSha --jq .headSha && [ "$GH_OUT" = "$TAG_SHA" ]; then :; else
      say "  run $id from an earlier attempt is not the build of $TAG; looking it up again"
      id=""
    fi
  fi
  if [ -z "$id" ]; then
    gh_need "the release.yml runs" run list -R nself-org/cli --workflow release.yml --branch "$TAG" --limit 10 \
      --json databaseId,headSha --jq ".[] | select(.headSha == \"$TAG_SHA\") | .databaseId"
    id=$(printf '%s\n' "$GH_OUT" | head -1)
  fi
  if [ -n "$id" ]; then state_set run "$id"; fi
  RUN_ID=$id
}
run_present() { find_run; [ -n "$RUN_ID" ]; }

# ensure_checksums: CHECKSUMS_FILE = checksums.txt, fetched fresh every time. The
# published release asset wins when the release exists; before publish it is the
# build run's `release-checksums` artifact (retention: 1 day). Returns 1 if neither.
ensure_checksums() {
  local d="$WORK/checksums"
  CHECKSUMS_FILE=""
  find_run
  rm -rf "$d"; mkdir -p "$d"
  if gh release download "$TAG" -R nself-org/cli -p checksums.txt -D "$d" --clobber >/dev/null 2>&1 && [ -s "$d/checksums.txt" ]; then
    CHECKSUMS_SRC="the published release"
  elif [ -n "$RUN_ID" ] && gh run download "$RUN_ID" -R nself-org/cli -n release-checksums -D "$d" >/dev/null 2>&1 && [ -s "$d/checksums.txt" ]; then
    CHECKSUMS_SRC="the build artifact of run $RUN_ID"
  else
    rm -rf "$d"; return 1
  fi
  CHECKSUMS_FILE=$d/checksums.txt
}

# sums_hash <file> <arm64|amd64>: the darwin tarball's sha256 from a checksums.txt.
sums_hash() { awk -v a="nself-${VERSION}-darwin-$2.tar.gz" '$2 == a { print $1; exit }' "$1"; }

formula_block_sha() {
  awk -v want="$1" '
    /on_arm do/ { b = "arm" } /on_intel do/ { b = "intel" } /^[[:space:]]*end[[:space:]]*$/ { b = "" }
    b == want && /sha256/ { sub(/^[^"]*"/, ""); sub(/".*$/, ""); print; exit }'
}
# load_formula: F_VER F_ARM F_INTEL of Formula/nself.rb on homebrew-nself main.
load_formula() {
  gh_need "Formula/nself.rb on homebrew-nself" api -H 'Accept: application/vnd.github.raw' \
    repos/nself-org/homebrew-nself/contents/Formula/nself.rb
  F_VER=$(printf '%s\n' "$GH_OUT" | sed -n 's/^[[:space:]]*version "\([^"]*\)".*/\1/p' | head -1)
  F_ARM=$(printf '%s\n' "$GH_OUT" | formula_block_sha arm)
  F_INTEL=$(printf '%s\n' "$GH_OUT" | formula_block_sha intel)
}

# verify_published (S7): the formula on main must carry the hashes of the checksums.txt
# that was actually published. A rebuilt release changes every hash.
verify_published() {
  local d="$WORK/published" arm intel
  rm -rf "$d"; mkdir -p "$d"
  gh_need "the published checksums.txt" release download "$TAG" -R nself-org/cli -p checksums.txt -D "$d" --clobber
  arm=$(sums_hash "$d/checksums.txt" arm64); intel=$(sums_hash "$d/checksums.txt" amd64)
  [ -n "$arm" ] && [ -n "$intel" ] || die_step S7 release-wait "the published checksums.txt has no darwin arm64/amd64 lines"
  load_formula
  if [ "$F_VER" != "$VERSION" ] || [ "$F_ARM" != "$arm" ] || [ "$F_INTEL" != "$intel" ]; then
    die_step S7 release-wait "Formula/nself.rb on homebrew-nself main does not match the published checksums.txt (arm formula ${F_ARM:0:12} vs published ${arm:0:12}; intel formula ${F_INTEL:0:12} vs published ${intel:0:12}). brew install would fail its checksum. Re-write the formula from the published file" S6
  fi
}

# release_gate (S6): every release.yml job except the Homebrew lockstep job and the
# publish job must have succeeded before the formula is merged, so main never names
# a version that cannot publish.
# shellcheck disable=SC2016
gate_state() {
  gh_need "the jobs of run $RUN_ID" run view "$RUN_ID" -R nself-org/cli --json jobs --jq '
    [.jobs[] | select((.name | test("Homebrew|Sign and publish"; "i")) | not)] as $j
    | ($j | map(select(.status == "completed" and (.conclusion == "failure" or .conclusion == "cancelled" or .conclusion == "timed_out")))) as $f
    | ($j | map(select(.status != "completed"))) as $p
    | if ($f | length) > 0 then "failed:" + $f[0].name elif ($p | length) > 0 then "pending" else "ok" end'
  GATE=$GH_OUT
}
gate_settled() { gate_state; [ "$GATE" != pending ]; }
release_gate() {
  find_run
  [ -n "$RUN_ID" ] || die_step S6 homebrew-formula "no release.yml run found for $TAG (run S5 first)" S5
  poll "release.yml jobs (build, sbom, provenance, Pre-Release Gate)" 120 gate_settled || true
  gate_state
  case "$GATE" in
    ok) ;;
    failed:*) die_step S6 homebrew-formula "release.yml job '${GATE#failed:}' failed in run $RUN_ID; the formula is not merged for a release that cannot publish. Fix the cause and re-run the failed jobs: gh run rerun $RUN_ID -R nself-org/cli --failed" ;;
    *) die_step S6 homebrew-formula "release.yml jobs are still running after the wait budget (run $RUN_ID)" ;;
  esac
}
