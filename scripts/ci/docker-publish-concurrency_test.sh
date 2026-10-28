#!/usr/bin/env bash
# Purpose: assert the docker-publish.yml concurrency group expression normalises
#          dispatch input to v<ver> form and handles blank input correctly.
#          This kills two mutants: (1) dispatch keys on github.ref instead of
#          inputs.version, (2) dispatch with "1.4.13" (no v) produces a different
#          key than workflow_run with head_branch=v1.4.13.
# Inputs:  none.
# Outputs: "ok <case>" per passing case; exit 0 when all pass, exit 1 on failure.
# Constraints: bash 3.2 compatible; no network, no gh credential.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
WORKFLOW="${HERE}/../../.github/workflows/docker-publish.yml"

[ -f "${WORKFLOW}" ] || { echo "FAIL: ${WORKFLOW} not found" >&2; exit 1; }

# Extract the concurrency group expression (line starting with "  group:").
EXPR=$(sed -n '/^concurrency:/,/^[^ #]/p' "${WORKFLOW}" | sed -n 's/^  group: //p' | head -1)
[ -n "${EXPR}" ] || { echo "FAIL: could not extract concurrency group expression" >&2; exit 1; }

# Case 1: expression must contain the workflow_dispatch && inputs.version branch.
# Kills mutant where dispatch keys on github.ref instead of inputs.version.
if ! printf '%s' "${EXPR}" | grep -q "workflow_dispatch.*inputs.version"; then
  echo "FAIL: expression lacks workflow_dispatch && inputs.version branch" >&2
  printf 'expression: %s\n' "${EXPR}" >&2
  exit 1
fi
echo "ok dispatch-keys-on-inputs-version"

# Case 2: expression must normalise bare version to v<ver> via format('v{0}', ...).
# Kills mutant where dispatch with "1.4.13" produces key "...-1.4.13" while
# workflow_run with head_branch=v1.4.13 produces "...-v1.4.13" — they'd race.
if ! printf '%s' "${EXPR}" | grep -q "format.*v{0}.*inputs.version\|startsWith.*inputs.version.*v"; then
  echo "FAIL: expression lacks v-prefix normalisation for bare dispatch input" >&2
  printf 'expression: %s\n' "${EXPR}" >&2
  exit 1
fi
echo "ok dispatch-normalises-bare-version-to-v-prefix"

# Case 3: blank dispatch input must fall through to github.ref, never produce "".
# Kills mutant where blank input yields key "...-" (empty suffix), racing with
# any other empty-suffix run.
if ! printf '%s' "${EXPR}" | grep -q "inputs.version != ''"; then
  echo "FAIL: expression lacks blank-input guard (inputs.version != '')" >&2
  printf 'expression: %s\n' "${EXPR}" >&2
  exit 1
fi
echo "ok blank-dispatch-falls-through-to-github-ref"

echo "docker-publish-concurrency_test: all cases passed"
