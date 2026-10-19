#!/usr/bin/env bash
# Purpose: prove the P7-LIVE-23 Hasura move (graphql-engine v2.44.0 -> v2.51.0) on a real
#          stack and print one PASS or FAIL line per assertion.
#          NSELF_V15=1 (upgrade):
#            1. a new v1.5 project is pinned to the hasura-v14 ref (v2.44.0) in
#               nself.lock.yaml (the pin edit stands in for `nself update`, P7-LIVE-10)
#               and starts on it
#            2. a fixture migration (tables and rows) is applied with `nself db migrate`;
#               metadata (tracked tables, a relationship, a select permission, an event
#               trigger to a webhook receiver, a remote schema to a GraphQL receiver) is
#               applied with `nself db hasura metadata apply` and exported with
#               `nself db hasura metadata export`
#            3. the functional checks pass on v2.44.0: rows, permission, mutation,
#               event trigger delivery, remote schema call, subscription event
#            4. a verified backup (nself backup create, verify, verify --restore-test)
#            5. the switch to the `hasura` ref (v2.51.0): the same functional checks, the
#               metadata export equals the pre-switch export (files and normalised JSON),
#               `nself db hasura diff` is clean, apply then export round-trips
#            6. rollback, unsupported path: pin back to v2.44.0 WITHOUT restoring the backup;
#               the observed outcome is asserted (a catalog refusal, or a clean start that
#               passes the functional checks, with the catalog versions in the message)
#            7. rollback, supported path: restore the pre-switch backup over the
#               migrated database (plain restore, no db reset), keep the v2.44.0 pin,
#               start cleanly; rows and metadata are the pre-switch ones, functional checks pass
#          NSELF_V15 unset (baseline): steps 1 to 3 on v2.44.0 plus the metadata
#          apply/export round trip; no switch.
# Usage:   NSELF_V15=1 bash scripts/ci/hasura-upgrade-smoke.sh [path-to-nself-binary]
#          env -u NSELF_V15 bash scripts/ci/hasura-upgrade-smoke.sh [path-to-nself-binary]
# Inputs:  optional binary (built with go into the temp dir when absent); docker with the
#          compose plugin and a reachable engine, curl (7.55+), python3 and openssl on PATH (nself
#          build generates the dev certificate with it; go only when no binary is given). Image refs are read from internal/compose/images.lock.json
#          (hasura, hasura-v14, python), never hardcoded. The stack publishes 127.0.0.1:8080
#          (Hasura), :5432, :80 and :443, so those must be free. TMPDIR must be a path the
#          Docker engine can bind-mount. The webhook and GraphQL receivers are one synthetic
#          container (scripts/ci/testdata/hasura-smoke/receiver.py) on the project network.
# Outputs: one line per assertion, "PASS <name>" or "FAIL <name>" (detail on stderr);
#          the last line is "HASURA-SMOKE: PASS" or "HASURA-SMOKE: FAIL". Exit 1 on any
#          FAIL, when nothing passed, or when a tool is missing. There is no SKIP: an
#          assertion that cannot run is a FAIL. Admin secrets are never printed.
# Constraints: bash 3.2 compatible. The project, HOME and every secret file live in one
#          0700 temp dir; the EXIT trap removes the project's containers, volumes and
#          networks (matched by the unique compose project label or exact container name)
#          and then the dir. Only synthetic rows and runtime-generated secrets are used;
#          the admin secret travels through 0600 files, never argv. No real project,
#          server or remote is touched.
set -uo pipefail

cd "$(dirname "$0")/../.." || exit 1
ROOT="$PWD"
REAL_HOME="$HOME"
FIX="$ROOT/scripts/ci/testdata/hasura-smoke"

PASSED=0
FAILED=0
pass() { printf 'PASS %s\n' "$1"; PASSED=$((PASSED + 1)); }
# redact: strip anything that looks like a JWT or a long hex secret from detail lines.
redact() { sed -E 's/eyJ[A-Za-z0-9_.-]+/<jwt>/g; s/[A-Fa-f0-9]{32,}/<hex>/g'; }
fail() {
  printf 'FAIL %s\n' "$1"
  FAILED=$((FAILED + 1))
  if [ -n "${2:-}" ]; then printf '  %s\n' "$2" | redact >&2; fi
}
finish() {
  printf '%s assertions: %s passed, %s failed\n' "$((PASSED + FAILED))" "$PASSED" "$FAILED" >&2
  if [ "$FAILED" -eq 0 ] && [ "$PASSED" -gt 0 ]; then
    echo "HASURA-SMOKE: PASS"
    exit 0
  fi
  echo "HASURA-SMOKE: FAIL"
  exit 1
}
fatal() { fail "$@"; finish; }
info() { printf 'INFO %s\n' "$1"; }
step() { printf '== %s\n' "$1"; }
# check <name> <command...>: PASS when the command exits 0.
check() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then pass "$name"; else fail "$name"; fi
}

UPGRADE=0
V15ENV=""
if [ "${NSELF_V15:-}" = "1" ]; then UPGRADE=1; V15ENV="NSELF_V15=1"; fi

for t in docker curl python3; do
  command -v "$t" >/dev/null 2>&1 || fatal "tool-$t-missing"
done
docker info >/dev/null 2>&1 || fatal "docker-engine-unreachable"
docker compose version >/dev/null 2>&1 || fatal "docker-compose-plugin-missing"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/hasura-smoke.XXXXXX")" || fatal "mktemp-failed"
PROJECT="hasurasmoke$(python3 -c 'import secrets;print(secrets.token_hex(3))')"
PDIR="$WORK/project"
# shellcheck disable=SC2329 # invoked by the EXIT trap
cleanup() {
  local ids
  ids="$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null)"
  # backup verify --restore-test starts ${PROJECT}_pg_restore_test without a compose
  # label, and the receiver is this script's own container: remove exactly those names.
  ids="$ids $(docker ps -aq --filter "name=^${PROJECT}_pg_restore_test\$" 2>/dev/null)"
  ids="$ids $(docker ps -aq --filter "name=^${PROJECT}_receiver\$" 2>/dev/null)"
  # shellcheck disable=SC2086
  if [ -n "${ids// /}" ]; then docker rm -f -v $ids >/dev/null 2>&1; fi
  ids="$(docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null)"
  # shellcheck disable=SC2086
  if [ -n "$ids" ]; then docker volume rm -f $ids >/dev/null 2>&1; fi
  ids="$(docker network ls -q --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null)"
  # shellcheck disable=SC2086
  if [ -n "$ids" ]; then docker network rm $ids >/dev/null 2>&1; fi
  rm -rf "$WORK"
}
trap cleanup EXIT
mkdir -p "$PDIR" "$WORK/home"
chmod 0700 "$WORK"
# Files that can hold secrets are created 0600 here and only ever truncated afterwards.
# (nself itself runs under umask 022: its build plan assumes 0644 files, D-0664.)
(umask 077; : > "$WORK/resp"; : > "$WORK/out"; : > "$WORK/err"; : > "$WORK/curl.err"; : > "$WORK/body"; : > "$WORK/admin.hdr"; : > "$WORK/admin.secret")

# ---- the binary and the image refs ---------------------------------------------------
BIN="${1:-}"
if [ -z "$BIN" ]; then
  command -v go >/dev/null 2>&1 || fatal "tool-go-missing (pass a built nself binary as the first argument)"
  mkdir -p "$WORK/bin"
  BIN="$WORK/bin/nself"
  if CGO_ENABLED=0 go build -mod=vendor -buildvcs=false -o "$BIN" ./cmd/nself 2>"$WORK/build.err"; then
    pass "build the nself binary"
  else
    redact < "$WORK/build.err" >&2
    fatal "build the nself binary"
  fi
fi
case "$BIN" in /*) ;; *) BIN="$ROOT/$BIN" ;; esac
[ -x "$BIN" ] || fatal "binary $BIN is not executable"

# lock_ref <entry>: repository:version@index_digest of an image lock entry.
lock_ref() {
  python3 -c 'import json,sys
for e in json.load(open(sys.argv[1]))["images"]:
    if e["name"] == sys.argv[2]:
        print("%s:%s@%s" % (e["repository"], e["version"], e["index_digest"]))' "$ROOT/internal/compose/images.lock.json" "$1"
}
lock_field() {
  python3 -c 'import json,sys
for e in json.load(open(sys.argv[1]))["images"]:
    if e["name"] == sys.argv[2]:
        print(e[sys.argv[3]])' "$ROOT/internal/compose/images.lock.json" "$1" "$2"
}
REF_V14="$(lock_ref hasura-v14)"
REF_NEW="$(lock_ref hasura)"
REF_PY="$(lock_ref python)"
LEGACY_V14="$(lock_field hasura-v14 legacy_ref)"
VER_V14="$(lock_field hasura-v14 version)"
VER_NEW="$(lock_field hasura version)"
if [ -n "$REF_V14" ] && [ -n "$REF_NEW" ] && [ "$REF_V14" != "$REF_NEW" ] && [ -n "$REF_PY" ]; then
  pass "image lock has distinct hasura-v14 and hasura refs"
else
  fatal "image lock has distinct hasura-v14 and hasura refs"
fi
info "host=$(uname -sm) engine=$(docker version --format '{{.Server.Os}}/{{.Server.Arch}}' 2>/dev/null) binary=$(uname -sm)"
info "hasura-v14=${REF_V14%%@*} hasura=${REF_NEW%%@*} mode=$([ "$UPGRADE" = 1 ] && echo upgrade || echo baseline)"

# ---- helpers -------------------------------------------------------------------------
HASURA_URL="http://127.0.0.1:8080"
RC=0
# nsf <args...>: run nself in the project; stdout $WORK/out, stderr $WORK/err, status $RC.
nsf() {
  local dh=""
  RC=0
  if [ -n "${DOCKER_HOST:-}" ]; then dh="DOCKER_HOST=$DOCKER_HOST"; fi
  # The caller's environment never steers nself at another stack: env -i plus a fixed
  # allow-list (a db reset aimed at someone's own project would be destructive).
  # shellcheck disable=SC2086
  (umask 022; cd "$PDIR" && env -i PATH="$PATH" HOME="$WORK/home" TMPDIR="${TMPDIR:-/tmp}" \
    DOCKER_CONFIG="${DOCKER_CONFIG:-$REAL_HOME/.docker}" $dh $V15ENV NSELF_CMD_LOG_ENABLED=false \
    AI_AUTO_INSTALL=false LANG="${LANG:-C}" "$BIN" "$@" > "$WORK/out" 2> "$WORK/err" < /dev/null) || RC=$?
}
nsf_tail() { { tail -n 6 "$WORK/err"; tail -n 4 "$WORK/out"; } | redact | tr '\n' '|'; }

HTTP=000
# jbody <printf format> <args...>: write the next POST body to a 0600 file.
# shellcheck disable=SC2059 # the caller supplies the format
jbody() { printf "$@" > "$WORK/body"; }
# call <GET|POST> <url> [extra-header]: admin credentials from the 0600 header file; the POST
# body is $WORK/body; sets HTTP and writes the response to $WORK/resp.
call() {
  local m="$1" u="$2" x="${3:-}"
  : > "$WORK/resp"
  set -- -sS -m 30 -o "$WORK/resp" -w '%{http_code}' -X "$m" -H "@$WORK/admin.hdr"
  if [ -n "$x" ]; then set -- "$@" -H "$x"; fi
  if [ "$m" = GET ]; then
    HTTP="$(curl "$@" "$u" 2>"$WORK/curl.err" < /dev/null)" || HTTP=000
  else
    HTTP="$(curl "$@" -H 'Content-Type: application/json' --data-binary "@$WORK/body" "$u" 2>"$WORK/curl.err")" || HTTP=000
  fi
}
# jv <python expr over d>: evaluate against the last response; empty on any error.
jv() {
  python3 -c 'import json,sys
try:
    d = json.load(open(sys.argv[1]))
    v = eval(sys.argv[2])
    print("" if v is None else v)
except Exception:
    print("")' "$WORK/resp" "$1"
}
why() {
  local m
  m="$(jv 'd.get("error") or d.get("message") or d.get("errors")[0]["message"]' | cut -c1-140)"
  printf 'http=%s %s' "$HTTP" "$m"
}
# gql <query> [role]: POST a GraphQL query as admin, or as role with the same secret.
gql() {
  jbody '{"query":%s}' "$(python3 -c 'import json,sys;print(json.dumps(sys.argv[1]))' "$1")"
  if [ -n "${2:-}" ]; then call POST "$HASURA_URL/v1/graphql" "X-Hasura-Role: $2"; else call POST "$HASURA_URL/v1/graphql"; fi
}
dbq() {
  printf '%s\n' "$1" | docker exec -i "${PROJECT}_postgres" psql -U postgres -d "${2:-$PROJECT}" -At -v ON_ERROR_STOP=1 -f - 2>/dev/null
}

# catalog_version: the Hasura metadata catalog version stored in the database.
catalog_version() { dbq 'select version from hdb_catalog.hdb_version limit 1;'; }
compose_hasura_image() {
  awk '/^    hasura:$/{f=1;next} f && /^        image:/{print $2;exit} /^    [a-z]/{f=0}' "$PDIR/docker-compose.yml"
}
lock_hasura_pin() { sed -n 's/^  hasura: //p' "$PDIR/nself.lock.yaml"; }
running_hasura_image() { docker inspect -f '{{.Config.Image}}' "${PROJECT}_hasura" 2>/dev/null; }
running_hasura_version() { call GET "$HASURA_URL/v1/version"; jv 'd["version"]'; }
# set_pin <ref>: stand-in for `nself update` (P7-LIVE-10, the official pin mover): rewrite
# the hasura pin line of nself.lock.yaml.
set_pin() {
  sed "s|^  hasura: .*|  hasura: $1|" "$PDIR/nself.lock.yaml" > "$PDIR/nself.lock.yaml.new" \
    && mv "$PDIR/nself.lock.yaml.new" "$PDIR/nself.lock.yaml"
}
# own_port: 127.0.0.1:8080 is published by this run's Hasura container, so no request can
# land in another stack.
own_port() { docker port "${PROJECT}_hasura" 2>/dev/null | grep -q '127.0.0.1:8080$'; }
# wait_hasura [seconds]: wait for this run's Hasura to answer /healthz.
wait_hasura() {
  local i=0 max=$(( ${1:-240} / 3 ))
  while [ "$i" -lt "$max" ]; do
    if [ "$(curl -s -m 5 -o /dev/null -w '%{http_code}' "$HASURA_URL/healthz" 2>/dev/null)" = 200 ] && own_port; then
      return 0
    fi
    i=$((i + 1))
    sleep 3
  done
  return 1
}
# build_start <label>: nself build then nself start, then wait for health.
# Starts after the switch pass --skip-db-init: a plain start re-applies saved metadata (D-1152) and would
# repair what an upgrade lost before the preservation checks run.
build_start() {
  nsf build
  if [ "$RC" -ne 0 ]; then fail "$1: nself build exits 0" "rc=$RC $(nsf_tail)"; return 1; fi
  pass "$1: nself build exits 0"
  nsf start --skip-db-init
  if [ "$RC" -ne 0 ]; then fail "$1: nself start exits 0" "rc=$RC $(nsf_tail)"; return 1; fi
  pass "$1: nself start exits 0"
  if wait_hasura; then pass "$1: Hasura healthy"; else fail "$1: Hasura healthy" "$(docker ps -a --format '{{.Names}} {{.Status}}' | grep "^$PROJECT" | tr '\n' '|')"; return 1; fi
  return 0
}

# ---- receivers: one synthetic container on the project network ------------------------
start_receiver() {
  local net
  net="$(docker inspect -f '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}' "${PROJECT}_hasura" 2>/dev/null | awk '{print $1}')"
  [ -n "$net" ] || return 1
  docker rm -f -v "${PROJECT}_receiver" >/dev/null 2>&1
  : > "$WORK/rcv.err"
  docker pull -q "$REF_PY" >/dev/null 2>>"$WORK/rcv.err" || return 1
  docker create --name "${PROJECT}_receiver" --network "$net" --network-alias receiver "$REF_PY" python /tmp/receiver.py >/dev/null 2>"$WORK/rcv.err" \
    && docker cp "$FIX/receiver.py" "${PROJECT}_receiver:/tmp/receiver.py" >/dev/null 2>>"$WORK/rcv.err" \
    && docker start "${PROJECT}_receiver" >/dev/null 2>>"$WORK/rcv.err" || return 1
  local i=0
  while [ "$i" -lt 20 ]; do
    if docker exec "${PROJECT}_receiver" python -c 'import urllib.request as u;u.urlopen(u.Request("http://127.0.0.1:9101/graphql",b"{\"query\":\"{ smoke_ping }\"}"),timeout=2)' >/dev/null 2>&1; then return 0; fi
    i=$((i + 1))
    sleep 1
  done
  return 1
}
receiver_events() { docker exec "${PROJECT}_receiver" cat /tmp/events.jsonl 2>/dev/null; }

# ---- metadata capture and compare -------------------------------------------------------
# export_json <file>: export_metadata (v2) through the Hasura API, normalised: keys sorted,
# resource_version dropped (it counts every metadata write).
export_json() {
  jbody '{"type":"export_metadata","version":2,"args":{}}'
  call POST "$HASURA_URL/v1/metadata"
  [ "$HTTP" = 200 ] || return 1
  python3 -c 'import json,sys
d = json.load(open(sys.argv[1]))
m = d["metadata"] if "metadata" in d else d
json.dump(m, open(sys.argv[2], "w"), sort_keys=True, indent=1)' "$WORK/resp" "$1"
}
# normalise_tree <dir> <out>: concatenate the exported files in name order. Files are the
# CLI's own sorted YAML, so a byte compare is the normalised compare.
normalise_tree() { (cd "$1" && export LC_ALL=C && for f in *; do printf '### %s\n' "$f"; cat "$f"; done) > "$2"; }
# export_tree <dir>: nself db hasura metadata export, copied to <dir> and flattened to <dir>.txt.
export_tree() {
  nsf db hasura metadata export
  [ "$RC" -eq 0 ] || return 1
  rm -rf "$1"
  cp -R "$PDIR/hasura/metadata" "$1" && normalise_tree "$1" "$1.txt"
}

# ---- functional assertions -------------------------------------------------------------
LAST_ID=""
# functional <label> <min-rows>: the checks that must hold on whatever Hasura runs.
functional() {
  local label="$1" minrows="$2" n names ver
  ver="$(running_hasura_version)"
  gql '{ smoke_items_aggregate { aggregate { count } } }'
  n="$(jv 'd["data"]["smoke_items_aggregate"]["aggregate"]["count"]')"
  if [ "$HTTP" = 200 ] && [ -n "$n" ] && [ "$n" -ge "$minrows" ]; then pass "$label: existing rows readable ($n rows, at least $minrows)"; else fail "$label: existing rows readable" "$(why) count='$n' want>=$minrows"; fi
  gql '{ smoke_items(order_by: {id: asc}, limit: 3) { name tags { tag } } }'
  names="$(jv '",".join("%s:%s" % (r["name"], r["tags"][0]["tag"]) for r in d["data"]["smoke_items"])')"
  if [ "$names" = "alpha:first,beta:second,gamma:third" ]; then pass "$label: fixture rows and the tags relationship are intact"; else fail "$label: fixture rows and the tags relationship are intact" "got '$names' $(why)"; fi
  gql '{ smoke_items(limit: 1) { id name } }' user
  if [ "$HTTP" = 200 ] && [ -n "$(jv 'd["data"]["smoke_items"][0]["name"]')" ]; then pass "$label: role user reads the permitted columns"; else fail "$label: role user reads the permitted columns" "$(why)"; fi
  gql '{ smoke_items(limit: 1) { secret_note } }' user
  if [ "$(jv '"secret_note" in d["errors"][0]["message"]')" = True ]; then pass "$label: role user cannot read secret_note (permission holds)"; else fail "$label: role user cannot read secret_note (permission holds)" "$(why)"; fi
  gql "mutation { insert_smoke_items_one(object: {name: \"${label}-mutation\"}) { id name } }"
  LAST_ID="$(jv 'd["data"]["insert_smoke_items_one"]["id"]')"
  if [ "$HTTP" = 200 ] && [ -n "$LAST_ID" ]; then pass "$label: mutation inserts a row (id $LAST_ID)"; else fail "$label: mutation inserts a row" "$(why)"; fi
  local i=0 hit=""
  while [ "$i" -lt 20 ]; do
    hit="$(receiver_events | grep -c "\"${label}-mutation\"")"
    if [ "$hit" -ge 1 ] 2>/dev/null; then break; fi
    i=$((i + 1))
    sleep 2
  done
  if [ "${hit:-0}" -ge 1 ] 2>/dev/null; then pass "$label: event trigger delivered the insert to the webhook"; else fail "$label: event trigger delivered the insert to the webhook" "receiver saw $(receiver_events | wc -l | tr -d ' ') events, none for ${label}-mutation"; fi
  gql '{ smoke_ping }'
  if [ "$(jv 'd["data"]["smoke_ping"]')" = pong ]; then pass "$label: remote schema call answers pong"; else fail "$label: remote schema call answers pong" "$(why)"; fi
  if python3 "$FIX/ws_sub.py" 127.0.0.1 8080 "$WORK/admin.secret" "${label}-subscription" > "$WORK/ws.out" 2> "$WORK/ws.err"; then
    pass "$label: subscription pushes a new row ($(head -n1 "$WORK/ws.out"))"
  else
    fail "$label: subscription pushes a new row" "$(redact < "$WORK/ws.err" | head -n 3 | tr '\n' '|')"
  fi
  info "$label: hasura answers /v1/version $ver"
}

# ---- step 1: new project on the hasura-v14 ref ----------------------------------------
step "1 new project on hasura-v14 ($VER_V14)"
# The stack publishes fixed loopback ports: refuse to start when any is taken.
if ! python3 -c 'import socket,sys
for p in (80, 443, 5432, 8080):
    s = socket.socket()
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    try:
        s.bind(("127.0.0.1", p))
    except OSError:
        sys.exit("port %d is in use" % p)
    finally:
        s.close()' 2>"$WORK/ports.err"; then
  fatal "ports 80, 443, 5432 and 8080 are free on 127.0.0.1" "$(cat "$WORK/ports.err")"
fi
pass "ports 80, 443, 5432 and 8080 are free on 127.0.0.1"
nsf init --non-interactive --name "$PROJECT" --domain local.nself.org --no-pgvector
[ "$RC" -eq 0 ] || fatal "nself init --non-interactive exits 0" "rc=$RC $(nsf_tail)"
pass "nself init --non-interactive exits 0"
if [ "$(grep -c "^PROJECT_NAME=$PROJECT\$" "$PDIR/.env")" -eq 1 ]; then
  pass "project .env names PROJECT_NAME=$PROJECT"
else
  fatal "project .env names PROJECT_NAME=$PROJECT" "refusing to continue: the project is not the one this run created"
fi
check "project .env carries the init default HASURA_VERSION=$VER_V14" grep -q "^HASURA_VERSION=$VER_V14\$" "$PDIR/.env"
(umask 077; sed -n 's/^HASURA_GRAPHQL_ADMIN_SECRET=//p' "$PDIR/.env" | head -n1 | tr -d '\r\n"' > "$WORK/admin.secret")
if [ -s "$WORK/admin.secret" ]; then
  (umask 077; printf 'X-Hasura-Admin-Secret: %s\n' "$(cat "$WORK/admin.secret")" > "$WORK/admin.hdr")
  pass "admin secret read from the project .env into a 0600 file"
else
  fatal "admin secret read from the project .env into a 0600 file"
fi
nsf build
[ "$RC" -eq 0 ] || fatal "nself build exits 0" "rc=$RC $(nsf_tail)"
pass "nself build exits 0"
if [ "$UPGRADE" = 1 ]; then
  # v1.5 pins the new default on first build; this project is an existing v2.44.0 project, so
  # the pin goes back to the hasura-v14 ref before anything starts.
  if [ "$(lock_hasura_pin)" = "$REF_NEW" ]; then pass "v1.5 first build pins hasura to the new ref"; else fail "v1.5 first build pins hasura to the new ref" "got $(lock_hasura_pin)"; fi
  set_pin "$REF_V14"
  nsf build
  [ "$RC" -eq 0 ] || fatal "nself build after the pin edit exits 0" "rc=$RC $(nsf_tail)"
  pass "nself build after the pin edit exits 0"
  if [ "$(compose_hasura_image)" = "$REF_V14" ]; then pass "compose hasura image is the hasura-v14 ref (the pin wins over the v1.5 default)"; else fail "compose hasura image is the hasura-v14 ref" "got $(compose_hasura_image)"; fi
  if [ "$(lock_hasura_pin)" = "$REF_V14" ]; then pass "nself.lock.yaml pins hasura to the hasura-v14 ref"; else fail "nself.lock.yaml pins hasura to the hasura-v14 ref" "got $(lock_hasura_pin)"; fi
  EXPECT_V14="$REF_V14"
else
  if [ "$(compose_hasura_image)" = "$LEGACY_V14" ]; then pass "v1.4 compose hasura image is $LEGACY_V14"; else fail "v1.4 compose hasura image is $LEGACY_V14" "got $(compose_hasura_image)"; fi
  EXPECT_V14="$LEGACY_V14"
fi
nsf start
[ "$RC" -eq 0 ] || fatal "nself start exits 0" "rc=$RC $(nsf_tail)"
pass "nself start exits 0"
wait_hasura || fatal "Hasura healthy on hasura-v14"
pass "Hasura healthy on hasura-v14"
if [ "$(running_hasura_image)" = "$EXPECT_V14" ]; then pass "running Hasura container uses the hasura-v14 ref"; else fail "running Hasura container uses the hasura-v14 ref" "got $(running_hasura_image)"; fi
if [ "$(running_hasura_version)" = "$VER_V14" ]; then pass "Hasura reports version $VER_V14"; else fail "Hasura reports version $VER_V14" "got $(running_hasura_version)"; fi

# ---- step 2: data and metadata on hasura-v14 ------------------------------------------
step "2 migration and metadata on hasura-v14"
mkdir -p "$PDIR/hasura/migrations/default"
cp "$FIX/migrations/"*.sql "$PDIR/hasura/migrations/default/"
cp "$FIX/metadata.json" "$PDIR/hasura/metadata.json"
nsf db migrate up
if [ "$RC" -eq 0 ] && grep -q 'Applied 1 migration' "$WORK/out"; then pass "nself db migrate applies the fixture migration"; else fail "nself db migrate applies the fixture migration" "rc=$RC $(nsf_tail)"; fi
if [ "$(dbq 'select count(*) from public.smoke_items;')" = 3 ]; then pass "fixture rows are in postgres (3 smoke_items)"; else fail "fixture rows are in postgres (3 smoke_items)"; fi
if start_receiver; then pass "receiver container (webhook :9100, remote schema :9101) is up on the project network"; else fatal "receiver container is up on the project network" "$(redact < "$WORK/rcv.err" | tail -n 3 | tr '\n' '|')"; fi
nsf db hasura metadata apply
if [ "$RC" -eq 0 ]; then pass "nself db hasura metadata apply exits 0"; else fatal "nself db hasura metadata apply exits 0" "rc=$RC $(nsf_tail)"; fi
if export_tree "$WORK/meta-pre" && export_json "$WORK/meta-pre.json"; then pass "metadata exported before any switch (tree and API JSON)"; else fatal "metadata exported before any switch" "rc=$RC $(nsf_tail)"; fi
check "exported metadata tracks both tables, the event trigger and the remote schema" python3 -c 'import json,sys
m = json.load(open(sys.argv[1]))
t = {x["table"]["name"]: x for x in m["sources"][0]["tables"]}
assert set(t) == {"smoke_items", "smoke_tags"}
assert t["smoke_items"]["event_triggers"][0]["name"] == "smoke_item_events"
assert t["smoke_items"]["select_permissions"][0]["role"] == "user"
assert m["remote_schemas"][0]["name"] == "smoke_remote"' "$WORK/meta-pre.json"
nsf db hasura metadata apply
if [ "$RC" -eq 0 ] && export_tree "$WORK/meta-rt" && cmp -s "$WORK/meta-pre.txt" "$WORK/meta-rt.txt"; then
  pass "round trip on hasura-v14: apply the exported tree, export again, identical"
else
  fail "round trip on hasura-v14: apply the exported tree, export again, identical" "rc=$RC"
fi
CAT_V14="$(catalog_version)"
info "hasura catalog version on $VER_V14: $CAT_V14"
functional v14 3
export_json "$WORK/meta-v14-after.json" || fail "metadata export through the API"
if cmp -s "$WORK/meta-pre.json" "$WORK/meta-v14-after.json"; then pass "v14: API metadata is unchanged by the functional checks"; else fail "v14: API metadata is unchanged by the functional checks"; fi

if [ "$UPGRADE" != 1 ]; then
  finish
fi

# ---- step 3: verified backup -----------------------------------------------------------
step "3 verified backup"
NROWS_BACKUP="$(dbq 'select count(*) from public.smoke_items;')"
nsf backup create
if [ "$RC" -eq 0 ]; then pass "nself backup create exits 0"; else fail "nself backup create exits 0" "rc=$RC $(nsf_tail)"; fi
nsf backup list --format json
BID="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["data"][0]["id"])' "$WORK/out" 2>/dev/null)"
if [ -n "$BID" ] && [ -f "$PDIR/backups/$BID.dump" ]; then
  pass "backup recorded: id $BID, path backups/$BID.dump ($NROWS_BACKUP smoke_items rows)"
else
  fatal "backup recorded (id and file)" "id='$BID' $(find "$PDIR/backups" -type f 2>&1 | head -3 | tr '\n' ' ')"
fi
nsf backup verify "$BID"
if [ "$RC" -eq 0 ]; then pass "nself backup verify exits 0"; else fail "nself backup verify exits 0" "rc=$RC $(nsf_tail)"; fi
nsf backup verify "$BID" --restore-test
if [ "$RC" -eq 0 ] && grep -q 'restore-test' "$WORK/out"; then pass "nself backup verify --restore-test exits 0"; else fail "nself backup verify --restore-test exits 0" "rc=$RC $(nsf_tail)"; fi

# ---- step 4: switch to the hasura ref --------------------------------------------------
step "4 switch to the hasura ref ($VER_NEW)"
set_pin "$REF_NEW"
# nself update is the official pin mover (P7-LIVE-10): the pin edit above is its stand-in.
nsf build
if [ "$RC" -eq 0 ]; then pass "switch: nself build exits 0"; else fail "switch: nself build exits 0" "rc=$RC $(nsf_tail)"; fi
if [ "$(compose_hasura_image)" = "$REF_NEW" ]; then pass "switch: compose hasura image is the hasura ref"; else fail "switch: compose hasura image is the hasura ref" "got $(compose_hasura_image)"; fi
if [ "$(lock_hasura_pin)" = "$REF_NEW" ]; then pass "switch: nself.lock.yaml pins the hasura ref"; else fail "switch: nself.lock.yaml pins the hasura ref" "got $(lock_hasura_pin)"; fi
nsf start --skip-db-init
if [ "$RC" -eq 0 ]; then pass "switch: nself start exits 0"; else fail "switch: nself start exits 0" "rc=$RC $(nsf_tail)"; fi
if wait_hasura; then pass "switch: Hasura healthy"; else fail "switch: Hasura healthy" "$(docker logs --tail 5 "${PROJECT}_hasura" 2>&1 | redact | tr '\n' '|')"; fi
if [ "$(running_hasura_image)" = "$REF_NEW" ]; then pass "switch: running Hasura container uses the hasura ref"; else fail "switch: running Hasura container uses the hasura ref" "got $(running_hasura_image)"; fi
if [ "$(running_hasura_version)" = "$VER_NEW" ]; then pass "switch: Hasura reports version $VER_NEW"; else fail "switch: Hasura reports version $VER_NEW" "got $(running_hasura_version)"; fi

# ---- step 5: after the switch ----------------------------------------------------------
step "5 after the switch"
if export_json "$WORK/meta-post.json" && cmp -s "$WORK/meta-pre.json" "$WORK/meta-post.json"; then
  pass "metadata export (API JSON, normalised) equals the pre-switch export"
else
  fail "metadata export (API JSON, normalised) equals the pre-switch export" "$(diff "$WORK/meta-pre.json" "$WORK/meta-post.json" 2>&1 | head -n 8 | tr '\n' '|')"
fi
if export_tree "$WORK/meta-post" && cmp -s "$WORK/meta-pre.txt" "$WORK/meta-post.txt"; then
  pass "nself db hasura metadata export tree equals the pre-switch export"
else
  fail "nself db hasura metadata export tree equals the pre-switch export" "$(diff "$WORK/meta-pre.txt" "$WORK/meta-post.txt" 2>&1 | head -n 8 | tr '\n' '|')"
fi
nsf db hasura diff
if [ "$RC" -eq 0 ]; then pass "nself db hasura diff exits 0 (live metadata equals the on-disk export)"; else fail "nself db hasura diff exits 0" "rc=$RC $(nsf_tail)"; fi
nsf db hasura metadata apply
if [ "$RC" -eq 0 ] && export_tree "$WORK/meta-rt2" && cmp -s "$WORK/meta-pre.txt" "$WORK/meta-rt2.txt"; then
  pass "v2.51 round trip: apply the exported tree, export again, identical to the pre-switch export"
else
  fail "v2.51 round trip: apply the exported tree, export again, identical to the pre-switch export" "rc=$RC $(nsf_tail)"
fi
functional v251 "$NROWS_BACKUP"
NROWS_AFTER="$(dbq 'select count(*) from public.smoke_items;')"
CAT_NEW="$(catalog_version)"
info "hasura catalog version on $VER_NEW: $CAT_NEW (on $VER_V14 it was $CAT_V14)"

# ---- step 6: rollback, unsupported (pin back, no restore) -----------------------------
step "6 pin back to hasura-v14 without restoring"
set_pin "$REF_V14"
# nself update is the official pin mover (P7-LIVE-10): the pin edit above is its stand-in.
# The CLI does not guard a pin-back: the outcome is whatever the older engine does with the
# catalog the newer one left, and it is asserted as observed. Hasura is either refused with a
# catalog message in its log (PASS), or it starts and passes every functional check (PASS,
# with the catalog versions in the message); anything else is a FAIL.
nsf build
if [ "$RC" -ne 0 ]; then
  fail "pin-back-without-restore-observed" "nself build failed, rc=$RC $(nsf_tail)"
else
  nsf start --skip-db-init
  START_RC="$RC"
  if wait_hasura 90 && [ "$(running_hasura_version)" = "$VER_V14" ] && [ "$(running_hasura_image)" = "$REF_V14" ]; then
    CAT_BACK="$(catalog_version)"
    pass "pin-back-without-restore-observed: Hasura $VER_V14 starts on the database $VER_NEW migrated (start rc=$START_RC; catalog version $CAT_V14 on $VER_V14, $CAT_NEW on $VER_NEW, $CAT_BACK after the pin-back)"
    functional pinback "$NROWS_AFTER"
  else
    WHY="$(docker logs --tail 40 "${PROJECT}_hasura" 2>&1 | grep -i -m1 -o 'catalog[^"]*\(version\|newer\|downgrade\)[^"]*\|[^"]*downgrade[^"]*' | cut -c1-160)"
    STATE="$(docker inspect -f '{{.State.Status}} restarts={{.RestartCount}}' "${PROJECT}_hasura" 2>/dev/null)"
    if [ -n "$WHY" ]; then
      pass "pin-back-without-restore-observed: Hasura $VER_V14 refuses the migrated database ($STATE; start rc=$START_RC; log: $WHY)"
    else
      fail "pin-back-without-restore-observed" "Hasura is not healthy ($STATE; start rc=$START_RC) but the log shows no catalog refusal: $(docker logs --tail 5 "${PROJECT}_hasura" 2>&1 | redact | cut -c1-160 | tr '\n' '|')"
    fi
  fi
fi

# ---- step 7: rollback, supported (restore the pre-switch backup) ---------------------
step "7 restore the pre-switch backup, keep hasura-v14"
if [ "$(lock_hasura_pin)" = "$REF_V14" ]; then pass "restore: lock still pins the hasura-v14 ref"; else fail "restore: lock still pins the hasura-v14 ref" "got $(lock_hasura_pin)"; fi
# Rows right before the restore: the pin-back step inserted more after NROWS_AFTER was read.
NROWS_BEFORE_RESTORE="$(dbq 'select count(*) from public.smoke_items;')"
# Stop only Hasura (postgres must stay up for the restore).
nsf stop hasura
if [ "$RC" -eq 0 ]; then pass "restore: nself stop hasura exits 0"; else fail "restore: nself stop hasura exits 0" "rc=$RC $(nsf_tail)"; fi
nsf backup restore "$BID" --only pg --yes
if [ "$RC" -eq 0 ]; then
  pass "restore: nself backup restore --only pg --yes exits 0 on the migrated database"
else
  fail "restore: nself backup restore --only pg --yes exits 0 on the migrated database" "rc=$RC $(nsf_tail)"
fi
build_start "restore" || true
if [ "$(compose_hasura_image)" = "$REF_V14" ] && [ "$(running_hasura_image)" = "$REF_V14" ]; then
  pass "restore: compose and container run the hasura-v14 ref"
else
  fail "restore: compose and container run the hasura-v14 ref" "compose=$(compose_hasura_image) running=$(running_hasura_image)"
fi
if [ "$(running_hasura_version)" = "$VER_V14" ]; then pass "restore: Hasura reports version $VER_V14"; else fail "restore: Hasura reports version $VER_V14" "got $(running_hasura_version)"; fi
if [ "$(dbq 'select count(*) from public.smoke_items;')" = "$NROWS_BACKUP" ]; then pass "restore: smoke_items has the $NROWS_BACKUP rows of the backup (the $((NROWS_BEFORE_RESTORE - NROWS_BACKUP)) rows added since are gone)"; else fail "restore: smoke_items has the $NROWS_BACKUP rows of the backup" "got $(dbq 'select count(*) from public.smoke_items;')"; fi
if export_json "$WORK/meta-restored.json" && cmp -s "$WORK/meta-pre.json" "$WORK/meta-restored.json"; then
  pass "restore: metadata export equals the pre-switch export"
else
  fail "restore: metadata export equals the pre-switch export" "$(diff "$WORK/meta-pre.json" "$WORK/meta-restored.json" 2>&1 | head -n 8 | tr '\n' '|')"
fi
functional restored "$NROWS_BACKUP"

finish
