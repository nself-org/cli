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

# Mutation check: a contexts-only body must trip pins_kept.
MUT="${TMP}/toggle-mutant.sh"
sed -e 's/^      contexts: \[\],$/      contexts: (.required_status_checks.contexts \/\/ []),/' \
    -e 's/^      checks: \[(\.required_status_checks\.contexts.*$/      checks: []/' "${TOGGLE}" > "${MUT}"
if cmp -s "${TOGGLE}" "${MUT}"; then
  fail "mutation did not apply (toggle layout changed: update the sed in this test)"
else
  run_toggle "${MUT}" nself-org/cli cli-drift
  if pins_kept cli-drift; then fail "mutant (contexts-only body) was NOT detected"; else pass "mutant (contexts-only body) is detected"; fi
fi

if [ "${FAILS}" -gt 0 ]; then printf '\n%d case(s) failed\n' "${FAILS}"; exit 1; fi
printf '\nall toggle pin tests passed\n'
