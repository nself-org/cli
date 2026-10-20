#!/usr/bin/env bash
# Verify elapsed polling time without sleeping or contacting GitHub.
set -euo pipefail
RELDIR="$(cd "$(dirname "$0")" && pwd)"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
for minutes in 1 2 45; do
  export RELEASE_WAIT_MINUTES="$minutes"
  . "$RELDIR/lib/common.sh"
  . "$RELDIR/lib/checks.sh"
  . "$RELDIR/lib/tag.sh"
  sleep() { echo "$(( $(cat "$TMP/seconds") + $1 ))" > "$TMP/seconds"; }
  gh() { return 8; }
  die_step() { exit 17; }
  echo 0 > "$TMP/seconds"
  rc=0
  (wait_checks S2 test admin 1) > "$TMP/output" 2>&1 || rc=$?
  test "$rc" -eq 17
  test "$(cat "$TMP/seconds")" -eq "$(( minutes * 60 ))" || { echo "FAIL checks budget $minutes: $(cat "$TMP/seconds") seconds"; exit 1; }
  echo 0 > "$TMP/seconds"
  lic_state() { LIC=pending; }
  lic_wait main > "$TMP/output"
  test "$(cat "$TMP/seconds")" -eq "$(( minutes * 60 ))" || { echo "FAIL license budget $minutes"; exit 1; }
  echo "PASS exact $minutes-minute PR and license budgets"
done
for invalid in 0 00 abc; do
  if (export RELEASE_WAIT_MINUTES="$invalid"; . "$RELDIR/lib/common.sh") > /dev/null 2>&1; then
    echo "FAIL accepted wait budget $invalid"; exit 1
  fi
done
echo 'PASS invalid wait budgets rejected'
