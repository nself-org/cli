# Sourced by hasura-upgrade-smoke.sh after ROOT, FIX and result helpers are set.
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
