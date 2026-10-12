#!/usr/bin/env bash
# Linux request parity and refusal proof for ADR 0027 / EPIC DEPL D3-D6.
set -euo pipefail
export LC_ALL=C
# Keep every install/telemetry path away from the live ping service.
export NSELF_PING_URL=http://127.0.0.1:9
case ${1:-} in
  --round-trip) echo 'round-trip leg is P7-DEPL-04' >&2; exit 2 ;;
  --add) [[ $# == 1 ]] || exit 2 ;;
  *) echo 'usage: proxy-swap.sh --add|--round-trip' >&2; exit 2 ;;
esac
[[ $(uname -s) == Linux ]] || echo 'PROVISIONAL (Constitution §10.3): Linux evidence is required'
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
FIXTURE=$ROOT/scripts/e2e/fixtures/proxy
PLUGINS_REF=${PLUGINS_REF:-b3f4cc1fc2c6a4e6636f9e6dc607d30c247dd931}
for tool in docker jq python3 curl openssl go git; do
  command -v "$tool" >/dev/null || { echo "FAIL: required tool missing: $tool" >&2; exit 1; }
done
command -v sha256sum >/dev/null || command -v shasum >/dev/null || {
  echo 'FAIL: sha256sum or shasum is required' >&2; exit 1;
}
sha256() {
  if command -v sha256sum >/dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi
}
epoch_ms() {
  local value
  value=$(date +%s%3N)
  case $value in
    ''|*[!0-9]*) python3 -c 'import time; print(time.time_ns() // 1000000)' ;;
    *) printf '%s\n' "$value" ;;
  esac
}
docker compose version >/dev/null || { echo 'FAIL: Docker Compose is unavailable' >&2; exit 1; }
W=$(mktemp -d "${E2E_TMP:-${RUNNER_TEMP:-/tmp}}/proxy-swap.XXXXXX")
EVIDENCE=${E2E_EVIDENCE:-$W/evidence}
mkdir -p "$W/evidence" "$W/home" "$W/registry" "$W/a" "$W/b"
registry_pid='' probe_pid='' swap_started=0
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
redact_logs() {
  python3 -c 'import re, sys
for line in sys.stdin:
    sys.stdout.write(re.sub(r"(PASSWORD|SECRET|TOKEN|KEY)[A-Z_]*=\S+", "<redacted>", line))'
}
swap_diagnostics() (
  # Best-effort evidence must neither interrupt cleanup nor replace its exit status.
  set +e
  trap - ERR
  local dir project service id ids generated
  for dir in "$W/a" "$W/b"; do
    project=$(compose "$dir" config --format json 2>"$W/evidence/swap-config-$(basename "$dir").stderr" | jq -er .name)
    if [[ -z $project ]]; then
      project=$(python3 - "$dir/.env" <<'PY'
import pathlib, sys
for line in pathlib.Path(sys.argv[1]).read_text().splitlines():
    if line.startswith('PROJECT_NAME='): print(line.split('=', 1)[1]); break
PY
)
    fi
    docker ps -a --filter "label=com.docker.compose.project=$project" >"$W/evidence/swap-docker-ps-$(basename "$dir").txt" 2>&1
    compose "$dir" ps -a --format json >"$W/evidence/swap-compose-ps-$(basename "$dir").json" 2>"$W/evidence/swap-compose-ps-$(basename "$dir").stderr"
    for service in traefik traefik-config traefik-acme nginx; do
      ids=$(docker ps -aq --filter "label=com.docker.compose.project=$project" --filter "label=com.docker.compose.service=$service")
      : >"$W/evidence/swap-$service-$(basename "$dir").log"
      for id in $ids; do
        printf 'container %s\n' "$id" >>"$W/evidence/swap-$service-$(basename "$dir").log"
        docker logs --tail 200 "$id" 2>&1 | redact_logs >>"$W/evidence/swap-$service-$(basename "$dir").log"
        if [[ $service == traefik ]]; then
          docker port "$id" >"$W/evidence/swap-traefik-ports-$(basename "$dir").txt" 2>&1
        fi
      done
    done
    generated=$dir/.nself/generated/traefik
    python3 - "$generated" "$W/evidence" "$(basename "$dir")" <<'PY'
import pathlib, re, sys
root, evidence, label = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2]), sys.argv[3]
listing = evidence / ('swap-traefik-generated-' + label + '.txt')
if not root.is_dir():
    listing.write_text('generated traefik directory is absent\n')
    raise SystemExit(0)
listing.write_text('\n'.join(str(p.relative_to(root)) + (' -> ' + str(p.readlink()) if p.is_symlink() else '') for p in sorted(root.rglob('*'))) + '\n')
source = root / 'dynamic.yml'
if not source.is_file():
    listing.write_text(listing.read_text() + 'dynamic.yml is absent\n')
    raise SystemExit(0)
resolved = source.resolve()
if root.resolve() not in resolved.parents or 'ssl' in resolved.parts:
    raise SystemExit('dynamic.yml refused: path escapes generated directory or enters ssl/')
data = source.read_text()
# Only configuration and certificate file paths are permitted, never PEM/key bytes.
if '-----BEGIN' in data or re.search(r'(?im)^\s*[A-Za-z0-9+/]{64,}={0,2}\s*$', data) or re.search(r'(?i)[\"\x27]?(?:private[_-]?key|key)[\"\x27]?\s*:', data):
    raise SystemExit('dynamic.yml refused: possible key material')
(evidence / ('swap-traefik-dynamic-' + label + '.yml')).write_text(data)
PY
  done
  command -v ss >/dev/null && ss -ltnp >"$W/evidence/swap-listeners.txt" 2>&1
  curl --verbose --head --noproxy '*' -k --max-time 10 --resolve 'hasura.proxy.test:443:127.0.0.1' https://hasura.proxy.test/v1/version >"$W/evidence/swap-probe-headers.txt" 2>"$W/evidence/swap-probe-verbose.txt"
  return 0
)
cleanup() {
  local rc=$?
  trap - EXIT
  [[ -z $probe_pid ]] || kill "$probe_pid" 2>/dev/null || true
  if [[ $rc != 0 && $swap_started == 1 ]]; then
    swap_diagnostics >"$W/evidence/swap-diagnostics.stdout" 2>"$W/evidence/swap-diagnostics.stderr" || true
  fi
  for dir in "$W/a" "$W/b"; do
    [[ ! -f $dir/.nself/compose-files.txt ]] || compose "$dir" down -v --remove-orphans 2>&1 | redact_logs >"$W/evidence/cleanup-$(basename "$dir").log" || true
  done
  [[ -z $registry_pid ]] || kill "$registry_pid" 2>/dev/null || true
  # Containers may own generated fixture files; restore only this mktemp tree.
  if [[ $(uname -s) == Linux ]] && command -v sudo >/dev/null; then
    sudo -n chown -R "$(id -u):$(id -g)" "$W" || {
      echo 'FAIL: could not restore temporary fixture ownership' >&2; rc=1;
    }
  fi
  if [[ $EVIDENCE != "$W/evidence" ]]; then
    mkdir -p "$EVIDENCE"
    cp -R "$W/evidence/." "$EVIDENCE/"
    python3 - "$W" <<'PY'
import shutil, sys
shutil.rmtree(sys.argv[1])
PY
  else
    python3 - "$W" <<'PY'
import pathlib, shutil, sys
for p in pathlib.Path(sys.argv[1]).iterdir():
    if p.name == 'evidence': continue
    if p.is_dir(): shutil.rmtree(p)
    else: p.unlink()
PY
  fi
  [[ $rc == 0 ]] || echo "FAIL: proxy swap stopped (exit $rc); evidence: $EVIDENCE" >&2
  exit "$rc"
}
trap cleanup EXIT
trap 'echo "FAIL: line $LINENO: $BASH_COMMAND" >&2' ERR
if [[ -z ${PLUGINS_DIR:-} ]]; then
  PLUGINS_DIR=$W/plugins
  git init "$PLUGINS_DIR"
  git -C "$PLUGINS_DIR" fetch --depth 1 https://github.com/nself-org/plugins.git "$PLUGINS_REF"
  git -C "$PLUGINS_DIR" checkout FETCH_HEAD
else
  [[ $(git -C "$PLUGINS_DIR" rev-parse HEAD) == "$(git -C "$PLUGINS_DIR" rev-parse "$PLUGINS_REF")" ]] || {
    echo "FAIL: PLUGINS_DIR must be checked out at $PLUGINS_REF" >&2; exit 1;
  }
fi
export HOME=$W/home
export NSELF_V15=1
export NSELF_LICENSE_SKIP_VERIFY=1
# The install command acknowledges --force but does not propagate it to this gate.
export NSELF_LICENSE_SKIP_VERIFY_FORCE=1
# security.go documents this development opt-out; archive checksums still verify.
export NSELF_PLUGIN_REQUIRE_CHECKSUM=0
CGO_ENABLED=0 go build -mod=vendor -o "$W/nself" "$ROOT/cmd/nself"
cp "$FIXTURE/project.env.fixture" "$W/a/.env"
cp "$FIXTURE/project.env.fixture" "$W/b/.env"
chmod 600 "$W/a/.env" "$W/b/.env"
for dir in "$W/a" "$W/b"; do
  printf '\nNSELF_PLUGIN_DIR=%s/.nself/plugins\n' "$dir" >> "$dir/.env"
done
python3 - "$W/b/.env" <<'PY'
import pathlib, sys
p=pathlib.Path(sys.argv[1]); p.write_text(p.read_text().replace('proxy-swap-a','proxy-swap-b').replace('proxy.test','proxy-b.test')+'\nNGINX_HTTP_PORT=18080\nNGINX_HTTPS_PORT=18443\n')
PY
# Source packages contain plugin.json at the archive root, as extractTarGz expects.
python3 - "$PLUGINS_DIR/free/traefik" "$FIXTURE/plugins/echo" "$W/registry" <<'PY'
import hashlib, json, pathlib, sys, tarfile
out=pathlib.Path(sys.argv[3]); plugins={}
for source in sys.argv[1:3]:
    source=pathlib.Path(source); m=json.loads((source/'plugin.json').read_text()); name=m['name']
    target=out/'plugins'/name/'tarball'; target.parent.mkdir(parents=True)
    with tarfile.open(target,'w:gz') as tar:
        for p in sorted(source.rglob('*')):
            if p.is_file(): tar.add(p,arcname=str(p.relative_to(source)))
    plugins[name]={'name':name,'version':m['version'],'tier':'free','license':'free','licenseType':'free','publishStatus':'experimental','checksum':hashlib.sha256(target.read_bytes()).hexdigest()}
data=json.dumps({'plugins':plugins})
(out/'registry.json').write_text(data); (out/'index.html').write_text(data)
PY
# Bind port zero in the server itself, avoiding a free-port discovery race.
python3 - "$W/registry" "$W/registry.port" >"$W/evidence/registry.log" 2>&1 <<'PY' &
import functools, http.server, pathlib, sys
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(http.server.SimpleHTTPRequestHandler,directory=sys.argv[1]))
pathlib.Path(sys.argv[2]).write_text(str(server.server_port)); server.serve_forever()
PY
registry_pid=$!
for _ in $(seq 1 100); do [[ ! -f $W/registry.port ]] || break; sleep .05; done
NSELF_PLUGIN_REGISTRY=http://127.0.0.1:$(cat "$W/registry.port")
export NSELF_PLUGIN_REGISTRY
# remove resolves its plugin dir before loading .env, so provide the same path up front.
nself() { local dir=$1; shift; (cd "$dir" && NSELF_PLUGIN_DIR="$dir/.nself/plugins" "$W/nself" "$@"); }
pin_fixture_credentials() {
  # Reuse generated database/admin credentials, which build does not persist.
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
manifest() { bash "$ROOT/scripts/e2e/gen-manifest.sh" "$1" > "$W/evidence/$2"; }
same() { diff -u "$W/evidence/$1" "$W/evidence/$2" || { echo "FAIL: $1 differs from $2" >&2; return 1; }; }
wait_healthy() {
  local service=$1 id state
  for _ in $(seq 1 180); do
    id=$(compose "$W/a" ps -q "$service")
    if [[ -n $id ]]; then
      state=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$id")
      if [[ $state == healthy || ( $service != traefik && $state == running ) ]]; then return 0; fi
    fi
    sleep 1
  done
  case $service in
    traefik|traefik-config|traefik-acme|nginx)
      compose "$W/a" logs --no-color --tail 50 "$service" 2>&1 | redact_logs >&2 ;;
  esac
  echo "FAIL: $service did not become healthy" >&2; return 1
}
nself "$W/a" add echo --force --yes
nself "$W/a" build --yes
pin_fixture_credentials "$W/a"
# D2 snippet cert parsing accepts the flat cert dir; core writes certificates/<dir>.
# Alias only fixture certificate bytes, without editing generated configuration.
ln -s certificates/proxy-test "$W/a/ssl/proxy-test"
nself "$W/a" start --timeout 180
wait_healthy nginx
nself "$W/b" build --yes
pin_fixture_credentials "$W/b"
manifest "$W/b" b-before.manifest
matrix() {
  local label=$1 host path id status redirect ws cert docs_host
  docs_host=$(jq -er '"docs." + .base_domain' "$W/a/.nself/generated/routes.json")
  local output=$W/evidence/$label.matrix
  : > "$output"
  jq -r '.routes[] | .id as $id | .server_names[] | [$id,.] | @tsv' "$W/a/.nself/generated/routes.json" | sort -u |
  while IFS=$'\t' read -r id host; do
    # D-0310: docs.<base> is not served by traefik; exclude this accepted difference.
    [[ $host != "$docs_host" ]] || continue
    path=/; ws=0
    case $id in
      core:hasura) path=/v1/version ;;
      core:auth) path=/healthz ;;
      core:storage) path=/minio/health/live ;;
      internal:1) ws=1 ;;
    esac
    local args=(--silent --show-error --noproxy '*' --resolve "$host:443:127.0.0.1" -k --max-time 10 -D "$W/headers" -o "$W/body")
    if [[ $ws == 1 ]]; then
      # RFC 6455 section 1.3 sample key, built at run time so secret scanners see no literal
      args+=(--http1.1 -H 'Connection: Upgrade' -H 'Upgrade: websocket' -H 'Sec-WebSocket-Version: 13' -H "Sec-WebSocket-Key: $(printf 'the sample nonce' | base64)")
    fi
    # A successful upgrade stays open; curl's timeout 28 is expected only for 101.
    local rc=0
    status=$(curl "${args[@]}" -w '%{http_code}' "https://$host$path") || rc=$?
    if [[ $rc != 0 && ! ( $ws == 1 && $rc == 28 && $status == 101 ) ]]; then
      echo "FAIL: $id $host$path curl exit $rc status $status" >&2; return 1
    fi
    if [[ ( $ws == 1 && $status != 101 ) || ( $ws == 0 && $status != 2?? ) ]]; then
      echo "FAIL: $id $host$path unexpected status $status (websocket=$ws)" >&2; return 1
    fi
    printf 'route %s %s status %s\n' "$id" "$host" "$status" >> "$output"
    python3 - "$W/headers" >> "$output" <<'PY'
import pathlib, sys
headers=[]
for line in pathlib.Path(sys.argv[1]).read_text().splitlines():
    if ':' not in line: continue
    k,v=line.split(':',1)
    if k.lower() not in ('date','server'): headers.append(k.lower()+': '+v.strip())
print('\n'.join(sorted(headers)))
PY
    # whoami includes request headers and container identity; only version/health responses are stable.
    case $id in core:hasura|core:auth|core:storage) printf 'body %s\n' "$(sha256 "$W/body" | cut -d ' ' -f 1)" >> "$output" ;; esac
    # /healthz is intentionally served over HTTP; use the root for redirect proof.
    redirect=$(curl --silent --show-error --noproxy '*' --resolve "$host:80:127.0.0.1" --max-time 10 -o /dev/null -w '%{http_code}' "http://$host/")
    [[ $redirect == 301 || $redirect == 308 ]] || { echo "FAIL: $id HTTP redirect status $redirect" >&2; return 1; }
    printf 'redirect %s\n' "$redirect" >> "$output"
    cert=$(openssl s_client -connect 127.0.0.1:443 -servername "$host" </dev/null 2>/dev/null | openssl x509 -noout -ext subjectAltName)
    printf '%s\n' "$cert" | tr ',' '\n' | sed 's/^[[:space:]]*//' | sort >> "$output"
  done
  [[ -s $output ]]
}
matrix nginx
python3 - "$W/a/.env" <<'PY'
import pathlib,sys
p=pathlib.Path(sys.argv[1]); p.write_text(p.read_text().replace('ENV=dev','ENV=prod'))
PY
manifest "$W/a" prod-before.manifest
rc=0
# Production forbids the dev bypass. A fake free-account key suppresses registration;
# the local registry still verifies its archive checksum, without a paid licence.
NSELF_V15=1 NSELF_LICENSE_SKIP_VERIFY=0 NSELF_LICENSE_SKIP_VERIFY_FORCE=0 NSELF_PLUGIN_LICENSE_KEY=fixture-fake-free-account nself "$W/a" add traefik --force </dev/null >"$W/evidence/prod-plan.stdout" 2>"$W/evidence/prod-plan.stderr" || rc=$?
if [[ $rc != 4 ]] || ! grep -q E403 "$W/evidence/prod-plan.stderr"; then
  echo "FAIL: prod add expected exit 4 E403, got exit $rc" >&2
  cat "$W/evidence/prod-plan.stderr" >&2
  exit 1
fi
cat "$W/evidence/prod-plan.stdout" "$W/evidence/prod-plan.stderr" > "$W/evidence/plan-sample.txt"
# RenderHuman currently omits added services and orphan removals from add's plan.
# Preserve that sample and supplement it with the supported read-only diff verb.
nself "$W/a" build --plan --diff --remove-orphans > "$W/evidence/plan-diff.txt" 2>&1
grep -Eq '^-[[:space:]]+nginx:' "$W/evidence/plan-diff.txt"
grep -Eq '^\+.*[/]traefik/docker-compose[.]plugin[.]yml' "$W/evidence/plan-diff.txt"
python3 - "$W/a/.nself/plugins/traefik/docker-compose.plugin.yml" > "$W/evidence/planned-services.txt" <<'PY'
import pathlib, re, sys
services=False
for line in pathlib.Path(sys.argv[1]).read_text().splitlines():
    if line == 'services:': services=True; continue
    if services and line and not line.startswith((' ', '#')): break
    if services and re.fullmatch(r'  [a-zA-Z0-9_-]+:',line):
        print('planned added service '+line.strip().rstrip(':')+' (fragment referenced by plan)')
PY
grep -q '^planned added service traefik ' "$W/evidence/planned-services.txt"
grep -q '^planned added service traefik-config ' "$W/evidence/planned-services.txt"
manifest "$W/a" prod-after.manifest
same prod-before.manifest prod-after.manifest
# Remove through the CLI without V15 reconciliation while the fixture is prod.
NSELF_V15=0 nself "$W/a" remove traefik
python3 - "$W/a/.env" <<'PY'
import pathlib,sys
p=pathlib.Path(sys.argv[1]); p.write_text(p.read_text().replace('ENV=prod','ENV=dev'))
PY
cp "$FIXTURE/confd-variant/nginx/conf.d/shadow.conf" "$W/a/nginx/conf.d/shadow.conf"
manifest "$W/a" confd-before.manifest
rc=0
NSELF_V15=1 nself "$W/a" add traefik --force --yes >"$W/evidence/confd.stdout" 2>"$W/evidence/confd.stderr" || rc=$?
if [[ $rc != 1 ]] || ! grep -q E481 "$W/evidence/confd.stderr"; then
  echo "FAIL: conf.d add expected exit 1 E481, got exit $rc" >&2
  cat "$W/evidence/confd.stderr" >&2
  exit 1
fi
manifest "$W/a" confd-after.manifest
same confd-before.manifest confd-after.manifest
NSELF_V15=0 nself "$W/a" remove traefik
rm "$W/a/nginx/conf.d/shadow.conf"
swap_started=1
(
  while :; do
    timestamp=$(epoch_ms)
    status=$(curl --silent --noproxy '*' -k --max-time 1 --resolve 'hasura.proxy.test:443:127.0.0.1' -o /dev/null -w '%{http_code}' https://hasura.proxy.test/v1/version) || status=000
    printf '%s %s\n' "$timestamp" "$status"
    sleep .05
  done
) > "$W/evidence/downtime.log" &
probe_pid=$!
NSELF_V15=1 nself "$W/a" add traefik --force --yes
nself "$W/a" build --remove-orphans --yes
if ! nself "$W/a" start --timeout 180; then
  compose "$W/a" logs --no-color --tail 60 traefik traefik-config traefik-acme nginx 2>&1 | redact_logs > "$W/evidence/swap-start-failed.log"
  compose "$W/a" ps -a --format json > "$W/evidence/swap-start-failed.ps.json"
  echo 'FAIL: proxy swap start failed; container diagnostics saved' >&2
  exit 1
fi
wait_healthy traefik
recovered=0
for _ in $(seq 1 60); do
  status=$(curl --silent --noproxy '*' -k --max-time 1 --resolve 'hasura.proxy.test:443:127.0.0.1' -o /dev/null -w '%{http_code}' https://hasura.proxy.test/v1/version) || status=000
  if [[ $status == 2?? ]]; then recovered=1; break; fi
  sleep .05
done
[[ $recovered == 1 ]] || { echo 'FAIL: stable route did not recover after traefik became healthy' >&2; exit 1; }
sleep 1.1
kill "$probe_pid"; wait "$probe_pid" 2>/dev/null || true; probe_pid=
python3 - "$W/evidence/downtime.log" > "$W/evidence/downtime.txt" <<'PY'
import sys
start=None; worst=0; last=None
for line in open(sys.argv[1]):
    timestamp,status=line.split(); timestamp=int(timestamp); last=timestamp
    if not status.startswith('2'):
        if start is None: start=timestamp
    elif start is not None:
        worst=max(worst,timestamp-start); start=None
if start is not None: worst=max(worst,last-start)
print('DOWNTIME_MS='+str(worst))
PY
cat "$W/evidence/downtime.txt"
matrix traefik
same nginx.matrix traefik.matrix
for dir in "$W/a" "$W/b"; do
  project=$(compose "$dir" config --format json | jq -er .name)
  ids=$(docker ps -q --filter "label=com.docker.compose.project=$project")
  if [[ -n $ids ]]; then
    # shellcheck disable=SC2086
    docker inspect $ids > "$W/evidence/mounts-$(basename "$dir").json"
    jq -e 'all(.[]; all(.Mounts[]; .Source != "/var/run/docker.sock" and .Destination != "/var/run/docker.sock"))' "$W/evidence/mounts-$(basename "$dir").json"
  fi
done
nself "$W/b" build --yes
compose "$W/b" config --services | grep -qx nginx
manifest "$W/b" b-after.manifest
same b-before.manifest b-after.manifest
echo "PASS: proxy add parity, E403/E481 refusals, no Docker socket, project B unchanged; evidence: $EVIDENCE"
