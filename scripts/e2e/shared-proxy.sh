#!/usr/bin/env bash
# Linux e2e for the shared host proxy runtime (P7-ADOPT-21, EPIC ADOPT D2/D16/D18).
#
# Owner A (NGINX_MODE=shared) at $W/a, fronted B and C nested under it
# (NGINX_FRONTED_BY=a). Proves: two projects behind one nginx, isolation between
# fronted projects, stop/start/restart/reset/uninstall ordering, owner restart
# re-attach, E506 quarantine (own conf and a neighbour's conf) and the ACME dry run.
#
# Usage: shared-proxy.sh        (needs docker, curl, jq, openssl, python3, go)
# Evidence (logs, docker state on failure) goes to $E2E_EVIDENCE.
set -euo pipefail
export LC_ALL=C
# Keep every install/telemetry path away from the live ping service.
export NSELF_PING_URL=http://127.0.0.1:9
[[ $(uname -s) == Linux ]] || echo 'PROVISIONAL (Constitution §10.3): Linux evidence is required'
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
for tool in docker curl jq openssl python3 go; do
  command -v "$tool" >/dev/null || { echo "FAIL: required tool missing: $tool" >&2; exit 1; }
done
docker compose version >/dev/null || { echo 'FAIL: Docker Compose is unavailable' >&2; exit 1; }

W=$(mktemp -d "${E2E_TMP:-${RUNNER_TEMP:-/tmp}}/shared-proxy.XXXXXX")
EVIDENCE=${E2E_EVIDENCE:-$W/evidence}
mkdir -p "$EVIDENCE"
OWNER=e2e-adopt-a PB=e2e-adopt-b PC=e2e-adopt-c
# Host-published ports must differ per project; the proxy ports (80/443) belong to A alone.
PORTS_B=(AUTH_PORT=4001 HASURA_PORT=8081 POSTGRES_EXPOSE_PORT=false POSTGRES_PORT=5433)
PORTS_C=(AUTH_PORT=4002 HASURA_PORT=8082 POSTGRES_EXPOSE_PORT=false POSTGRES_PORT=5434)
A=$W/a B=$W/a/b C=$W/a/c
# Optional: run the curl/openssl probes elsewhere (a Mac with Colima cannot bind 80/443; use E2E_HOST_EXEC='colima ssh --').
read -r -a HOSTX <<<"${E2E_HOST_EXEC:-}"
NSELF_BIN=$W/nself
PASS=0
step() { printf '\n== %s\n' "$*"; }
ok() { PASS=$((PASS + 1)); printf 'ok   %s\n' "$*"; }
die() { printf 'FAIL %s\n' "$*" >&2; exit 1; }

nself() { local dir=$1; shift; (cd "$dir" && "$NSELF_BIN" "$@"); }
compose() {
  local dir=$1 path
  shift
  local args=(docker compose --project-directory "$dir")
  while IFS= read -r path; do [[ -z $path || ! -f $path ]] || args+=(-f "$path"); done < "$dir/.nself/compose-files.txt"
  if [[ -f $dir/.nself/compose-env-files.txt ]]; then
    while IFS= read -r path; do [[ -z $path || ! -f $path ]] || args+=(--env-file "$path"); done < "$dir/.nself/compose-env-files.txt"
  else
    args+=(--env-file "$dir/.env")
    [[ ! -f $dir/.nself/compose.env ]] || args+=(--env-file "$dir/.nself/compose.env")
  fi
  "${args[@]}" "$@"
}

diagnostics() (
  set +e
  docker ps -a >"$EVIDENCE/docker-ps.txt" 2>&1
  docker network ls >"$EVIDENCE/docker-networks.txt" 2>&1
  for p in $OWNER $PB $PC; do
    docker network inspect "${p}_network" >"$EVIDENCE/network-$p.json" 2>&1
  done
  docker logs "${OWNER}_nginx" >"$EVIDENCE/nginx-a.log" 2>&1
  [[ ! -d $A/nginx/shared ]] || find "$A/nginx" -maxdepth 4 | sort >"$EVIDENCE/owner-nginx-tree.txt"
  cp "$W"/*.log "$EVIDENCE"/ 2>/dev/null
  return 0
)
cleanup() {
  local rc=$?
  trap - EXIT
  [[ $rc == 0 ]] || diagnostics
  for dir in "$C" "$B" "$A"; do
    [[ -f $dir/.nself/compose-files.txt ]] || continue
    compose "$dir" down -v --remove-orphans >/dev/null 2>&1 || true
  done
  for p in $PC $PB $OWNER; do
    docker ps -aq --filter "label=com.docker.compose.project=$p" | xargs -r docker rm -f >/dev/null 2>&1 || true
    docker network rm "${p}_network" >/dev/null 2>&1 || true
  done
  [[ -n ${E2E_KEEP:-} ]] || rm -rf "$W/a" 2>/dev/null || sudo rm -rf "$W/a" 2>/dev/null || true
  [[ $rc != 0 ]] || printf '\nshared-proxy e2e: %s checks passed\n' "$PASS"
  exit "$rc"
}
trap cleanup EXIT

write_env() { # dir project base [extra...]
  local dir=$1 project=$2 base=$3
  shift 3
  mkdir -p "$dir"
  {
    printf 'ENV=dev\nPROJECT_NAME=%s\nBASE_DOMAIN=%s\nSSL_MODE=local\nHASURA_ROUTE=api\nAUTH_ENABLED=true\n' "$project" "$base"
    printf 'MINIO_ENABLED=false\nMINIO_ROOT_USER=fixture-fake-admin\nMINIO_ROOT_PASSWORD=fixture-fake-9u8i7o6p5a4s3d2f\n'
    printf '%s\n' "$@"
  } >"$dir/.env"
  chmod 600 "$dir/.env"
}
pin_credentials() { # dir: build does not persist generated credentials
  python3 - "$1" <<'PY'
import os, pathlib, sys
project = pathlib.Path(sys.argv[1])
target = project / '.env.secrets'
existing = target.read_text() if target.exists() else ''
keys = {'POSTGRES_PASSWORD', 'HASURA_GRAPHQL_ADMIN_SECRET'}
present = {line.split('=', 1)[0] for line in existing.splitlines() if '=' in line}
generated = {}
for line in (project / '.nself/compose.env').read_text().splitlines():
    key = line.split('=', 1)[0]
    if key in keys:
        generated[key] = line
if generated.keys() != keys:
    raise SystemExit('FAIL: generated fixture credentials are incomplete')
with target.open('a') as output:
    os.chmod(target, 0o600)
    if existing and not existing.endswith('\n'):
        output.write('\n')
    for key in sorted(keys - present):
        output.write(generated[key] + '\n')
PY
}

# serves HOST: 200 on /healthz through the owner nginx, printing the status.
status_of() { ${HOSTX[@]+"${HOSTX[@]}"} curl -sk --noproxy '*' --max-time 10 --resolve "$1:443:127.0.0.1" -o /dev/null -w '%{http_code}' "https://$1/healthz" || true; }
serves() {
  local host=$1 code
  for _ in $(seq 1 30); do
    code=$(status_of "$host")
    [[ $code == 200 ]] && return 0
    sleep 1
  done
  die "$host answered $code, want 200"
}
attached() { docker inspect -f '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}' "${OWNER}_nginx" | tr ' ' '\n' | grep -qx "${1}_network"; }
assert_attached() { attached "$1" || die "${OWNER}_nginx is not attached to ${1}_network"; }
assert_detached() { ! attached "$1" || die "${OWNER}_nginx is still attached to ${1}_network"; }
endpoint_id() { docker inspect -f "{{(index .NetworkSettings.Networks \"${1}_network\").EndpointID}}" "${OWNER}_nginx"; }
container_exists() { docker ps -a --format '{{.Names}}' | grep -qx "$1"; }
mode_of() { stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1"; }

step 'build the CLI (linux)'
CGO_ENABLED=0 go build -mod=vendor -o "$NSELF_BIN" "$ROOT/cmd/nself"

step 'project files'
write_env "$A" $OWNER a.local.nself.org NGINX_MODE=shared
nself "$A" build --yes --no-monorepo >"$W/build-a.log" 2>&1 || { cat "$W/build-a.log"; die 'owner build failed'; }
pin_credentials "$A"
write_env "$B" $PB b.local.nself.org NGINX_FRONTED_BY=a "${PORTS_B[@]}"
write_env "$C" $PC c.local.nself.org NGINX_FRONTED_BY=a "${PORTS_C[@]}"
for p in "$B" "$C"; do
  nself "$p" build --yes >"$W/build-$(basename "$p").log" 2>&1 || { cat "$W/build-$(basename "$p").log"; die "build failed in $p"; }
  pin_credentials "$p"
done
for f in $PB $PC; do [[ -f $A/nginx/shared/$f.json ]] || die "registration $f.json missing"; done
ok 'owner and two fronted projects built; both registered'

step 'two projects behind one nginx'
nself "$A" start --timeout 240 >"$W/start-a.log" 2>&1 || { cat "$W/start-a.log"; die 'owner start failed'; }
for p in "$B" "$C"; do
  nself "$p" start --timeout 240 >"$W/start-$(basename "$p").log" 2>&1 || { cat "$W/start-$(basename "$p").log"; die "start failed in $p"; }
done
serves api.a.local.nself.org
serves api.b.local.nself.org
serves api.c.local.nself.org
ok 'api.a, api.b and api.c answer 200 through the owner nginx'
! container_exists ${PB}_nginx || die "${PB}_nginx exists"
! container_exists ${PC}_nginx || die "${PC}_nginx exists"
if docker ps --filter "name=${PB}_" --format '{{.Ports}}' | grep -Eq ':(80|443)->'; then die "a $PB container binds 80/443"; fi
ok 'fronted projects run no nginx and bind neither 80 nor 443'
san=$(${HOSTX[@]+"${HOSTX[@]}"} sh -c 'openssl s_client -connect 127.0.0.1:443 -servername api.b.local.nself.org </dev/null 2>/dev/null | openssl x509 -noout -ext subjectAltName')
grep -q 'api.b.local.nself.org' <<<"$san" || die "B is not served with its own certificate: $san"
ok "api.b is served with B's certificate"

step 'isolation'
if docker exec ${PB}_hasura getent hosts ${PC}_hasura >/dev/null 2>&1; then die "B can resolve ${PC}_hasura"; fi
foreign=$(docker network inspect ${PB}_network --format '{{range .Containers}}{{.Name}}{{"\n"}}{{end}}' | grep -v "^${PB}_" | sed '/^$/d')
[[ $foreign == "${OWNER}_nginx" ]] || die "foreign containers on ${PB}_network: $foreign"
ok "B cannot reach C; ${OWNER}_nginx is the only foreign container on ${PB}_network"

step 'stop and start in a fronted project'
id0=$(endpoint_id $PB)
nself "$B" start --timeout 240 >"$W/start-b2.log" 2>&1 || die 'second start failed'
[[ $(endpoint_id $PB) == "$id0" ]] || die 'second start reconnected the network (not idempotent)'
ok 'second start in B makes no new connection'
nself "$B" stop >"$W/stop-b.log" 2>&1 || { cat "$W/stop-b.log"; die 'nself stop in B failed'; }
serves api.a.local.nself.org
! docker network inspect ${PB}_network >/dev/null 2>&1 || die "${PB}_network still exists after stop"
ok 'stop in B exits 0, A still answers, B network is gone'
nself "$B" start --timeout 240 >"$W/start-b3.log" 2>&1 || die 'start after stop failed'
assert_attached $PB
serves api.b.local.nself.org
ok 'start in B reattaches'

step 'restart, start --fresh and reset in a fronted project'
nself "$B" restart >"$W/restart-b.log" 2>&1 || { cat "$W/restart-b.log"; die 'restart in B failed'; }
nself "$B" start --timeout 240 >/dev/null 2>&1 || die 'start after restart failed'
assert_attached $PB; serves api.b.local.nself.org
nself "$B" start --fresh --timeout 240 >"$W/fresh-b.log" 2>&1 || { cat "$W/fresh-b.log"; die 'start --fresh in B failed'; }
assert_attached $PB; serves api.b.local.nself.org
nself "$B" reset --yes --no-monorepo >"$W/reset-b.log" 2>&1 || { cat "$W/reset-b.log"; die 'reset --yes in B failed'; }
serves api.a.local.nself.org
ok 'restart, start --fresh and reset --yes in B exit 0 while A nginx is attached'
# reset removes generated files; rebuild and start B again for the next legs.
write_env "$B" $PB b.local.nself.org NGINX_FRONTED_BY=a "${PORTS_B[@]}"
nself "$B" build --yes >"$W/build-b2.log" 2>&1 || { cat "$W/build-b2.log"; die 'rebuild of B failed'; }
pin_credentials "$B"
nself "$B" start --timeout 240 >"$W/start-b4.log" 2>&1 || die 'start after reset failed'
serves api.b.local.nself.org

step 'owner nginx restart re-attaches registered, running projects'
nself "$C" stop >/dev/null 2>&1 || die 'stop C failed'
nself "$A" restart nginx >"$W/restart-nginx.log" 2>&1 || { cat "$W/restart-nginx.log"; die 'restart nginx in A failed'; }
nself "$A" start --timeout 240 >"$W/start-a2.log" 2>&1 || { cat "$W/start-a2.log"; die 'start in A after nginx restart failed'; }
assert_attached $PB
serves api.b.local.nself.org; serves api.a.local.nself.org
warns=$(grep -c "$PC" "$W/start-a2.log" || true)
[[ $warns -ge 1 ]] || die "no warning for the stopped project $PC"
ok 'A start reattaches B; stopped C gives a warning; A starts'
nself "$C" start --timeout 240 >/dev/null 2>&1 || die 'start C failed'
serves api.c.local.nself.org

step 'a failing nginx -t (E506) on the project own conf'
nself "$B" stop >/dev/null 2>&1 || die 'stop B failed'
bconf=$(jq -r '.files[] | select(startswith("nginx/sites/"))' "$A/nginx/shared/$PB.json" | head -1)
[[ -n $bconf ]] || die 'B registration lists no site file'
cp "$A/$bconf" "$W/b-conf.good"
printf 'this is not valid nginx;\n' >> "$A/$bconf"
set +e; nself "$B" start --timeout 240 >"$W/start-bad.log" 2>&1; rc=$?; set -e
[[ $rc != 0 ]] || die 'start with a broken conf exited 0'
grep -q 'E506' "$W/start-bad.log" || { cat "$W/start-bad.log"; die 'no E506 in the output'; }
[[ -d $A/nginx/shared/quarantine/$PB ]] || die "B files were not quarantined to nginx/shared/quarantine/$PB"
[[ ! -e $A/$bconf ]] || die 'broken conf still in the live tree'
assert_detached $PB
serves api.a.local.nself.org
ok 'E506; B files quarantined; A keeps serving; B detached'
[[ $(mode_of "$A/nginx/shared/quarantine/$PB") == 700 ]] || die 'quarantine directory is not 0700'
ok 'quarantine directory is 0700'
# Recover: drop the quarantine, rebuild B (regenerates conf and registration), start it.
rm -rf "$A/nginx/shared/quarantine/$PB"
nself "$B" build --yes >"$W/build-b3.log" 2>&1 || { cat "$W/build-b3.log"; die 'rebuild of B failed'; }
nself "$B" start --timeout 240 >"$W/start-b5.log" 2>&1 || { cat "$W/start-b5.log"; die 'start of B after recovery failed'; }
serves api.b.local.nself.org

step 'R3: a neighbour broken conf quarantines the neighbour, not the starter'
cconf=$(jq -r '.files[] | select(startswith("nginx/sites/"))' "$A/nginx/shared/$PC.json" | head -1)
[[ -n $cconf ]] || die 'C registration lists no site file'
printf 'this is not valid nginx;\n' >> "$A/$cconf"
nself "$B" stop >/dev/null 2>&1 || die 'stop B failed'
nself "$B" start --timeout 240 >"$W/start-r3.log" 2>&1 || { cat "$W/start-r3.log"; die 'start of B failed while C conf was broken'; }
[[ -d $A/nginx/shared/quarantine/$PC ]] || die "C files were not quarantined"
[[ ! -d $A/nginx/shared/quarantine/$PB ]] || die 'B was quarantined instead of C'
serves api.a.local.nself.org; serves api.b.local.nself.org
[[ $(mode_of "$A/nginx/shared/quarantine/$PC") == 700 ]] || die 'C quarantine directory is not 0700'
while IFS= read -r key; do
  [[ $(mode_of "$key") == 600 ]] || die "quarantined key $key is not 0600"
done < <(find "$A/nginx/shared/quarantine/$PC" -name '*.pem' -path '*privkey*' -o -name '*.key' 2>/dev/null)
ok 'start in B quarantined C; A and B keep serving; modes 0700/0600'
rm -rf "$A/nginx/shared/quarantine/$PC"
nself "$C" build --yes >"$W/build-c2.log" 2>&1 || { cat "$W/build-c2.log"; die 'rebuild of C failed'; }
nself "$C" stop >/dev/null 2>&1 || true
nself "$C" start --timeout 240 >"$W/start-c2.log" 2>&1 || { cat "$W/start-c2.log"; die 'start of C failed'; }
serves api.c.local.nself.org

step 'ACME dry run in a fronted project'
# The ACME client reads the age key; keep it inside the scratch tree, never in the real home.
export SECRETS_AGE_KEY_PATH=$W/age-key.txt
nself "$B" secrets init >"$W/secrets-init.log" 2>&1 || { cat "$W/secrets-init.log"; die 'secrets init failed'; }
nself "$B" trust ssl renew --acme --dry-run >"$W/acme-dry.log" 2>&1 || { cat "$W/acme-dry.log"; die 'ssl renew --acme --dry-run failed'; }
grep -q "${OWNER}_nginx" "$W/acme-dry.log" || { cat "$W/acme-dry.log"; die 'dry run does not name the owner nginx'; }
grep -q "$A/ssl" "$W/acme-dry.log" || { cat "$W/acme-dry.log"; die "dry run does not name $A/ssl"; }
ok 'dry run targets the owner nginx and its ssl dir'

step 'R2b: uninstall in a fronted project removes it from the owner'
bfiles=$(jq -r '.files[]' "$A/nginx/shared/$PB.json")
nself "$B" uninstall --yes >"$W/uninstall-b.log" 2>&1 || { cat "$W/uninstall-b.log"; die 'uninstall in B failed'; }
[[ ! -e $A/nginx/shared/$PB.json ]] || die 'B registration still present'
while IFS= read -r f; do [[ -z $f || ! -e $A/$f ]] || die "B file $f still in the owner tree"; done <<<"$bfiles"
serves api.a.local.nself.org; serves api.c.local.nself.org
ok 'uninstall removed B registration and files; A and C keep serving'

echo "evidence: $EVIDENCE"
