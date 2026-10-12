# shellcheck shell=bash
# Purpose: the required-checks gate that runs before any admin-merge.sh call.
# Inputs:  helpers from common.sh (gh_try, die_step, say); globals CUR_S CUR_ID
# Outputs: wait_checks returns 0 only when the PR's required checks are green AND
#          every required context named by branch protection is among the passing
#          ones, or protection requires none. Anything else ends in die_step.
# Constraints: GET-only; time-bounded (45 min of pending, 6 tries for unreported
#          checks); never calls admin-merge.sh itself.

CHECKS_BUDGET=90   # polls of 30 s = 45 min of pending checks

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
# (by name; a skipped check counts as satisfied), space separated; "?" if unreadable; nothing when all are present.
missing_required() {
  local ctx got
  ctx=$(required_contexts "$1")
  if [ -z "$ctx" ]; then return 0; fi
  if [ "$ctx" = "?" ]; then printf '?\n'; return 0; fi
  got=$(gh pr checks "$2" -R "nself-org/$1" --required --json name,bucket \
    --jq '.[] | select(.bucket == "pass" or .bucket == "skipping") | .name') || { printf '?\n'; return 0; }
  printf '%s\n' "$ctx" | while IFS= read -r c; do
    printf '%s\n' "$got" | grep -qxF -- "$c" || printf '%s ' "$c"
  done
}

# wait_checks <S#> <id> <repo> <n>. Non-watch `gh pr checks --required` in a loop:
# exit 0 green, 8 pending (keep waiting, bounded), other: red or "no checks".
# "No checks reported" is accepted only when protection requires none; checks that
# are required but missing or unreported are retried six times, then the run stops.
wait_checks() {
  local s=$1 id=$2 repo=$3 n=$4 t=0 i=0 out rc ctx miss="" POLL_SECONDS=30
  out=$(mktemp "${TMPDIR:-/tmp}/nself-release-checks.XXXXXX")
  while :; do
    set +e
    gh pr checks "$n" -R "nself-org/$repo" --required > "$out" 2>&1
    rc=$?
    set -e
    head -4 "$out" | sed "s/^/  $repo#$n: /"
    if [ "$rc" -eq 8 ]; then
      t=$((t + 1))
      if [ "$t" -ge "$CHECKS_BUDGET" ]; then
        rm -f "$out"
        die_step "$s" "$id" "$repo#$n: required checks still pending after 45 minutes; nothing was merged. Look at the PR, then --from $s"
      fi
      sleep "$POLL_SECONDS"; continue
    fi
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
