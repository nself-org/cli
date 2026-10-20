# Sourced by hasura-upgrade-smoke.sh after the isolated fixture is prepared.
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
build_start() {
  nsf build
  if [ "$RC" -ne 0 ]; then fail "$1: nself build exits 0" "rc=$RC $(nsf_tail)"; return 1; fi
  pass "$1: nself build exits 0"
  nsf start
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
