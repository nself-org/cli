#!/usr/bin/env bash
# tools/imagebump/dispatch-wait.sh — dispatch a workflow on a branch and wait for THAT run.
#
# Purpose: the weekly pin-bump job (P7-HYG-34) must run the image probe and the
# stack smoke on the bump branch head and report each result. It dispatches the
# workflow, then picks the run its own dispatch created. It never takes "the
# latest run" (`gh run list -L 1`): another run of the same workflow may exist.
# Usage:  tools/imagebump/dispatch-wait.sh <workflow-file> <branch> <head-sha> [gh workflow run args...]
#         e.g. dispatch-wait.sh e2e-golden-path.yml chore/image-pins "$sha" -f source=local
# Inputs: GH_TOKEN (actions: write), GH_REPO or GITHUB_REPOSITORY (owner/name);
#         DISPATCH_POLL_SECONDS (default 180) and DISPATCH_POLL_INTERVAL (default 5)
#         bound the wait for the run to appear.
# Output: stdout gets RUN_ID=, RUN_URL= and CONCLUSION= lines; the watch log goes
#         to stderr.
# Exit:   0 the run succeeded; 1 the run did not succeed or never appeared;
#         2 usage error.
# Selection: the run id is greater than the highest run id of that workflow seen
#         before the dispatch, and event=workflow_dispatch, headBranch=<branch>,
#         headSha=<head-sha>.
set -euo pipefail

if [ "$#" -lt 3 ]; then
  echo "usage: dispatch-wait.sh <workflow-file> <branch> <head-sha> [gh workflow run args...]" >&2
  exit 2
fi
workflow="$1"; branch="$2"; sha="$3"; shift 3
if [ -z "${GH_REPO:-}" ] && [ -n "${GITHUB_REPOSITORY:-}" ]; then export GH_REPO="$GITHUB_REPOSITORY"; fi
poll_seconds="${DISPATCH_POLL_SECONDS:-180}"
interval="${DISPATCH_POLL_INTERVAL:-5}"

before="$(gh run list --workflow "$workflow" --limit 100 --json databaseId --jq 'map(.databaseId) | max // 0')"
gh workflow run "$workflow" --ref "$branch" ${1+"$@"} >&2

run_id=""
waited=0
while [ "$waited" -le "$poll_seconds" ]; do
  run_id="$(gh run list --workflow "$workflow" --branch "$branch" --event workflow_dispatch --limit 50 \
    --json databaseId,headSha,headBranch \
    --jq "map(select(.databaseId > $before and .headSha == \"$sha\" and .headBranch == \"$branch\")) | map(.databaseId) | min // empty")"
  [ -n "$run_id" ] && break
  sleep "$interval"
  waited=$((waited + interval))
done
if [ -z "$run_id" ]; then
  echo "no run of $workflow appeared for $branch@$sha within ${poll_seconds}s" >&2
  echo "CONCLUSION=not_found"
  exit 1
fi

url="$(gh run view "$run_id" --json url --jq .url)"
echo "RUN_ID=$run_id"
echo "RUN_URL=$url"
rc=0
gh run watch "$run_id" --exit-status >&2 || rc=$?
conclusion="$(gh run view "$run_id" --json conclusion --jq '.conclusion // "unknown"')"
echo "CONCLUSION=$conclusion"
[ "$conclusion" = "success" ] || rc=1
exit "$rc"
