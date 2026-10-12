# shellcheck shell=bash
# Purpose: shared helpers for scripts/release/release.sh (state, plan lines, prompts,
#          polling, clones, PR open + owner-answered merge).
# Inputs:  globals set by release.sh: VERSION TAG PLAN YES WORK STATE ADMIN_MERGE
# Outputs: functions only; nothing runs on source.
# Constraints: bash 3.2 (no assoc arrays, no mapfile, no ${x,,}); no token is read,
#          printed or stored: only the ambient `gh` session is used.

POLL_SECONDS=20
# Docker image wait: 180 polls x 20 s = 60 min. The multi-arch admin build takes about
# 40 min under QEMU (v1.4.14), which the old 90 polls (30 min) never covered.
# shellcheck disable=SC2034  # read by image_step in steps3.sh
IMAGE_WAIT_POLLS=180

say() { printf '%s\n' "$*"; }
err() { printf 'release: %s\n' "$*" >&2; }

# die_step <S#> <id> <what failed> [resume-step]: exit 1 naming the step and how to
# resume (default: the same step).
die_step() {
  err "$1 $2 failed: $3"
  err "recovery: fix the cause, then run: scripts/release/release.sh --version ${VERSION} --from ${4:-$1}"
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

# repo_tag_sha <repo> <tag>: 0 and TAG_SHA = the commit the tag points at (GET);
# 1 when the tag is absent. Any other failure stops the run (call it outside $(...)).
TAG_SHA=""
repo_tag_sha() {
  # shellcheck disable=SC2034  # TAG_SHA is read by the callers
  if gh_try api "repos/nself-org/$1/commits/$2" --jq .sha; then TAG_SHA=$GH_OUT; return 0; fi
  if is_404 || printf '%s' "$GH_ERR" | grep -q -E '422|No commit found'; then return 1; fi
  err "cannot resolve $2 on $1: $GH_ERR"
  exit 1
}

# gh_need <what> <gh args...>: gh whose failure ends the step with a recovery line.
# The value is left in GH_OUT. Use it instead of a bare x=$(gh ...).
gh_need() {
  local what=$1
  shift
  gh_try "$@" || die_step "$CUR_S" "$CUR_ID" "cannot read $what: ${GH_ERR:-gh failed}"
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
# bump_all [--no-goldens]: S3 keeps the goldens (they go in the release PR); S2 passes
# --no-goldens because only admin files are committed there.
bump_all() { (cd "$WORK/cli" && bash scripts/bump-version.sh "$VERSION" "$@") >/dev/null; }

# --- pull requests ---
# pr_info <repo> <branch>: PR_INFO = "<n> <STATE>" for the branch, or empty.
PR_INFO=""
pr_info() {
  gh_need "the PRs of $1" pr list -R "nself-org/$1" --head "$2" --state all --json number,state \
    --jq '.[0] | select(.) | "\(.number) \(.state)"'
  PR_INFO=$GH_OUT
}
# pr_merged <repo> <n>: success when the PR is merged.
pr_merged() {
  gh_need "PR $1#$2" pr view "$2" -R "nself-org/$1" --json state --jq .state
  [ "$GH_OUT" = MERGED ]
}

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

# merge_pr <S#> <id> <repo> <n>: green checks, then admin-merge.sh with the
# owner's terminal. No stdin redirect, no pre-answer, and --yes never applies here.
merge_pr() {
  local s=$1 id=$2 repo=$3 n=$4
  if pr_merged "$repo" "$n"; then return 0; fi
  wait_checks "$s" "$id" "$repo" "$n"
  say "$s $id: handing the terminal to admin-merge.sh for $repo#$n (answer its prompt)"
  (cd "$WORK/$repo" && bash "$ADMIN_MERGE" --repo "nself-org/$repo" --pr "$n") \
    || die_step "$s" "$id" "admin-merge.sh failed on $repo#$n (check branch protection: it restores itself; see its audit log)"
  pr_merged "$repo" "$n" || die_step "$s" "$id" "$repo#$n is not merged after admin-merge.sh (declined at its prompt?)"
}

# pr_flow <S#> <id> <repo> <branch> <state-key> <title> <body>: reuse an open PR
# or open one from the commit already made; then merge it.
pr_flow() {
  local s=$1 id=$2 repo=$3 br=$4 key=$5 n
  pr_info "$repo" "$br"
  if [ -n "$PR_INFO" ]; then
    n=${PR_INFO%% *}
    say "$s $id: reusing $repo#$n (${PR_INFO##* })"
  else
    n=$(open_pr "$s" "$id" "$repo" "$br" "$6" "$7")
    say "$s $id: opened $repo#$n"
  fi
  state_set "$key" "$n"
  merge_pr "$s" "$id" "$repo" "$n"
}
