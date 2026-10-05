#!/usr/bin/env bash
# Two-command drill (EPIC P7-CANON D14): `nself init && nself start` with zero
# prompts on a clean host, timed.
#
#   NSELF_BIN=/path/to/nself bash scripts/drill/two-command.sh --host
#   bash scripts/drill/two-command.sh --self-test
#
# --host runs in an empty temp dir with CI, GITHUB_ACTIONS and NSELF_NONINTERACTIVE
# unset for the drilled commands:
#   leg A  stdin /dev/null, no TTY: init then start must succeed.
#   leg B  a pseudo-TTY that never receives input: init then start must finish
#          inside their timeouts, so any prompt blocks and fails the drill.
# Health is `nself status` exit 0 (mode agnostic) plus a GraphQL answer from Hasura.
# Time to healthy (init start to healthy, seconds) is recorded in drill.json and
# perfbench.json (perfbench/v1). It is informational and never gated.
#
# Env: NSELF_BIN (required for --host), NSELF_V15 (unset or 1), DRILL_OUT (result
# dir, default ./drill-out), DRILL_GRAPHQL_URL (default http://localhost:8080/v1/graphql),
# DRILL_INIT_TIMEOUT (120), DRILL_START_TIMEOUT (600), DRILL_HEALTH_TIMEOUT (600).
# Runs only on GitHub-hosted runners or a local Linux box. Never point it at a server.
# Exit: 0 drill passed, 1 drill failed, 2 bad usage or missing tool.

set -u
HERE=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=scripts/drill/lib.sh
. "$HERE/lib.sh"

INIT_T=${DRILL_INIT_TIMEOUT:-120}
START_T=${DRILL_START_TIMEOUT:-600}
HEALTH_T=${DRILL_HEALTH_TIMEOUT:-600}
GQL_URL=${DRILL_GRAPHQL_URL:-http://localhost:8080/v1/graphql}
OUT=${DRILL_OUT:-$PWD/drill-out}
WORK=""

log() { printf 'drill: %s\n' "$*"; }
die() { printf 'drill: FAIL %s\n' "$*" >&2; }

# cleanup_dir <dir>: measurement is done; stop the stack and drop its volumes.
cleanup_dir() {
  [ -d "$1" ] || return 0
  (
    cd "$1" || exit 0
    run_leg_a 120 "$OUT/cleanup.log" "$NSELF_BIN" stop || true
    docker compose down -v --remove-orphans >>"$OUT/cleanup.log" 2>&1 || true
  )
}

finish() {
  [ -n "$WORK" ] && cleanup_dir "$WORK/a" && cleanup_dir "$WORK/b"
  [ -n "$WORK" ] && rm -rf "$WORK"
  return 0
}

# wait_healthy: poll `nself status` until exit 0 within HEALTH_T seconds.
wait_healthy() {
  local end=$(($(date +%s) + HEALTH_T))
  while [ "$(date +%s)" -lt "$end" ]; do
    if drill_env "$NSELF_BIN" status </dev/null >/dev/null 2>&1; then return 0; fi
    sleep 5
  done
  return 1
}

# wait_graphql <env-file>: the stack can be healthy a moment before routing answers.
wait_graphql() {
  local i=0
  while [ "$i" -lt 12 ]; do
    graphql_probe "$1" "$GQL_URL" && return 0
    sleep 5
    i=$((i + 1))
  done
  return 1
}

# leg_a: init and start without a TTY, then health and GraphQL. Sets TTH, GQL.
leg_a() {
  mkdir -p "$WORK/a" && cd "$WORK/a" || return 1
  local t0 rc
  t0=$(date +%s)
  run_leg_a "$INIT_T" "$OUT/a-init.log" "$NSELF_BIN" init
  rc=$?
  if [ "$rc" -ne 0 ]; then die "leg A: nself init exited $rc (see a-init.log)"; return 1; fi
  run_leg_a "$START_T" "$OUT/a-start.log" "$NSELF_BIN" start
  rc=$?
  if [ "$rc" -ne 0 ]; then die "leg A: nself start exited $rc (see a-start.log)"; return 1; fi
  if ! wait_healthy; then die "leg A: nself status did not reach exit 0 within ${HEALTH_T}s"; return 1; fi
  TTH=$(($(date +%s) - t0))
  if wait_graphql "$WORK/a/.env"; then GQL=true; else GQL=false; fi
  log "leg A healthy in ${TTH}s, graphql_ok=$GQL"
  return 0
}

# leg_b: init and start under a pseudo-TTY with no input. Sets PROMPT.
leg_b() {
  mkdir -p "$WORK/b" && cd "$WORK/b" || return 1
  local rc
  run_leg_b "$INIT_T" "$OUT/b-init.log" "$NSELF_BIN" init
  rc=$?
  case "$rc" in 124 | 137) PROMPT=true; die "leg B: nself init blocked (prompt?)"; return 1 ;; esac
  if [ "$rc" -ne 0 ]; then die "leg B: nself init exited $rc (see b-init.log)"; return 1; fi
  run_leg_b "$START_T" "$OUT/b-start.log" "$NSELF_BIN" start
  rc=$?
  case "$rc" in 124 | 137) PROMPT=true; die "leg B: nself start blocked (prompt?)"; return 1 ;; esac
  if [ "$rc" -ne 0 ]; then die "leg B: nself start exited $rc (see b-start.log)"; return 1; fi
  log "leg B finished without a prompt"
  return 0
}

host_mode() {
  [ -n "${NSELF_BIN:-}" ] && [ -x "$NSELF_BIN" ] || { die "NSELF_BIN must be an executable nself binary"; return 2; }
  [ -n "$TIMEOUT_BIN" ] || { die "timeout (coreutils) is required"; return 2; }
  command -v jq >/dev/null 2>&1 || { die "jq is required"; return 2; }
  NSELF_BIN=$(cd "$(dirname "$NSELF_BIN")" && pwd)/$(basename "$NSELF_BIN")
  [ -n "${NSELF_V15:-}" ] || unset NSELF_V15
  mkdir -p "$OUT"
  OUT=$(cd "$OUT" && pwd)
  WORK=$(mktemp -d "${TMPDIR:-/tmp}/two-command-drill.XXXXXX")
  trap finish EXIT
  TTH=null GQL=false PROMPT=false
  local mode=v14 la=pass lb=skipped status=0
  [ -n "${NSELF_V15:-}" ] && mode=v15
  if ! leg_a; then la=fail; status=1; fi
  # Always run leg B: a prompt that leg A hid by failing early is still reported.
  if [ "$status" -eq 0 ]; then
    cleanup_dir "$WORK/a"
    if leg_b; then lb=pass; else lb=fail; status=1; fi
  fi
  [ "$status" -eq 0 ] && [ "$GQL" != true ] && { die "GraphQL probe did not answer"; status=1; }
  write_results "$OUT" "$mode" "$TTH" "$GQL" "$PROMPT" "$la" "$lb"
  log "mode=$mode time_to_healthy_s=$TTH graphql_ok=$GQL prompt_detected=$PROMPT leg_a=$la leg_b=$lb (informational, never gated)"
  return $status
}

# --self-test: the detector against fixtures, and the writer against jq.
self_test() {
  [ -n "$TIMEOUT_BIN" ] || { die "timeout (coreutils) is required"; return 2; }
  command -v jq >/dev/null 2>&1 || { die "jq is required"; return 2; }
  local d rc=0 la=0
  d=$(mktemp -d "${TMPDIR:-/tmp}/two-command-selftest.XXXXXX")
  if detect_prompt 3 bash "$HERE/fixtures/prompt.sh"; then
    log "self-test: prompt_detected true for fixtures/prompt.sh under leg B"
  else die "self-test: prompt fixture was NOT detected under leg B"; rc=1; fi
  run_leg_a 10 "$d/a.log" bash "$HERE/fixtures/prompt.sh" || la=$?
  if [ "$la" -ne 0 ]; then log "self-test: prompt fixture fails under leg A (rc=$la)"
  else die "self-test: prompt fixture passed under leg A"; rc=1; fi
  if detect_prompt 10 bash "$HERE/fixtures/ok.sh"; then
    die "self-test: clean fixture reported as a prompt"; rc=1
  else log "self-test: prompt_detected false for fixtures/ok.sh"; fi
  write_results "$d" v14 42 true false pass pass
  if jq -e '.schema=="two-command-drill/v1" and .mode=="v14" and .graphql_ok==true and .prompt_detected==false and .time_to_healthy_s==42' "$d/drill.json" >/dev/null &&
    jq -e '.schema=="perfbench/v1" and .scenario=="two-command-drill" and .runs==1 and .metrics[0].name=="time_to_healthy" and .metrics[0].unit=="s" and .metrics[0].p50==42 and .metrics[0].n==1' "$d/perfbench.json" >/dev/null; then
    log "self-test: drill.json and perfbench.json valid"
  else die "self-test: synthetic result files invalid"; rc=1; fi
  rm -rf "$d"
  [ "$rc" -eq 0 ] && log "self-test: OK"
  return $rc
}

case "${1:-}" in
  --host) host_mode ;;
  --self-test) self_test ;;
  *) printf 'usage: %s --host | --self-test\n' "$0" >&2; exit 2 ;;
esac
