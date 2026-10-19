#!/usr/bin/env bash
# Shared helpers for the lifecycle drill legs clean-host and upgrade-continuity (P7-CI-20).
# Sourced by their `run` files, never run. Not a drill itself: it holds no `run` file.
#
# Purpose: give a multi-step drill one nested ci.evidence/v1 document (one check per step),
#   a throw-away HOME and project directory, the seeded-table row digest, and a cleanup
#   that stops the stack and drops its volumes on every exit path.
# Inputs (set by the caller before use): ROOT (repo root), LEG (leg name), OUT (result dir),
#   WORK (scratch dir), PROJ (project directory name), NSELF (the binary a step runs).
# Outputs: $OUT/<step>.log per step, $OUT/evidence.json, and the document as the last line
#   of stdout (ev_emit). All other messages go to stderr.
# Constraints: bash 3.2 compatible (macOS); needs jq, timeout (or gtimeout), docker with compose.
#   No secret is printed: the step logs are the CLI's own output and a failed step's excerpt
#   is its last five log lines, ANSI-stripped and cut to 800 characters. Runs only on a runner or a
#   local box with Docker; never point it at a server.

# shellcheck source=/dev/null disable=SC2153
. "$ROOT/scripts/drill/lib.sh" # TIMEOUT_BIN, graphql_probe, drill_env
# shellcheck source=/dev/null
. "$ROOT/scripts/drill/stack-lock.sh" # stack_lock_acquire, stack_lock_release

log() { printf 'drill: %s\n' "$*" >&2; }

sha256_of() { if command -v sha256sum >/dev/null 2>&1; then sha256sum | cut -c1-64; else shasum -a 256 | cut -c1-64; fi; }

# need_tools: exit 2 (precondition) when a tool is missing, before anything is created.
need_tools() {
  local t
  [ -n "$TIMEOUT_BIN" ] || { log "timeout (coreutils) is required"; return 2; }
  for t in jq docker git; do
    command -v "$t" >/dev/null 2>&1 || { log "$t is required"; return 2; }
  done
  docker compose version >/dev/null 2>&1 || { log "docker compose is required"; return 2; }
  docker info >/dev/null 2>&1 || { log "the Docker daemon is not reachable"; return 2; }
  return 0
}

# ---- evidence ---------------------------------------------------------------------------

# ev_begin <leg> <out-dir>: start the document. Steps append to $EV_OUT/checks.ndjson.
ev_begin() {
  EV_LEG=$1 EV_OUT=$2
  mkdir -p "$EV_OUT"
  EV_CHECKS=$EV_OUT/checks.ndjson
  : >"$EV_CHECKS"
  EV_START=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  EV_T0=$(date +%s)
  EV_TOOLS='{}'
}

# ev_tool <name> <version>: record a tool or binary version in tool_versions.
ev_tool() { EV_TOOLS=$(printf '%s' "$EV_TOOLS" | jq -c --arg k "$1" --arg v "$2" '.[$k] = $v'); }

# ev_step <id> <pass|fail|skip> [excerpt]: append one nested check.
ev_step() {
  jq -nc --arg id "$1" --arg job "drill-$EV_LEG" --arg r "$2" --arg x "${3:-}" '
    {id: $id, job_id: $job, kind: "test", tool: "bash", tool_version: null, required: true,
     substantive: true, result: $r, findings: (if $r == "fail" then 1 else 0 end),
     excerpt: ($x | .[0:800]), log_ref: null}
    + (if $r == "skip" then {reason: "dep.failed"} else {} end)
    + (if $r == "fail" then {failure_class: "code"} else {} end)' >>"$EV_CHECKS"
}

# ev_emit: write $EV_OUT/evidence.json and print it as one compact line on stdout.
ev_emit() {
  local rev state digest root rid trig trust prov iso plat bound reason fin fstate freason
  rev=$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || true)
  printf '%s' "$rev" | grep -Eq '^[0-9a-f]{40}$' || rev=${GITHUB_SHA:-}
  printf '%s' "$rev" | grep -Eq '^[0-9a-f]{40}$' || rev=0000000000000000000000000000000000000000
  state=clean digest=null bound=bound reason=null fstate=current freason=null
  if [ -n "$(git -C "$ROOT" status --porcelain --untracked-files=no 2>/dev/null)" ]; then
    state=dirty bound=unbound reason='"dirty"' fstate=unknown freason='"binding.dirty"'
    digest="\"$(git -C "$ROOT" diff HEAD 2>/dev/null | sha256_of)\""
  fi
  root=$(git -C "$ROOT" rev-list --max-parents=0 HEAD 2>/dev/null | head -n 1)
  rid="local:$(printf '%s' "$root" | sha256_of)"
  case "${GITHUB_EVENT_NAME:-}" in
    schedule) trig=schedule ;; pull_request) trig=pull_request ;; workflow_dispatch) trig=manual ;; *) trig=local ;;
  esac
  prov=local iso=process trust=owner
  if [ -n "${GITHUB_ACTIONS:-}" ]; then prov=github iso=hosted-disposable trust=internal; fi
  case "$(uname -m)" in x86_64) plat=amd64 ;; aarch64 | arm64) plat=arm64 ;; *) plat=$(uname -m) ;; esac
  plat="$(uname -s | tr '[:upper:]' '[:lower:]')/$plat"
  fin=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  jq -nc --slurpfile checks "$EV_CHECKS" --argjson tools "$EV_TOOLS" \
    --arg rev "$rev" --arg rid "$rid" --arg run "$EV_LEG-$EV_T0" --arg leg "$EV_LEG" --arg trig "$trig" \
    --arg trust "$trust" --arg state "$state" --argjson digest "$digest" --arg bound "$bound" \
    --argjson reason "$reason" --arg fstate "$fstate" --argjson freason "$freason" --arg prov "$prov" --arg iso "$iso" --arg plat "$plat" \
    --arg start "$EV_START" --arg fin "$fin" --argjson ms "$((($(date +%s) - EV_T0) * 1000))" '
    ($checks) as $c
    | ("0" * 64) as $zero
    | {schema: "ci.evidence/v1", revision: $rev, repo_id: $rid, run_id: $run,
       pipeline: {id: "drills", name: "drills", kind: "drill"}, trigger: $trig, source_trust: $trust,
       policy_digest: $zero, input_digests: {config: $zero, lockfiles: [], generated: []},
       worktree_state: $state, tracked_changes_digest: $digest,
       binding: {state: $bound, reason: $reason},
       runner: {id: $prov, provider: $prov, platform: $plat, isolation: $iso},
       producer: {kind: "self-reported", version: "1"},
       timings: {queued_at: $start, started_at: $start, finished_at: $fin, duration_ms: $ms},
       tool_versions: $tools, checks: $c, artifacts: [], retries: [],
       selection: {mode: "full", reason: "selection.no_base", digest: $zero},
       result: (if any($c[]; .result == "fail" or .result == "error") then "fail" else "pass" end),
       counts: {pass: ([$c[] | select(.result == "pass")] | length),
                fail: ([$c[] | select(.result == "fail")] | length),
                skip: ([$c[] | select(.result == "skip")] | length),
                error: ([$c[] | select(.result == "error")] | length), total: ($c | length)},
       freshness: {state: $fstate, reason: $freason}, signature: {kind: "coordinator-recorded"}}' |
    tee "$EV_OUT/evidence.json"
}

# ev_result: 0 when no step failed (read from the checks file).
ev_result() { ! jq -e 'select(.result == "fail" or .result == "error")' "$EV_CHECKS" >/dev/null; }

# note <text>: a step's pass excerpt, shown in the document.
note() { printf '%s' "$*" >"$EV_OUT/$CUR_STEP.note"; }

# run_steps <id>...: run step_<id> (dashes become underscores) in this shell with stdin from
# /dev/null and output in $OUT/<id>.log; the first failure marks the later steps skipped.
run_steps() {
  local id fn rc failed=0 ex
  for id in "$@"; do
    if [ "$failed" -eq 1 ]; then ev_step "$id" skip ""; continue; fi
    fn=step_${id//-/_}
    CUR_STEP=$id
    log "step $id"
    rc=0
    "$fn" </dev/null >"$EV_OUT/$id.log" 2>&1 || rc=$?
    if [ "$rc" -eq 0 ]; then
      ev_step "$id" pass "$(cat "$EV_OUT/$id.note" 2>/dev/null)"
    else
      ex=$(tail -n 5 "$EV_OUT/$id.log" | sed "s/$(printf '\033')\[[0-9;]*[A-Za-z]//g" | tr '\n' ' ')
      ev_step "$id" fail "exit $rc: $ex"
      log "step $id FAILED ($rc); see $id.log"
      failed=1
    fi
  done
}

# ---- project and stack --------------------------------------------------------------------

# isolate_home: a clean host has no ~/.nself. Plugins and CLI state go to $WORK/home.
isolate_home() {
  export DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker}
  export HOME=$WORK/home
  mkdir -p "$HOME"
}

# ref_bin: the cli under test (N+1): NSELF_BIN, else .nself/bin/nself built from this checkout.
ref_bin() {
  REF_BIN=${NSELF_BIN:-$ROOT/.nself/bin/nself}
  if [ ! -x "$REF_BIN" ]; then
    command -v go >/dev/null 2>&1 || { log "NSELF_BIN $REF_BIN is missing and go is not on PATH"; return 2; }
    log "building nself from this checkout into $REF_BIN"
    mkdir -p "$(dirname "$REF_BIN")"
    (cd "$ROOT" && go build -mod=vendor -o "$REF_BIN" ./cmd/nself) || { log "go build failed"; return 2; }
  fi
  REF_BIN=$(cd "$(dirname "$REF_BIN")" && pwd)/$(basename "$REF_BIN")
}

# proj_init: after `nself init`, read the names the stack uses.
proj_init() {
  PROJ_DIR=$WORK/$PROJ
  PG_USER=$(sed -n 's/^POSTGRES_USER=//p' "$PROJ_DIR/.env" | head -n 1)
  PG_DB=$(sed -n 's/^POSTGRES_DB=//p' "$PROJ_DIR/.env" | head -n 1)
  PROJECT_NAME=$(sed -n 's/^PROJECT_NAME=//p' "$PROJ_DIR/.env" | head -n 1)
  [ -n "$PG_USER" ] && [ -n "$PG_DB" ] && [ -n "$PROJECT_NAME" ] || { echo "init wrote no POSTGRES_USER, POSTGRES_DB or PROJECT_NAME"; return 1; }
}

# in_proj <cmd...>: run in the project directory with the prompt guards off (stdin is /dev/null).
in_proj() { (cd "$PROJ_DIR" && "$@" </dev/null); }

# nself_t <seconds> <args...>: the current binary in the project, with a timeout.
nself_t() {
  local secs=$1
  shift
  in_proj "$TIMEOUT_BIN" -k 5 "$secs" "$NSELF" "$@"
}

# wait_healthy: `nself status` exit 0, then a Hasura GraphQL answer, each within a deadline.
wait_healthy() {
  local end=$(($(date +%s) + ${DRILL_HEALTH_TIMEOUT:-600})) i=0
  while [ "$(date +%s)" -lt "$end" ]; do
    in_proj "$TIMEOUT_BIN" -k 5 60 "$NSELF" status >/dev/null 2>&1 && break
    sleep 5
  done
  in_proj "$TIMEOUT_BIN" -k 5 60 "$NSELF" status >/dev/null 2>&1 || { echo "nself status did not reach exit 0"; return 1; }
  while [ "$i" -lt 12 ]; do
    graphql_probe "$PROJ_DIR/.env" "${DRILL_GRAPHQL_URL:-http://localhost:8080/v1/graphql}" && return 0
    sleep 5
    i=$((i + 1))
  done
  echo "the GraphQL probe did not answer"
  return 1
}

# sql <statement>: one statement through the Postgres container, tuples only.
sql() { nself_t 60 exec -T "${PROJECT_NAME}_postgres" -- psql -U "$PG_USER" -d "$PG_DB" -At -v ON_ERROR_STOP=1 -c "$1"; }

# seed_rows: a migration creates public.drill_seed (50 rows) and `nself db migrate up` applies it.
seed_rows() {
  local d=$PROJ_DIR/hasura/migrations/default/20260101000000_drill_seed
  mkdir -p "$d"
  cat >"$d/up.sql" <<'SQL'
CREATE TABLE public.drill_seed (id integer PRIMARY KEY, v text NOT NULL);
INSERT INTO public.drill_seed SELECT g, md5(g::text) FROM generate_series(1, 50) AS g;
SQL
  nself_t 120 db migrate up || return 1
}

# row_digest: "<count> <md5 of every row in id order>" of the seeded table. Postgres may still be
# restarting after a restore, so a failed query is retried for a minute.
row_digest() {
  local i=0 out
  while [ "$i" -lt 12 ]; do
    if out=$(sql "SELECT count(*) || ' ' || coalesce(md5(string_agg(t::text, '|' ORDER BY id)), 'none') FROM public.drill_seed AS t"); then
      printf '%s\n' "$out" | tail -n 1
      return 0
    fi
    sleep 5
    i=$((i + 1))
  done
  return 1
}

# cleanup_stack: stop the stack and drop its volumes; safe to call twice.
cleanup_stack() {
  [ -n "${PROJ_DIR:-}" ] && [ -d "$PROJ_DIR" ] || return 0
  (
    cd "$PROJ_DIR" || exit 0
    "$TIMEOUT_BIN" -k 5 180 "${NSELF:-${REF_BIN:-nself}}" stop --volumes </dev/null >>"$EV_OUT/cleanup.log" 2>&1 || true
    docker compose down -v --remove-orphans >>"$EV_OUT/cleanup.log" 2>&1 || true
  )
}
