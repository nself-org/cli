#!/usr/bin/env bash
# toggle_pins_test.sh: branch-protection-toggle.sh must keep `checks` app pins
# (P7-CI-60, review F2). Runs `--on --dry-run` against recorded app-pinned
# fixtures through a stub gh and fails if any checks[].app_id present in the
# fixture is missing or changed in the PUT body. A mutant toggle that builds a
# contexts-only body must make the same assertion fail (mutation check).
# Also checks that pending_nself_ci is ignored: a fixture equal to the baseline
# reports a no-op and never mentions nself-ci.
# Usage: toggle_pins_test.sh   (exit 0 when all pass, 1 otherwise)

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOGGLE="${HERE}/../../../branch-protection-toggle.sh"
POLICY="${HERE}/../../../.policy/branch-protection.yaml"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT
mkdir -p "${TMP}/bin" "${TMP}/stub" "${TMP}/home" "${TMP}/work"
cp "${HERE}/stub-gh.sh" "${TMP}/bin/gh"
chmod +x "${TMP}/bin/gh"
export STUB_DIR="${TMP}/stub" HOME="${TMP}/home" TMPDIR="${TMP}/work"
export PATH="${TMP}/bin:${PATH}"
FAILS=0
pass() { printf 'PASS  %s\n' "$1"; }
fail() { printf 'FAIL  %s\n' "$1"; FAILS=$((FAILS + 1)); }

# run_toggle <toggle-script> <repo> <fixture>: dry-run output in ${TMP}/out.
run_toggle() {
  rm -rf "${STUB_DIR:?}"/* "${HOME}/.nself"
  cp "${HERE}/$3.json" "${STUB_DIR}/state.json"
  bash "$1" --on --dry-run --repo "$2" --policy "${POLICY}" > "${TMP}/out" 2>&1
}
# pins_kept <fixture>: succeeds when the PUT body in ${TMP}/out carries every
# fixture check that has a non-null app_id, unchanged.
pins_kept() {
  sed -n '/^{/,$p' "${TMP}/out" | jq -e --slurpfile fx "${HERE}/$1.json" '
    (.required_status_checks.checks // []) as $got
    | [$fx[0].required_status_checks.checks[] | select(.app_id != null)
       | . as $want | ($got | any(.context == $want.context and .app_id == $want.app_id))]
    | (length > 0) and all' > /dev/null 2>&1
}

for pair in "nself-org/cli cli-drift" "nself-org/plugins plugins-drift"; do
  repo="${pair%% *}"; fx="${pair##* }"
  run_toggle "${TOGGLE}" "${repo}" "${fx}"
  if grep -q 'DRY-RUN: would PUT' "${TMP}/out"; then pass "${repo}: drifted fixture produces a PUT body"; else fail "${repo}: no PUT body"; fi
  if pins_kept "${fx}"; then pass "${repo}: every app_id pin kept in the PUT body"; else fail "${repo}: a pin is missing or changed"; fi
  if grep -q nself-ci "${TMP}/out"; then fail "${repo}: pending_nself_ci leaked into the toggle"; else pass "${repo}: pending_nself_ci ignored"; fi
  if [ ! -f "${STUB_DIR}/calls" ] || ! grep -q '^PUT ' "${STUB_DIR}/calls"; then pass "${repo}: dry-run sent no PUT"; else fail "${repo}: dry-run sent a PUT"; fi
done

# A non-15368 pin must survive too (cli-drift pins the second check to 12345).
run_toggle "${TOGGLE}" nself-org/cli cli-drift
if sed -n '/^{/,$p' "${TMP}/out" | jq -e '.required_status_checks.checks | any(.app_id == 12345)' > /dev/null 2>&1; then
  pass "a non-default app pin (12345) survives"; else fail "app pin 12345 was changed"; fi

# Fixtures equal to the baseline: no-op, pending ignored.
for pair in "nself-org/cli cli-live" "nself-org/plugins plugins-live" "nself-org/packages packages-live"; do
  repo="${pair%% *}"; fx="${pair##* }"
  run_toggle "${TOGGLE}" "${repo}" "${fx}"
  if grep -q 'already matches baseline' "${TMP}/out" && ! grep -q nself-ci "${TMP}/out"; then
    pass "${repo}: pinned live fixture reads as no-op"; else fail "${repo}: pinned live fixture reads as drift"; fi
done

# Once `nself-ci-protect.sh --apply` has put nself-ci live, `toggle --on` must
# not strip it (it is still only a pending check, never applied by the toggle).
for pair in "nself-org/cli cli-gated" "nself-org/plugins plugins-gated"; do
  repo="${pair%% *}"; fx="${pair##* }"
  run_toggle "${TOGGLE}" "${repo}" "${fx}"
  if grep -q 'already matches baseline' "${TMP}/out"; then pass "${repo}: live nself-ci survives --on (no-op)"; else fail "${repo}: --on would strip the live nself-ci"; fi
done
run_toggle "${TOGGLE}" nself-org/cli cli-gated-drift
if sed -n '/^{/,$p' "${TMP}/out" | jq -e '.required_status_checks.checks | any(.context == "nself-ci" and .app_id == 15368)' > /dev/null 2>&1; then
  pass "nself-org/cli: a drifted --on PUT body keeps the live nself-ci pin"; else fail "nself-org/cli: PUT body drops nself-ci"; fi
if pins_kept cli-gated-drift; then pass "nself-org/cli: all pins of the gated fixture kept"; else fail "nself-org/cli: gated fixture pin lost"; fi

# A context pinned to two apps live cannot be carried: refuse, send nothing.
rm -rf "${STUB_DIR:?}"/* "${HOME}/.nself"
cp "${HERE}/cli-dup-pins.json" "${STUB_DIR}/state.json"
rc=0
bash "${TOGGLE}" --on --repo nself-org/cli --policy "${POLICY}" > "${TMP}/out" 2>&1 || rc=$?
if [ "${rc}" -ne 0 ] && grep -q 'more than one app' "${TMP}/out" && ! grep -q '^PUT ' "${STUB_DIR}/calls"; then
  pass "duplicate live pins are refused and nothing is PUT"; else fail "duplicate live pins were not refused (rc=${rc})"; fi

# A 200 that is not a protection document (here `{}`, an incomplete body, or
# another repo's answer) is refused, never turned into a PUT that unpins checks.
for bad in '{}' '{"url":"https://api.github.com/repos/nself-org/cli/branches/main/protection"}' "$(cat "${HERE}/plugins-live.json")"; do
  rm -rf "${STUB_DIR:?}"/* "${HOME}/.nself"
  printf '%s' "${bad}" > "${STUB_DIR}/state.json"
  for extra in "" "--dry-run"; do
    rc=0
    # shellcheck disable=SC2086  # $extra is empty or one flag
    bash "${TOGGLE}" --on --repo nself-org/cli --policy "${POLICY}" ${extra} > "${TMP}/out" 2>&1 || rc=$?
    if [ "${rc}" -ne 0 ] && grep -q 'not with a branch-protection document' "${TMP}/out" \
       && ! grep -q 'would PUT' "${TMP}/out" && ! grep -q '^PUT ' "${STUB_DIR}/calls"; then
      pass "odd 200 body refused (${extra:-apply}): ${bad:0:30}"; else fail "odd 200 body not refused (${extra:-apply}, rc=${rc}): ${bad:0:30}"; fi
  done
done

# Mutation check: a contexts-only body must trip pins_kept.
MUT="${TMP}/toggle-mutant.sh"
# shellcheck disable=SC2016  # sed patterns hold a literal $names
sed -e 's/^      contexts: \[\],$/      contexts: (.required_status_checks.contexts \/\/ []),/' \
    -e 's/^      checks: (\[\$names.*$/      checks: []/' "${TOGGLE}" > "${MUT}"
if cmp -s "${TOGGLE}" "${MUT}"; then
  fail "mutation did not apply (toggle layout changed: update the sed in this test)"
else
  run_toggle "${MUT}" nself-org/cli cli-drift
  if pins_kept cli-drift; then fail "mutant (contexts-only body) was NOT detected"; else pass "mutant (contexts-only body) is detected"; fi
fi

if [ "${FAILS}" -gt 0 ]; then printf '\n%d case(s) failed\n' "${FAILS}"; exit 1; fi
printf '\nall toggle pin tests passed\n'
