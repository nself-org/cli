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

# gh_try <gh args...>: run gh with stdout in GH_OUT and stderr in GH_ERR (never
# mixed, so a warning cannot corrupt a value); returns gh's exit status.
GH_OUT=""; GH_ERR=""
gh_try() {
  local ef rc=0
  ef=$(mktemp "${TMPDIR:-/tmp}/nself-release-err.XXXXXX")
  GH_OUT=$(gh "$@" 2>"$ef") || rc=$?
  GH_ERR=$(cat "$ef")
  rm -f "$ef"
  return "$rc"
}
is_404() { printf '%s' "$GH_ERR" | grep -q -E '404|Not Found|not protected|not enabled'; }

# tag_exists: 0 exists, 1 absent (404); any other failure stops the run, because
# an unreadable answer must never read as "absent".
tag_exists() {
  if gh_try api "repos/nself-org/cli/git/ref/tags/$TAG"; then return 0; fi
  if is_404; then return 1; fi
  err "cannot check tag $TAG: $GH_ERR"
  exit 1
}

# --- clones: sibling layout under $WORK so cli/scripts/bump-version.sh finds ../admin ---
# CUR_S / CUR_ID name the running step for error messages (set by release.sh).
CUR_S="S?"; CUR_ID="?"
ensure_clone() {
  local d="$WORK/$1"
  if [ -d "$d/.git" ]; then return 0; fi
  if [ -e "$d" ]; then
    die_step "$CUR_S" "$CUR_ID" "$d exists but is not a git clone; remove it ($d), then re-run"
  fi
  mkdir -p "$WORK"
  gh repo clone "nself-org/$1" "$d" -- --quiet || die_step "$CUR_S" "$CUR_ID" "clone of $1 failed"
}
# reset_clone <repo>: back to a clean origin/main. The clone is ours, so -f and clean are safe.
reset_clone() {
  local d="$WORK/$1"
  { git -C "$d" fetch -q --tags origin main \
    && git -C "$d" checkout -q -f --detach origin/main \
    && git -C "$d" clean -fdq; } \
    || die_step "$CUR_S" "$CUR_ID" "could not refresh the clone of $1; remove $d and re-run"
}
# checkout_branch <repo> <branch>: the clone on the remote copy of the branch (PR already open).
checkout_branch() {
  local d="$WORK/$1"
  ensure_clone "$1"
  git -C "$d" ls-remote --exit-code --heads origin "$2" >/dev/null 2>&1 \
    || die_step "$CUR_S" "$CUR_ID" "a PR exists on $1 but branch $2 is gone; close the PR and re-run"
  { git -C "$d" fetch -q origin "$2" \
    && git -C "$d" checkout -q -f -B "$2" FETCH_HEAD \
    && git -C "$d" clean -fdq; } \
    || die_step "$CUR_S" "$CUR_ID" "could not check out $2 in $d"
}
# commit_all <repo> <message>: the clone is ours, so `add -A` takes only the bump.
commit_all() {
  git -C "$WORK/$1" add -A
  if git -C "$WORK/$1" diff --cached --quiet; then return 1; fi
  git -C "$WORK/$1" commit -q -m "$2"
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
# A branch left on the remote by an earlier attempt is reused only when it holds
# exactly this commit's tree; otherwise the run stops with the delete command.
open_pr() {
  local s=$1 id=$2 repo=$3 br=$4 d="$WORK/$3" url
  if git -C "$d" ls-remote --exit-code --heads origin "$br" >/dev/null 2>&1; then
    git -C "$d" fetch -q origin "$br"
    git -C "$d" diff --quiet FETCH_HEAD HEAD 2>/dev/null \
      || die_step "$s" "$id" "branch $br already exists on $repo with different content; delete it: gh api -X DELETE repos/nself-org/$repo/git/refs/heads/$br"
  else
    git -C "$d" push -q origin "$br" || die_step "$s" "$id" "push of $br to $repo was refused"
  fi
  url=$(gh pr create -R "nself-org/$repo" --base main --head "$br" --title "$5" --body "$6") \
    || die_step "$s" "$id" "gh pr create failed on $repo"
  printf '%s\n' "${url##*/}"
}

# required_contexts <repo>: names of the status checks main's protection requires
# (GET), one per line. Nothing for a 404 (no protection, or none required); "?" when
# the answer cannot be read.
required_contexts() {
  if gh_try api "repos/nself-org/$1/branches/main/protection/required_status_checks" \
    --jq '((.contexts // []) + [(.checks // [])[].context]) | unique | .[]'; then
    { printf '%s\n' "$GH_OUT" | grep . || true; }
  elif ! is_404; then printf '?\n'; fi
}

# missing_required <repo> <n>: required contexts not reported as passing on the PR
# (by name), space separated; "?" if unreadable; nothing when all are present.
missing_required() {
  local ctx got
  ctx=$(required_contexts "$1")
  if [ -z "$ctx" ]; then return 0; fi
  if [ "$ctx" = "?" ]; then printf '?\n'; return 0; fi
  got=$(gh pr checks "$2" -R "nself-org/$1" --required --json name,bucket \
    --jq '.[] | select(.bucket == "pass") | .name') || { printf '?\n'; return 0; }
  printf '%s\n' "$ctx" | while IFS= read -r c; do
    printf '%s\n' "$got" | grep -qxF -- "$c" || printf '%s ' "$c"
  done
}

# wait_checks <S#> <id> <repo> <n>: block until required checks are green.
# Red stops the run. Green must also name every required context from branch
# protection as passing (--watch can return early when only some checks exist yet).
# "No checks reported" is accepted only when protection requires none. Missing or
# unreported required checks are retried six times, then the run stops. Nothing
# reaches admin-merge.sh on an unverified PR.
wait_checks() {
  local s=$1 id=$2 repo=$3 n=$4 i=0 out rc ctx miss
  out=$(mktemp "${TMPDIR:-/tmp}/nself-release-checks.XXXXXX")
  while :; do
    set +e
    gh pr checks "$n" -R "nself-org/$repo" --required --watch --interval 30 2>&1 | tee "$out"
    rc=${PIPESTATUS[0]}
    set -e
    if [ "$rc" -eq 0 ]; then
      miss=$(missing_required "$repo" "$n")
      if [ -z "$miss" ]; then rm -f "$out"; return 0; fi
      if [ "$miss" = "?" ]; then
        rm -f "$out"; die_step "$s" "$id" "cannot read the required checks of $repo#$n; nothing was merged"
      fi
    elif grep -q -i -E 'no (required )?checks' "$out"; then
      ctx=$(required_contexts "$repo")
      if [ -z "$ctx" ]; then
        say "$s $id: $repo requires no status checks on main; continuing to the owner prompt"
        rm -f "$out"; return 0
      fi
      if [ "$ctx" = "?" ]; then
        rm -f "$out"; die_step "$s" "$id" "cannot read the required checks of $repo; nothing was merged"
      fi
      miss=$(printf '%s' "$ctx" | tr '\n' ' ')
    else
      rm -f "$out"
      die_step "$s" "$id" "required checks on $repo#$n are not green; nothing was merged"
    fi
    i=$((i + 1))
    if [ "$i" -ge 6 ]; then
      rm -f "$out"
      die_step "$s" "$id" "$repo#$n: required checks not reported as passing after 6 tries (${miss}); nothing was merged"
    fi
    sleep "$POLL_SECONDS"
  done
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
