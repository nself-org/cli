#!/usr/bin/env bash
# Exercise S6's standalone gate repair without GitHub or release mutations.
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
for state in failure success absent; do
  (
    . "$ROOT/scripts/release/lib/steps2.sh"
    TAG=v9.9.9
    GH_OUT=''
    calls=''
    gh_need() {
      [[ $* == *homebrew-version-lockstep.yml* && $* == *"--branch $TAG"* ]]
      if [[ $state == failure ]]; then GH_OUT=5552; else GH_OUT=''; fi
    }
    gh() { calls="$*"; }
    say() { :; }
    die_step() { exit 1; }
    find_run() { RUN_ID=''; }
    rerun_lockstep 0
    if [[ $state == failure ]]; then
      [[ $calls == 'run rerun 5552 -R nself-org/cli --failed' ]]
    else
      [[ -z $calls ]]
    fi
  )
done
echo 'PASS: S6 repairs failed standalone lockstep runs only'
