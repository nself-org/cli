#!/usr/bin/env bash
# Purpose: assert ci.yml and docker-publish.yml parse as YAML, and that the
#          docker-publish.yml concurrency group expression normalises dispatch input to v<ver> form and handles blank input correctly.
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

# Case 0: both workflows must parse as YAML. A mis-indented step in ci.yml once
# made the whole workflow invalid while every shell test here still passed.
yaml_ok() {
  if command -v ruby >/dev/null 2>&1; then
    ruby -ryaml -e 'YAML.load_file(ARGV[0])' "$1" 2>/dev/null
  elif python3 -c 'import yaml' >/dev/null 2>&1; then
    python3 -c 'import sys, yaml; yaml.safe_load(open(sys.argv[1]))' "$1" 2>/dev/null
  else
    echo "FAIL: neither ruby nor python3+yaml available to parse workflows" >&2
    exit 1
  fi
}
for wf in ci.yml docker-publish.yml; do
  yaml_ok "${HERE}/../../.github/workflows/${wf}" || { echo "FAIL: ${wf} is not valid YAML" >&2; exit 1; }
  echo "ok ${wf}-parses"
done

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

# Case 4 (D-0204): :latest promotion is its own job in one shared concurrency
# group, and the per-tag docker job no longer writes :latest. Kills the mutant
# where promotion runs inside the per-tag group (releases race, :latest goes
# backwards) or the group is keyed per tag.
JOB=$(sed -n '/^  promote-latest:/,$p' "${WORKFLOW}")
[ -n "${JOB}" ] || { echo "FAIL: no promote-latest job" >&2; exit 1; }
printf '%s\n' "${JOB}" | grep -qE '^      group: docker-publish-latest$' \
  || { echo "FAIL: promote-latest lacks the shared literal group 'docker-publish-latest'" >&2; exit 1; }
printf '%s\n' "${JOB}" | grep -q 'cancel-in-progress: false' \
  || { echo "FAIL: promote-latest must not cancel an in-flight promotion" >&2; exit 1; }
# queue: max keeps the newest pending promotion when an older build finishes
# later; without it GitHub keeps one pending run and drops the rest.
printf '%s\n' "${JOB}" | grep -qE '^      queue: max$' \
  || { echo "FAIL: promote-latest lacks 'queue: max' (newest pending promotion could be dropped)" >&2; exit 1; }
printf '%s\n' "${JOB}" | grep -q 'docker-publish-promote.sh' \
  || { echo "FAIL: promote-latest does not run the revalidating script" >&2; exit 1; }
if sed '/^  promote-latest:/,$d' "${WORKFLOW}" | grep -v '^ *#' | grep -q 'imagetools create'; then
  echo "FAIL: the per-tag docker job still promotes :latest outside the shared group" >&2
  exit 1
fi
echo "ok latest-promotion-is-serialised-and-revalidated"

echo "docker-publish-concurrency_test: all cases passed"
