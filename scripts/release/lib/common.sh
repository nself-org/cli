# shellcheck shell=bash
# Purpose: shared helpers for scripts/release/release.sh (state, plan lines, prompts,
#          polling, clones, PR open + owner-answered merge).
# Inputs:  globals set by release.sh: VERSION TAG PLAN YES WORK STATE ADMIN_MERGE
# Outputs: functions only; nothing runs on source.
# Constraints: bash 3.2 (no assoc arrays, no mapfile, no ${x,,}); no token is read,
#          printed or stored: only the ambient `gh` session is used.

POLL_SECONDS=20

say() { printf '%s\n' "$*"; }
err() { printf 'release: %s\n' "$*" >&2; }

# die_step <S#> <id> <what failed>: exit 1 naming the step and how to resume.
die_step() {
  err "$1 $2 failed: $3"
  err "recovery: fix the cause, then run: scripts/release/release.sh --version ${VERSION} --from $1"
  exit 1
}

# plan_line <S#> <id> <repo> <command...>: one plan row (plan mode only).
plan_line() {
  local s=$1 id=$2 repo=$3
  shift 3
  printf '%s %s %s %s\n' "$s" "$id" "$repo" "$*"
}

step_done() { say "$1 $2 done"; }

# step_confirm <S#> <id>: release.sh's own prompt. --yes skips only this prompt.
# It never answers admin-merge.sh's prompt (that one reads the terminal itself).
step_confirm() {
  local ans=""
  if [ "$YES" = 1 ]; then return 0; fi
  printf 'Run %s %s? [y/N] ' "$1" "$2"
  read -r ans || ans=""
  case "$ans" in
    y|Y) return 0 ;;
    *) err "stopped at $1 by the operator; resume with --from $1"; exit 1 ;;
  esac
}

# --- state file: key=value lines under $TMPDIR; plan mode never writes ---
state_set() {
  if [ "$PLAN" = 1 ]; then return 0; fi
  mkdir -p "$(dirname "$STATE")"
  touch "$STATE"
  { grep -v "^$1=" "$STATE" || true; printf '%s=%s\n' "$1" "$2"; } > "$STATE.tmp"
  mv "$STATE.tmp" "$STATE"
}
state_get() {
  if [ -f "$STATE" ]; then sed -n "s/^$1=//p" "$STATE" | tail -1; fi
}

# ver_gt A B: success when semver A > B (X.Y.Z only).
ver_gt() {
  awk -v a="$1" -v b="$2" 'BEGIN {
    split(a, x, "."); split(b, y, ".")
    for (i = 1; i <= 3; i++) { if (x[i] + 0 > y[i] + 0) exit 0; if (x[i] + 0 < y[i] + 0) exit 1 }
    exit 1 }'
}

# poll <what> <tries> <cmd...>: run cmd until it succeeds, sleeping between tries.
poll() {
  local what=$1 tries=$2 i=0
  shift 2
  while [ "$i" -lt "$tries" ]; do
    if "$@"; then return 0; fi
    i=$((i + 1))
    say "  waiting for ${what} (${i}/${tries})"
    sleep "$POLL_SECONDS"
  done
  return 1
}

# gh_raw <repo> <path>: file content of main via the contents API (GET).
gh_raw() { gh api -H 'Accept: application/vnd.github.raw' "repos/nself-org/$1/contents/$2"; }

# tag_exists: 0 exists, 1 absent (404); any other failure stops the run, because
# an unreadable answer must never read as "absent".
tag_exists() {
  local out
  if out=$(gh api "repos/nself-org/cli/git/ref/tags/$TAG" 2>&1); then return 0; fi
  if printf '%s' "$out" | grep -q -E '404|Not Found'; then return 1; fi
  err "cannot check tag $TAG: $out"
  exit 1
}

# --- clones: sibling layout under $WORK so cli/scripts/bump-version.sh finds ../admin ---
ensure_clone() {
  if [ -d "$WORK/$1/.git" ]; then return 0; fi
  mkdir -p "$WORK"
  gh repo clone "nself-org/$1" "$WORK/$1" -- --quiet
}

# bump_all: cli's own bump-version.sh edits cli and ../admin (idempotent).
bump_all() { (cd "$WORK/cli" && bash scripts/bump-version.sh "$VERSION") >/dev/null; }

# --- pull requests ---
pr_info() { # <repo> <branch> -> "<n> <STATE>" or nothing
  gh pr list -R "nself-org/$1" --head "$2" --state all --json number,state \
    --jq '.[0] | select(.) | "\(.number) \(.state)"'
}
pr_state() { gh pr view "$2" -R "nself-org/$1" --json state --jq .state; }

# open_pr <S#> <id> <repo> <branch> <title> <body>: commit is already made in
# $WORK/<repo>; push the branch and open the PR; prints the PR number.
open_pr() {
  local s=$1 id=$2 repo=$3 br=$4 url
  git -C "$WORK/$repo" push -q origin "$br" \
    || die_step "$s" "$id" "push of $br to $repo was refused (does the branch already exist? delete it, then retry)"
  url=$(gh pr create -R "nself-org/$repo" --base main --head "$br" --title "$5" --body "$6") \
    || die_step "$s" "$id" "gh pr create failed on $repo"
  printf '%s\n' "${url##*/}"
}

# wait_checks <S#> <id> <repo> <n>: block until required checks are green.
# Red -> exit 1. "No checks yet" is retried: a PR seconds old reports none.
wait_checks() {
  local s=$1 id=$2 repo=$3 n=$4 i=0 out rc
  out=$(mktemp "${TMPDIR:-/tmp}/nself-release-checks.XXXXXX")
  while [ "$i" -lt 6 ]; do
    set +e
    gh pr checks "$n" -R "nself-org/$repo" --required --watch --interval 30 2>&1 | tee "$out"
    rc=${PIPESTATUS[0]}
    set -e
    if [ "$rc" -eq 0 ]; then rm -f "$out"; return 0; fi
    if grep -q -i -E 'no (required )?checks' "$out"; then
      i=$((i + 1)); sleep "$POLL_SECONDS"; continue
    fi
    rm -f "$out"
    die_step "$s" "$id" "required checks on $repo#$n are not green; nothing was merged"
  done
  rm -f "$out"
  err "$s $id: $repo#$n reports no required checks after 6 tries; continuing to the owner prompt"
}

# merge_pr <S#> <id> <repo> <n>: green checks, then admin-merge.sh with the
# owner's terminal. No stdin redirect, no pre-answer, and --yes never applies here.
merge_pr() {
  local s=$1 id=$2 repo=$3 n=$4
  if [ "$(pr_state "$repo" "$n")" = MERGED ]; then return 0; fi
  wait_checks "$s" "$id" "$repo" "$n"
  say "$s $id: handing the terminal to admin-merge.sh for $repo#$n (answer its prompt)"
  (cd "$WORK/$repo" && bash "$ADMIN_MERGE" --repo "nself-org/$repo" --pr "$n") \
    || die_step "$s" "$id" "admin-merge.sh failed on $repo#$n (check branch protection: it restores itself; see its audit log)"
  if [ "$(pr_state "$repo" "$n")" != MERGED ]; then
    die_step "$s" "$id" "$repo#$n is not merged after admin-merge.sh (declined at its prompt?)"
  fi
}

# pr_flow <S#> <id> <repo> <branch> <state-key> <title> <body>: reuse an open PR
# or open one from the commit already made; then merge it.
pr_flow() {
  local s=$1 id=$2 repo=$3 br=$4 key=$5 info n
  info=$(pr_info "$repo" "$br")
  if [ -n "$info" ]; then
    n=${info%% *}
    say "$s $id: reusing $repo#$n (${info##* })"
  else
    n=$(open_pr "$s" "$id" "$repo" "$br" "$6" "$7")
    say "$s $id: opened $repo#$n"
  fi
  state_set "$key" "$n"
  merge_pr "$s" "$id" "$repo" "$n"
}

# release_run_id: the release.yml run for the tag (state cache, then GitHub).
release_run_id() {
  local id
  id=$(state_get run)
  if [ -z "$id" ]; then
    id=$(gh run list -R nself-org/cli --workflow release.yml --branch "$TAG" --limit 1 \
      --json databaseId --jq '.[0].databaseId // empty')
  fi
  if [ -n "$id" ]; then state_set run "$id"; fi
  printf '%s\n' "$id"
}

# ensure_checksums: checksums.txt from the build artifact; after the release is
# published (artifact retention is 1 day) fall back to the release asset.
ensure_checksums() {
  local f="$WORK/checksums/checksums.txt" id
  if [ -s "$f" ]; then printf '%s\n' "$f"; return 0; fi
  mkdir -p "$WORK/checksums"
  id=$(release_run_id)
  if [ -n "$id" ] && gh run download "$id" -R nself-org/cli -n release-checksums -D "$WORK/checksums" >/dev/null 2>&1 && [ -s "$f" ]; then
    :
  else
    gh release download "$TAG" -R nself-org/cli -p checksums.txt -D "$WORK/checksums" --clobber >/dev/null 2>&1 || true
  fi
  if [ ! -s "$f" ]; then return 1; fi
  state_set checksums "$f"
  printf '%s\n' "$f"
}
